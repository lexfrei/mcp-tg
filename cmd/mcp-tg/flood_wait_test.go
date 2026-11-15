package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

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

// A delay above maxFloodSleep goes straight back to the caller: sleeping it
// would hold a synchronous tool call for minutes or hours, while the tools
// layer already turns the error into a readable "retry after Ns". The context
// deadline makes a middleware that sleeps anyway fail fast instead of hanging.
func TestFloodWait_DelayAboveTheCapIsReturnedAtOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	next := &recordingInvoker{errs: []error{tgerr.New(420, "FLOOD_WAIT_3600")}}

	err := invokeWithFloodWait(t, ctx, next, logger)

	wait, isFlood := tgerr.AsFloodWait(err)
	if !isFlood || wait != time.Hour {
		t.Fatalf("a delay above the cap must surface as the raw flood error, got: %v", err)
	}

	if got := len(next.inputs); got != 1 {
		t.Errorf("a delay above the cap must not be retried, got %d attempts", got)
	}

	if !strings.Contains(buf.String(), "retryAfter=1h0m0s") {
		t.Errorf("the skipped wait must be logged with its retryAfter, got: %s", buf.String())
	}
}

// The cap is inclusive: a delay equal to it is still waited out and retried.
func TestFloodWait_DelayAtTheCapIsStillRetried(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	next := &recordingInvoker{errs: []error{
		tgerr.New(420, fmt.Sprintf("FLOOD_WAIT_%d", int(maxFloodSleep/time.Second))),
	}}

	err := invokeWithFloodWait(t, ctx, next, slog.New(slog.DiscardHandler))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("a delay at the cap must enter the backoff (and see the cancelled context), got: %v", err)
	}
}

// The login the server runs at startup keeps the server-named delay whatever
// its length, because a capped FLOOD_WAIT there fails the start (see
// withUncappedFloodWait). The probe is a users.getUsers call, not an auth.*
// one, so the opt-out rides the context.
func TestFloodWait_StartupLoginSleepsPastTheCap(t *testing.T) {
	ctx, cancel := context.WithCancel(withUncappedFloodWait(context.Background()))
	cancel()

	next := &recordingInvoker{errs: []error{tgerr.New(420, "FLOOD_WAIT_3600")}}
	handler := newFloodWaitMiddleware(slog.New(slog.DiscardHandler))(next)

	err := handler(ctx, &tg.UsersGetUsersRequest{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the startup login must enter the backoff past the cap, got: %v", err)
	}
}

// A login driven from a tool call has a caller waiting on it, so its
// sendCode gets the cap like any other request.
func TestFloodWait_ToolCallLoginStepGetsTheCap(t *testing.T) {
	next := &recordingInvoker{errs: []error{tgerr.New(420, "FLOOD_WAIT_3600")}}
	handler := newFloodWaitMiddleware(slog.New(slog.DiscardHandler))(next)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := handler(ctx, &tg.AuthSendCodeRequest{}, nil)
	if _, isFlood := tgerr.AsFloodWait(err); !isFlood || len(next.inputs) != 1 {
		t.Fatalf("a tool-call sendCode above the cap must return at once, got %v after %d attempt(s)", err, len(next.inputs))
	}
}
