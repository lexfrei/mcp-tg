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
	bare := tgerr.New(ServerErrorCode, "INTERDC_102_CALL_ERROR")

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
	}

	for _, err := range cases {
		if IsServerError(err) {
			t.Errorf("must not classify %v as a server error", err)
		}
	}
}
