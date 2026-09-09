package telegram

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
)

// The wrapper layer wraps every RPC error with context before it reaches a
// caller ("transcribing audio: ..."), and the middleware sees it bare. Both
// must classify it the same way, so the predicate has to unwrap.
func TestIsServerError_SeesThroughWrapping(t *testing.T) {
	bare := tgerr.New(serverErrorCode, "INTERDC_102_CALL_ERROR")

	if !IsServerError(bare) {
		t.Error("a bare 500 must be recognised")
	}

	if !IsServerError(errors.Wrap(bare, "transcribing audio")) {
		t.Error("a wrapped 500 must be recognised")
	}
}

// The code is what carries the meaning: everything under 500 describes the
// request, and resending it would only collect the same answer.
func TestIsServerError_RejectsEverythingElse(t *testing.T) {
	cases := []error{
		nil,
		errors.New("not an rpc error at all"),
		tgerr.New(400, "MSG_VOICE_MISSING"),
		tgerr.New(403, "PREMIUM_ACCOUNT_REQUIRED"),
		tgerr.New(420, "FLOOD_WAIT_30"),
		// A migration redirect is the one error that DOES ask the client to
		// act, and gotd acts on it; misreading it as an internal failure would
		// spend the retry schedule on a query that needs another datacenter.
		tgerr.New(303, "FILE_MIGRATE_4"),
		// Carries code 500 but describes the request: the server is refusing a
		// send it already accepted. Resending collects the same refusal for the
		// rest of the schedule, and calling it an internal failure invites a
		// retry that duplicates the message.
		tgerr.New(serverErrorCode, "RANDOM_ID_DUPLICATE"),
	}

	for _, err := range cases {
		if IsServerError(err) {
			t.Errorf("must not classify %v as a server error", err)
		}
	}
}

// The middleware needs the parsed error, not just the verdict: the log line
// carries rpcErr.Message so a post-mortem still names the DC hop that failed
// (gotd renders INTERDC_102_CALL_ERROR as "INTERDC_CALL_ERROR (102)"). Handing
// it back here is what keeps the caller from re-parsing what the predicate
// already parsed.
func TestAsServerError_ReturnsTheParsedError(t *testing.T) {
	rpcErr, ok := AsServerError(errors.Wrap(tgerr.New(500, "INTERDC_102_CALL_ERROR"), "transcribing audio"))
	if !ok {
		t.Fatal("a wrapped 500 must be recognised")
	}

	if rpcErr.Message != "INTERDC_102_CALL_ERROR" {
		t.Errorf("the raw message must survive, got: %q", rpcErr.Message)
	}

	if _, ok := AsServerError(tgerr.New(400, "MSG_VOICE_MISSING")); ok {
		t.Error("a 400 must not be reported as a server error")
	}

	if _, ok := AsServerError(errors.New("not an rpc error at all")); ok {
		t.Error("a non-rpc error must not be reported as a server error")
	}
}
