package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// testServerErrorDelay keeps the exhaustion path off the real 1s/2s/4s
// schedule; the middleware takes the base delay precisely so a test can.
const testServerErrorDelay = time.Microsecond

// interdcError is the failure that motivated this middleware: Telegram
// answering a transcription request with a 500 naming an internal DC hop it
// could not complete. gotd renders it "rpc error code 500: INTERDC_CALL_ERROR
// (102)" — the type with the argument split out.
func interdcError() error {
	return tgerr.New(500, "INTERDC_102_CALL_ERROR")
}

func invokeWithServerErrorRetry(
	t *testing.T, ctx context.Context, next *recordingInvoker, logger *slog.Logger,
) error {
	t.Helper()

	mw := newServerErrorMiddleware(logger, testServerErrorDelay)
	handler := mw(next)

	return handler(ctx, &tg.MessagesTranscribeAudioRequest{
		Peer:  &tg.InputPeerChannel{ChannelID: 4405002039, AccessHash: 1},
		MsgID: 320,
	}, nil)
}

func TestServerError_PassthroughSuccess(t *testing.T) {
	var buf bytes.Buffer
	next := &recordingInvoker{}

	err := invokeWithServerErrorRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Errorf("a successful call must be invoked once, got %d", len(next.inputs))
	}

	if buf.Len() != 0 {
		t.Errorf("a successful call must not log anything, got: %s", buf.String())
	}
}

