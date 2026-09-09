package tools

import (
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/telegram"
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

// These three are the members a resend actually fixes: Telegram reporting an
// internal failure, with nothing about the request to change. The marker is
// driven by the code because the class has no list to enumerate — Telegram can
// answer 500 with a name this table never heard of. The one name it does check,
// RANDOM_ID_DUPLICATE, is SUBTRACTED in telegram.AsServerError rather than
// added here.
func TestWrapTelegramError_ServerErrorMarksTheBackendFailures(t *testing.T) {
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

// RANDOM_ID_DUPLICATE is a 500 that describes the request rather than the
// backend: Telegram is refusing a send it has ALREADY accepted. The generic
// "retry it" advice is exactly wrong here, because every tool call mints a
// fresh random_id — a caller that follows it defeats the deduplication that
// produced this error and sends the message twice.
func TestWrapTelegramError_DuplicateSendIsNotAdvertisedAsRetryable(t *testing.T) {
	wrapped := wrapTelegramError(tgerr.New(500, "RANDOM_ID_DUPLICATE"))

	if errors.Is(wrapped, ErrServerError) {
		t.Fatalf("a refused duplicate send must not be marked ErrServerError, got: %v", wrapped)
	}

	got := wrapped.Error()
	if strings.Contains(got, "retry") {
		t.Errorf("a refused duplicate send must not invite a retry, got: %q", got)
	}

	if !strings.Contains(got, "already") {
		t.Errorf("the explanation must say the send was already accepted, got: %q", got)
	}
}

// The 500 class is not uniformly transient — gotd's generated docs put
// RANDOM_ID_DUPLICATE, AUTH_RESTART and CHAT_INVALID under the same code as
// INTERDC_X_CALL_ERROR — so the marker classifies on the code alone and must
// not promise anything about a request it never inspected.
func TestWrapTelegramError_ServerErrorVouchesForNothing(t *testing.T) {
	got := wrapTelegramError(tgerr.New(500, "INTERDC_102_CALL_ERROR")).Error()

	if strings.Contains(got, "nothing is wrong with the request") {
		t.Errorf("the marker must not vouch for the request, got: %q", got)
	}
}

// A query the middleware refused to resend reaches here having had no automatic
// retry at all, so the generic message is wrong twice: it reports retries that
// never ran, and it invites the blind repeat the refusal exists to prevent. A
// caller-level repeat is worse than the resend the middleware declined — the
// middleware would have put the SAME request back on the wire, while a new tool
// call builds one the server has nothing to match it against. The message must
// also claim nothing about WHAT the call does: the deny-list holds back a
// request type, and one of those types serves the idempotent folder edit and
// delete as well as folder creation.
func TestWrapTelegramError_UnresentQueryDoesNotInviteABlindRepeat(t *testing.T) {
	unresent := errors.Mark(tgerr.New(500, "RPC_CALL_FAIL"), telegram.ErrNotResent)

	got := wrapTelegramError(unresent).Error()

	if strings.Contains(got, "automatic retries") {
		t.Errorf("no automatic retry ran for this query, got: %q", got)
	}

	if strings.Contains(got, "retry the same call") {
		t.Errorf("a held-back call must not be advertised as safe to repeat, got: %q", got)
	}

	if !strings.Contains(got, "took effect") {
		t.Errorf("the caller must be told to check whether it applied, got: %q", got)
	}

	// "this call creates something" shipped here once and was false for
	// tg_folders_edit and tg_folders_delete, which share a request type with
	// tg_folders_create and are idempotent.
	if strings.Contains(got, "creates") {
		t.Errorf("the message must not claim what the call does, got: %q", got)
	}
}
