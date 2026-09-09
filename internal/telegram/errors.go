package telegram

import "github.com/gotd/td/tgerr"

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

// AsServerError returns the underlying RPC error when err is a Telegram
// 500-class internal error, unwrapping as far as needed to find it. Callers
// that go on to log or explain the failure want the parsed error rather than
// the raw string: gotd's Error() renders INTERDC_102_CALL_ERROR as
// "INTERDC_CALL_ERROR (102)", splitting the argument out of the type, so the
// hop the server failed to reach survives only in rpcErr.Message.
func AsServerError(err error) (*tgerr.Error, bool) {
	rpcErr, ok := tgerr.As(err)
	if !ok || rpcErr.Code != serverErrorCode {
		return nil, false
	}

	return rpcErr, true
}

// IsServerError reports whether err is a Telegram 500-class internal error.
func IsServerError(err error) bool {
	_, ok := AsServerError(err)

	return ok
}
