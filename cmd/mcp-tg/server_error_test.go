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
