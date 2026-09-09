package telegram

import (
	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
)

// ErrNotResent marks a 500 on a query the client declined to put back on the
// wire. The reason belongs to whoever declined (safeToResend); the marker
// carries only the fact, which is what the tools layer needs.
//
// It exists so that layer can tell the two shapes of an exhausted 500 apart.
// The ordinary one has been resent and refused every time, and repeating it is
// the remedy. This one was sent ONCE, so a message about spent retries would be
// false, and inviting a repeat would be worse than the resend the middleware
// just refused: the middleware would have put the SAME request back on the
// wire, while a fresh tool call builds one the server has nothing to match
// against.
var ErrNotResent = errors.New("telegram query not resent")

// serverErrorCode is the MTProto error code Telegram answers with when its own
// backend failed to handle an otherwise valid query: INTERDC_X_CALL_ERROR ("an
// error occurred while communicating with DC X"), INTERDC_X_CALL_RICH_ERROR,
// RPC_CALL_FAIL, RPC_MCGET_FAIL, WORKER_BUSY_TOO_LONG_RETRY and friends. The
// class says nothing about the request — it is Telegram reporting an internal
// failure, and the documented remedy is to send the same query again.
//
// It must not be confused with the 303 *_MIGRATE_X redirects, which DO ask the
// client to act: gotd handles those in Client.invokeDirect, either moving the
// primary connection or invoking on a sub-connection. A 500 names no DC the
// client can reach — INTERDC's argument is Telegram's internal designation for
// the hop that failed, not one of the 1-5 datacenters a client connects to — so
// there is nothing to migrate to and re-routing is not the answer.
const serverErrorCode = 500

// randomIDDuplicate is Telegram refusing a send it has ALREADY accepted — the
// deduplication that makes resending a send safe, arriving as an error. It
// carries code 500 like a backend failure does and is the one member of the
// class that must not be treated as one: resending only collects the same
// refusal, and telling the caller to retry makes it send the message twice,
// because every tool call mints a fresh random_id.
const randomIDDuplicate = "RANDOM_ID_DUPLICATE"

// AsServerError returns the underlying RPC error when err is a Telegram
// 500-class internal error, unwrapping as far as needed to find it. Callers
// that go on to log or explain the failure want the parsed error rather than
// the raw string: gotd's Error() renders INTERDC_102_CALL_ERROR as
// "INTERDC_CALL_ERROR (102)", splitting the argument out of the type, so the
// hop the server failed to reach survives only in rpcErr.Message.
//
// randomIDDuplicate is excluded here rather than at each call site, so the
// retry middleware and the tools layer inherit one answer instead of guarding
// separately — a guard only one of them had is what let the middleware spend
// its whole schedule on an error that cannot clear.
func AsServerError(err error) (*tgerr.Error, bool) {
	rpcErr, ok := tgerr.As(err)
	if !ok || rpcErr.Code != serverErrorCode || rpcErr.Type == randomIDDuplicate {
		return nil, false
	}

	return rpcErr, true
}

// IsServerError reports whether err is a Telegram 500-class internal error.
func IsServerError(err error) bool {
	_, ok := AsServerError(err)

	return ok
}
