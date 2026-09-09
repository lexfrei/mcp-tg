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

// The failure that motivated the retry middleware: Telegram answering a
// transcription request with INTERDC_102_CALL_ERROR, a 500 naming an internal
// DC hop it could not complete. Once the middleware has resent the query and
// been refused every time, the tools layer must say whose fault it is —
// "rpc error code 500: INTERDC_CALL_ERROR (102)" alone reads as a dead end.
func TestWrapTelegramError_ServerErrorSurfacesAsTransient(t *testing.T) {
	interdcErr := tgerr.New(500, "INTERDC_102_CALL_ERROR")

	wrapped := wrapTelegramError(interdcErr)
	if !errors.Is(wrapped, ErrServerError) {
		t.Fatalf("a 500 must be marked ErrServerError, got: %v", wrapped)
	}

	got := wrapped.Error()
	if !strings.Contains(got, "retry") {
		t.Errorf("a server error must invite a retry, got: %q", got)
	}

	if !errors.Is(wrapped, interdcErr) {
		t.Error("wrapTelegramError must preserve the original rpc error as cause")
	}
}

// The whole 500 class carries the same meaning and the same remedy, so the
// marker is driven by the code rather than a table of error names.
func TestWrapTelegramError_ServerErrorCoversTheWholeClass(t *testing.T) {
	for _, name := range []string{"RPC_CALL_FAIL", "WORKER_BUSY_TOO_LONG_RETRY", "INTERDC_4_CALL_RICH_ERROR"} {
		wrapped := wrapTelegramError(tgerr.New(500, name))
		if !errors.Is(wrapped, ErrServerError) {
			t.Errorf("%s must be marked ErrServerError, got: %v", name, wrapped)
		}
	}
}

// telegramErr composes wrapTelegramError, so the marker must survive through
// the wrapper the handlers actually call.
func TestTelegramErr_ServerErrorKeepsSentinel(t *testing.T) {
	wrapped := telegramErr("failed to transcribe audio", tgerr.New(500, "INTERDC_102_CALL_ERROR"))

	if !errors.Is(wrapped, ErrServerError) {
		t.Fatalf("telegramErr must keep the ErrServerError marker, got: %v", wrapped)
	}

	if !errors.Is(wrapped, ErrTelegram) {
		t.Error("telegramErr must still mark the error as ErrTelegram")
	}
}

// A caller's own mistake must not be dressed up as Telegram's.
func TestWrapTelegramError_ClientErrorNotMarkedServerError(t *testing.T) {
	wrapped := wrapTelegramError(tgerr.New(400, "MSG_VOICE_MISSING"))

	if errors.Is(wrapped, ErrServerError) {
		t.Errorf("a 400 must not be marked ErrServerError, got: %v", wrapped)
	}
}
