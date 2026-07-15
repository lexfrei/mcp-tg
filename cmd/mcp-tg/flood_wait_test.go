package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func invokeWithFloodWait(
	t *testing.T, ctx context.Context, next *recordingInvoker, logger *slog.Logger,
) error {
	t.Helper()

	mw := newFloodWaitMiddleware(logger)
	handler := mw(next)

	return handler(ctx, &tg.ContactsResolveUsernameRequest{Username: "example"}, nil)
}

func TestFloodWait_PassthroughSuccess(t *testing.T) {
	var buf bytes.Buffer
	next := &recordingInvoker{}

	err := invokeWithFloodWait(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("a successful call must not log anything, got: %s", buf.String())
	}
}

func TestFloodWait_PassthroughNonFlood(t *testing.T) {
	var buf bytes.Buffer

	otherErr := tgerr.New(400, "PEER_ID_INVALID")
	next := &recordingInvoker{errs: []error{otherErr}}

	err := invokeWithFloodWait(t, context.Background(), next, slog.New(slog.NewTextHandler(&buf, nil)))
	if !errors.Is(err, otherErr) {
		t.Fatalf("expected the original non-flood error, got: %v", err)
	}

	if strings.Contains(buf.String(), "FLOOD_WAIT") {
		t.Errorf("a non-flood error must not produce a flood log, got: %s", buf.String())
	}
}

// TestFloodWait_LogsWarnWithRetryAfterThenCtxCancel drives the backoff branch
// without sleeping: the flood error trips the WARN log, then an already-cancelled
// context makes the select pick ctx.Done immediately (time.After(30s) is not
// ready), so both the flood WARN and the ctx-cancel WARN fire and ctx.Err()
// surfaces — the exact path that previously dropped the process silently when a
// backoff was cancelled.
func TestFloodWait_LogsWarnWithRetryAfterThenCtxCancel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	next := &recordingInvoker{errs: []error{tgerr.New(420, "FLOOD_WAIT_30")}}

	err := invokeWithFloodWait(t, ctx, next, logger)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled backoff must return ctx.Err(), got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "level=WARN") {
		t.Errorf("FLOOD_WAIT must log at WARN, got: %s", out)
	}

	if !strings.Contains(out, "retryAfter=30s") {
		t.Errorf("FLOOD_WAIT log must carry the server's retryAfter, got: %s", out)
	}

	if !strings.Contains(strings.ToLower(out), "cancel") {
		t.Errorf("a cancelled backoff must log the cancellation, got: %s", out)
	}
}

// TestFloodWait_RetriesThenReturnsAfterExhaustion uses FLOOD_WAIT_0 so the
// time.After backoff fires immediately: three floods exhaust the retries and the
// raw flood error surfaces (the tools layer turns it into ErrFloodWait), with a
// WARN logged for each of the two retried attempts (the last is not retried).
func TestFloodWait_RetriesThenReturnsAfterExhaustion(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	next := &recordingInvoker{errs: []error{
		tgerr.New(420, "FLOOD_WAIT_0"),
		tgerr.New(420, "FLOOD_WAIT_0"),
		tgerr.New(420, "FLOOD_WAIT_0"),
	}}

	err := invokeWithFloodWait(t, context.Background(), next, logger)

	if _, isFlood := tgerr.AsFloodWait(err); !isFlood {
		t.Fatalf("after exhausting retries the raw flood error must surface, got: %v", err)
	}

	if got := strings.Count(buf.String(), "level=WARN"); got != maxFloodRetries-1 {
		t.Errorf("expected %d WARN lines (one per retried attempt), got %d\n%s",
			maxFloodRetries-1, got, buf.String())
	}
}
