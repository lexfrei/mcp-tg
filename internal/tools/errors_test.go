package tools

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
)

// A FLOOD_WAIT that survives the retry middleware reaches the tools layer as a
// raw rate-limit error. wrapTelegramError must turn it into the ErrFloodWait
// sentinel with a readable "retry after Ns" — not a raw code, and not literal
// JSON — so the caller can back off and retry rather than treat it as a crash.
func TestWrapTelegramError_FloodWaitSurfacesReadableRetry(t *testing.T) {
	floodErr := tgerr.New(420, "FLOOD_WAIT_42")

	wrapped := wrapTelegramError(floodErr)
	if !errors.Is(wrapped, ErrFloodWait) {
		t.Fatalf("FLOOD_WAIT must be marked ErrFloodWait, got: %v", wrapped)
	}

	got := wrapped.Error()
	if !strings.Contains(got, "retry after 42s") {
		t.Errorf("flood wait message must carry a readable retry hint, got: %q", got)
	}

	if !errors.Is(wrapped, floodErr) {
		t.Error("wrapTelegramError must preserve the original flood error as cause")
	}
}

// telegramErr composes wrapTelegramError, so the ErrFloodWait marker must
// survive through the tools-facing wrapper the handlers actually call.
func TestTelegramErr_FloodWaitKeepsSentinel(t *testing.T) {
	wrapped := telegramErr("reading history", tgerr.New(420, "FLOOD_WAIT_5"))

	if !errors.Is(wrapped, ErrFloodWait) {
		t.Fatalf("telegramErr must keep the ErrFloodWait marker, got: %v", wrapped)
	}

	if !errors.Is(wrapped, ErrTelegram) {
		t.Error("telegramErr must still mark the error as ErrTelegram")
	}
}

func TestWrapTelegramError_NonFloodUnaffected(t *testing.T) {
	wrapped := wrapTelegramError(tgerr.New(400, "PEER_ID_INVALID"))

	if errors.Is(wrapped, ErrFloodWait) {
		t.Errorf("a non-flood error must not be marked ErrFloodWait, got: %v", wrapped)
	}
}
