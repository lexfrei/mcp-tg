package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// lostInFlightError is what gotd hands back when the connection closes under a
// query the server had already acknowledged: rpc.Engine wraps the plain
// cancellation of its own context, so nothing in the chain names the
// connection. Shape taken from a live daemon log.
func lostInFlightError() error {
	return errors.Wrap(errors.Wrap(context.Canceled, "engine forcibly closed"), "rpcDoRequest")
}

func historyRequest() bin.Encoder {
	return &tg.MessagesGetHistoryRequest{
		Peer:  &tg.InputPeerChannel{ChannelID: 4405002039, AccessHash: 1},
		Limit: 100,
	}
}

func invokeWithConnectionLossRetry(
	t *testing.T, ctx context.Context, next *recordingInvoker, logger *slog.Logger, input bin.Encoder,
) error {
	t.Helper()

	mw := newConnectionLossMiddleware(logger)

	return mw(next)(ctx, input, bin.Decoder(nil))
}

func TestConnectionLoss_PassthroughSuccess(t *testing.T) {
	var buf bytes.Buffer
	next := &recordingInvoker{}

	err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)), historyRequest())
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

// The failure that motivated this middleware: Telegram closed the connection
// while twenty-one history reads were in flight, and the three the server had
// acknowledged came back as "context canceled" with the caller still waiting.
// A read is safe to send again, so the caller must never see that error.
func TestConnectionLoss_ResendsAQueryTheConnectionDropped(t *testing.T) {
	var buf bytes.Buffer
	next := &recordingInvoker{errs: []error{lostInFlightError()}}

	err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)), historyRequest())
	if err != nil {
		t.Fatalf("a query lost to a dropped connection must be resent and succeed, got: %v", err)
	}

	if len(next.inputs) != 2 {
		t.Fatalf("expected one resend (2 attempts), got %d", len(next.inputs))
	}

	if got := strings.Count(buf.String(), "level=WARN"); got != 1 {
		t.Errorf("expected one WARN for the resend, got %d\n%s", got, buf.String())
	}

	// The post-mortem question is "which call was it": the log line must name
	// the request, and gotd's TypeName is the form the API docs use.
	if !strings.Contains(buf.String(), "messages.getHistory") {
		t.Errorf("the resend WARN must name the request, got: %s", buf.String())
	}
}

// A resend must put the SAME request value back on the wire: a send carries a
// random_id Telegram deduplicates against, and a re-encoded copy would turn
// one message into two.
func TestConnectionLoss_ResendsTheIdenticalRequest(t *testing.T) {
	next := &recordingInvoker{errs: []error{lostInFlightError()}}

	err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.DiscardHandler), historyRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(next.inputs) != 2 || next.inputs[0] != next.inputs[1] {
		t.Error("the resend must reuse the identical request value, not a re-encoded copy")
	}
}

// The same context.Canceled with the CALLER's context done is the caller
// leaving. Resending on their behalf would run a query nobody is waiting for.
func TestConnectionLoss_PassthroughCallerCancellation(t *testing.T) {
	var buf bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	next := &recordingInvoker{errs: []error{errors.Wrap(context.Canceled, "rpcDoRequest")}}

	err := invokeWithConnectionLossRetry(t, ctx, next, slog.New(slog.NewTextHandler(&buf, nil)), historyRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a caller's cancellation must surface as such, got: %v", err)
	}

	if errors.Is(err, telegram.ErrLostInFlight) {
		t.Error("a caller's cancellation must not be marked as a lost query")
	}

	if len(next.inputs) != 1 {
		t.Errorf("a cancelled caller's query must not be resent, got %d attempts", len(next.inputs))
	}

	if buf.Len() != 0 {
		t.Errorf("a caller's cancellation must not log a resend, got: %s", buf.String())
	}
}