// A 400 is the caller's problem: resending it would burn the schedule to
// collect the same refusal, so it must surface untouched on the first attempt.
func TestServerError_PassthroughNonServerError(t *testing.T) {
	var buf bytes.Buffer

	otherErr := tgerr.New(400, "MSG_VOICE_MISSING")
	next := &recordingInvoker{errs: []error{otherErr}}

	err := invokeWithServerErrorRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if !errors.Is(err, otherErr) {
		t.Fatalf("expected the original non-500 error, got: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Errorf("a 400 must not be retried, got %d attempts", len(next.inputs))
	}

	if buf.Len() != 0 {
		t.Errorf("a non-500 error must not log a retry, got: %s", buf.String())
	}
}

// FLOOD_WAIT belongs to the middleware wrapping this one; retrying it here
// would spend the fast schedule ignoring a delay the server named.
func TestServerError_PassthroughFloodWait(t *testing.T) {
	var buf bytes.Buffer

	floodErr := tgerr.New(420, "FLOOD_WAIT_30")
	next := &recordingInvoker{errs: []error{floodErr}}

	err := invokeWithServerErrorRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if !errors.Is(err, floodErr) {
		t.Fatalf("expected the flood error to pass through, got: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Errorf("FLOOD_WAIT must not be retried here, got %d attempts", len(next.inputs))
	}
}

// The whole point: a 500 that clears on a resend must never reach the caller.
func TestServerError_RetriesUntilTheQueryGoesThrough(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	next := &recordingInvoker{errs: []error{interdcError(), interdcError()}}

	err := invokeWithServerErrorRetry(t, context.Background(), next, logger)
	if err != nil {
		t.Fatalf("a 500 that clears on retry must succeed, got: %v", err)
	}

	if len(next.inputs) != 3 {
		t.Errorf("expected two retries then success (3 attempts), got %d", len(next.inputs))
	}

	if got := strings.Count(buf.String(), "level=WARN"); got != 2 {
		t.Errorf("expected one WARN per retried attempt, got %d\n%s", got, buf.String())
	}
}

// Resending must put the SAME request value back on the wire. Every send
// operation carries a random_id Telegram deduplicates against, so a retry that
// re-encoded a fresh request would turn one message into two.
func TestServerError_RetriesTheIdenticalRequest(t *testing.T) {
	next := &recordingInvoker{errs: []error{interdcError()}}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	if err := invokeWithServerErrorRetry(t, context.Background(), next, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(next.inputs) != 2 {
		t.Fatalf("expected one retry (2 attempts), got %d", len(next.inputs))
	}

	if next.inputs[0] != next.inputs[1] {
		t.Error("the retry must resend the identical request value, not a re-encoded copy")
	}
}

// After the attempts are spent the RAW rpc error surfaces: the tools layer is
// what turns it into tools.ErrServerError, and it needs the original to do so.
func TestServerError_SurfacesRawErrorAfterExhaustion(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	errs := make([]error, maxServerErrorAttempts)
	for i := range errs {
		errs[i] = interdcError()
	}

	next := &recordingInvoker{errs: errs}

	err := invokeWithServerErrorRetry(t, context.Background(), next, logger)
	if !tgerr.Is(err, "INTERDC_CALL_ERROR") {
		t.Fatalf("after exhausting attempts the raw rpc error must surface, got: %v", err)
	}

	if len(next.inputs) != maxServerErrorAttempts {
		t.Errorf("expected %d attempts, got %d", maxServerErrorAttempts, len(next.inputs))
	}

	if got := strings.Count(buf.String(), "level=WARN"); got != maxServerErrorAttempts-1 {
		t.Errorf("expected %d WARN lines (one per retried attempt), got %d\n%s",
			maxServerErrorAttempts-1, got, buf.String())
	}

	// The DC the server failed to reach is the only diagnostic the error
	// carries; a post-mortem loses it if the log drops the raw message.
	if !strings.Contains(buf.String(), "INTERDC_102_CALL_ERROR") {
		t.Errorf("the retry WARN must carry the raw rpc error, got: %s", buf.String())
	}
}

// A cancelled context must abandon the backoff rather than sleep out the
// schedule for a caller that has already gone away.
func TestServerError_CancelledBackoffReturnsCtxErr(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	next := &recordingInvoker{errs: []error{interdcError()}}

	mw := newServerErrorMiddleware(logger, time.Hour)
	err := mw(next)(ctx, &tg.MessagesTranscribeAudioRequest{MsgID: 320}, bin.Decoder(nil))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled backoff must return ctx.Err(), got: %v", err)
	}

	if !strings.Contains(strings.ToLower(buf.String()), "cancel") {
		t.Errorf("a cancelled backoff must log the cancellation, got: %s", buf.String())
	}
}

// The refusal of an already-accepted send carries code 500 like a backend
// failure does, but it cannot clear: the resend puts the same random_id back on
// the wire and collects the same answer. Retrying it would spend the schedule
// an agent is blocked on, and log two WARN lines naming an internal failure
// that never happened.
func TestServerError_DoesNotRetryAnAlreadyAcceptedSend(t *testing.T) {
	var buf bytes.Buffer

	duplicateErr := tgerr.New(500, "RANDOM_ID_DUPLICATE")
	next := &recordingInvoker{errs: []error{duplicateErr}}

	err := invokeWithServerErrorRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if !errors.Is(err, duplicateErr) {
		t.Fatalf("expected the duplicate-send refusal to pass through, got: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Errorf("a refused duplicate send must not be retried, got %d attempts", len(next.inputs))
	}

	if buf.Len() != 0 {
		t.Errorf("a refused duplicate send must not log an internal-error retry, got: %s", buf.String())
	}
}

// These requests carry no random_id and no other token Telegram could
// deduplicate against, and each creates something the account keeps: a resend
// landing after the server applied the first attempt leaves a second chat, a
// second profile photo in the history, or a second working invite link the
// caller never receives and so can never revoke. Nothing makes the calls
// idempotent, so the only guard available is not resending them.
func TestServerError_DoesNotResendCreatingRequests(t *testing.T) {
	for name, request := range map[string]bin.Encoder{
		"channel": &tg.ChannelsCreateChannelRequest{Title: "example"},
		"chat":    &tg.MessagesCreateChatRequest{Title: "example"},
		// CreateFolder sends the filter with no ID, then looks the result up by
		// title, so a resend has nothing to collide with server-side either.
		"folder": &tg.MessagesUpdateDialogFilterRequest{
			Filter: &tg.DialogFilter{Title: tg.TextWithEntities{Text: "example"}},
		},
		// The same request with an ID is a folder edit, which IS idempotent.
		// Held back all the same: safeToResend switches on the request type, and
		// this pins that the id-bearing form takes the same branch rather than
		// the deny-list quietly narrowing to the create shape.
		"folder edit": &tg.MessagesUpdateDialogFilterRequest{
			ID:     7,
			Filter: &tg.DialogFilter{ID: 7, Title: tg.TextWithEntities{Text: "example"}},
		},
		"profile photo": &tg.PhotosUploadProfilePhotoRequest{},
		"invite link":   &tg.MessagesExportChatInviteRequest{Peer: &tg.InputPeerChannel{ChannelID: 1, AccessHash: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer

			interdcErr := interdcError()
			next := &recordingInvoker{errs: []error{interdcErr}}

			mw := newServerErrorMiddleware(slog.New(slog.NewTextHandler(&buf, nil)), testServerErrorDelay)
			err := mw(next)(context.Background(), request, bin.Decoder(nil))

			if !errors.Is(err, interdcErr) {
				t.Fatalf("expected the original error, got: %v", err)
			}

			if len(next.inputs) != 1 {
				t.Errorf("a creating request must not be resent, got %d attempts", len(next.inputs))
			}

			if buf.Len() != 0 {
				t.Errorf("a request that is not resent must not log a retry, got: %s", buf.String())
			}
		})
	}
}

// Refusing to resend is only half the protection. The error still travels to
// the tools layer, which reads a bare 500 as one that survived the schedule and
// tells the caller to repeat the call. Marking it is what keeps that message
// away from a query that creates something and was sent exactly once.
func TestServerError_UnresentCreatingRequestIsMarked(t *testing.T) {
	next := &recordingInvoker{errs: []error{interdcError()}}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	mw := newServerErrorMiddleware(logger, testServerErrorDelay)
	err := mw(next)(context.Background(), &tg.ChannelsCreateChannelRequest{Title: "example"}, bin.Decoder(nil))

	if !errors.Is(err, telegram.ErrNotResent) {
		t.Fatalf("a query held back from the resend must be marked, got: %v", err)
	}

	if !tgerr.Is(err, "INTERDC_CALL_ERROR") {
		t.Errorf("the original rpc error must survive the marking, got: %v", err)
	}
}

// A query that IS resent must not pick up the marker, or every exhausted 500
// would tell the caller to go check whether it took effect.
func TestServerError_ResentQueryIsNotMarkedUnresent(t *testing.T) {
	errs := make([]error, maxServerErrorAttempts)
	for i := range errs {
		errs[i] = interdcError()
	}

	next := &recordingInvoker{errs: errs}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	err := invokeWithServerErrorRetry(t, context.Background(), next, logger)
	if errors.Is(err, telegram.ErrNotResent) {
		t.Errorf("a query that was resent must not be marked as held back, got: %v", err)
	}
}