// Anything Telegram actually answered is an answer; this middleware only steps
// in when there was none.
func TestConnectionLoss_PassthroughOtherErrors(t *testing.T) {
	for name, cause := range map[string]error{
		"client error":      tgerr.New(400, "PEER_ID_INVALID"),
		"server error":      tgerr.New(500, "INTERDC_102_CALL_ERROR"),
		"deadline exceeded": errors.Wrap(context.DeadlineExceeded, "rpcDoRequest"),
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			next := &recordingInvoker{errs: []error{cause}}

			err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)), historyRequest())
			if !errors.Is(err, cause) {
				t.Fatalf("expected the original error, got: %v", err)
			}

			if errors.Is(err, telegram.ErrLostInFlight) {
				t.Error("an answered query must not be marked as lost")
			}

			if len(next.inputs) != 1 {
				t.Errorf("an answered query must not be resent, got %d attempts", len(next.inputs))
			}

			if buf.Len() != 0 {
				t.Errorf("an answered query must not log a resend, got: %s", buf.String())
			}
		})
	}
}

// The server may have run an acknowledged query before the connection closed.
// For the requests safeToResend holds back that means a second chat, a second
// profile photo or a second invite link, so they stay un-resent here exactly as
// they do on a 500 — and carry both marks, so the tools layer can say what
// happened and what not to do about it.
func TestConnectionLoss_DoesNotResendCreatingRequests(t *testing.T) {
	for name, request := range map[string]bin.Encoder{
		"channel":       &tg.ChannelsCreateChannelRequest{Title: "example"},
		"chat":          &tg.MessagesCreateChatRequest{Title: "example"},
		"profile photo": &tg.PhotosUploadProfilePhotoRequest{},
		"invite link":   &tg.MessagesExportChatInviteRequest{Peer: &tg.InputPeerChannel{ChannelID: 1, AccessHash: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			next := &recordingInvoker{errs: []error{lostInFlightError()}}

			err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)), request)

			if len(next.inputs) != 1 {
				t.Errorf("a creating request must not be resent, got %d attempts", len(next.inputs))
			}

			if !errors.Is(err, telegram.ErrLostInFlight) {
				t.Errorf("a lost creating request must be marked as lost, got: %v", err)
			}

			if !errors.Is(err, telegram.ErrNotResent) {
				t.Errorf("a query held back from the resend must be marked as such, got: %v", err)
			}

			if !errors.Is(err, context.Canceled) {
				t.Errorf("the original error must survive the marking, got: %v", err)
			}

			if buf.Len() != 0 {
				t.Errorf("a request that is not resent must not log a resend, got: %s", buf.String())
			}
		})
	}
}

// One resend, not a loop: a connection that dies twice under the same query
// is a network that is not coming back fast enough for a blocked agent. The
// second loss surfaces marked, so the tools layer can turn it into a hint
// instead of a bare "context canceled" that reads as the caller's own doing.
func TestConnectionLoss_SecondLossSurfacesMarked(t *testing.T) {
	var buf bytes.Buffer
	next := &recordingInvoker{errs: []error{lostInFlightError(), lostInFlightError()}}

	err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)), historyRequest())
	if err == nil {
		t.Fatal("a query lost twice must fail")
	}

	if len(next.inputs) != 2 {
		t.Errorf("expected exactly one resend (2 attempts), got %d", len(next.inputs))
	}

	if !errors.Is(err, telegram.ErrLostInFlight) {
		t.Errorf("a query lost twice must be marked as lost, got: %v", err)
	}

	if errors.Is(err, telegram.ErrNotResent) {
		t.Errorf("a query that WAS resent must not be marked as held back, got: %v", err)
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("the original error must survive the marking, got: %v", err)
	}
}

// The resend's own answer is Telegram's verdict on the query and travels back
// untouched: marking it as a connection loss would hide a refusal behind a
// network story.
func TestConnectionLoss_ResendAnswerSurfacesRaw(t *testing.T) {
	refusal := tgerr.New(400, "PEER_ID_INVALID")
	next := &recordingInvoker{errs: []error{lostInFlightError(), refusal}}

	err := invokeWithConnectionLossRetry(t, context.Background(), next, slog.New(slog.DiscardHandler), historyRequest())
	if !errors.Is(err, refusal) {
		t.Fatalf("expected the resend's own answer, got: %v", err)
	}

	if errors.Is(err, telegram.ErrLostInFlight) {
		t.Error("an answered resend must not be marked as lost")
	}
}
