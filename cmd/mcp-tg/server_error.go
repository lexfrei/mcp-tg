package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
)

const (
	// maxServerErrorAttempts bounds CALLS, not retries: 4 attempts is the
	// original plus three retries.
	maxServerErrorAttempts = 4
	// serverErrorBaseDelay is the first backoff; it doubles per retry, so the
	// four attempts span 1s + 2s + 4s = 7s of waiting at most.
	//
	// TDLib's NetQueryDelayer uses the same doubling schedule from the same 1s
	// start but keeps resending until the accumulated sleep would pass its own
	// limit, ~31s over five resends at the default. The shorter budget
	// here is deliberate: every RPC this middleware guards is a synchronous MCP
	// tool call with an agent blocked on it, and an exhausted retry does not
	// dead-end — it surfaces as tools.ErrServerError, which tells the caller the
	// failure is Telegram's and is worth retrying.
	serverErrorBaseDelay = time.Second
)

// newServerErrorMiddleware retries a query Telegram answered with a 500-class
// internal error, backing off exponentially between attempts.
//
// The 500 class means Telegram's own backend failed to serve an otherwise valid
// query — INTERDC_X_CALL_ERROR ("an error occurred while communicating with DC
// X"), RPC_CALL_FAIL, WORKER_BUSY_TOO_LONG_RETRY. Resending is what every
// reference client does with it, and only mcp-tg gave up on the first one:
// TDLib routes every code-500 query to NetQueryDelayer, which resends on a
// timeout doubling from 1s until the ACCUMULATED sleep would pass
// total_timeout_limit_ — 60s only where the caller leaves the default, and
// several of them do not; WORKER_BUSY_TOO_LONG_RETRY is pinned flat at 1s
// instead of doubling. Telethon retries six error types after a 2s sleep,
// ServerError and both InterdcCall kinds among them; Pyrogram retries
// InternalServerError after 0.5s. None of them re-route the query — that is
// the 303 *_MIGRATE_X path, which gotd already handles.
//
// Resending is safe for SENDS, which is the write this exists for. Every send
// operation carries a crypto-random random_id which Telegram deduplicates
// against, and a retry re-encodes the SAME request value, so the same random_id
// goes back on the wire and a message the server had in fact created before
// failing is not duplicated.
//
// That covers sends, not writes in general: some requests create something the
// account keeps and carry no token at all. Those are held back by safeToResend
// below rather than argued about here.
//
// baseDelay is a parameter rather than a bare constant so tests can drive the
// exhaustion path without sleeping for the real schedule; production passes
// serverErrorBaseDelay.
func newServerErrorMiddleware(logger *slog.Logger, baseDelay time.Duration) telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			delay := baseDelay
			resendable := safeToResend(input)

			for attempt := range maxServerErrorAttempts {
				err := next.Invoke(ctx, input, output)
				if err == nil {
					return nil
				}

				rpcErr, isServerErr := tgclient.AsServerError(err)
				if !isServerErr || attempt == maxServerErrorAttempts-1 {
					return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
				}

				if !resendable {
					// Marked, not just returned: the tools layer otherwise reads
					// this as a 500 that survived the schedule and tells the
					// caller to repeat a call that was never retried at all.
					return errors.Mark(err, tgclient.ErrNotResent)
				}

				logServerErrorRetry(logger, rpcErr, delay, attempt)

				select {
				case <-time.After(delay):
				case <-ctx.Done():
					logger.Warn("context cancelled during Telegram server-error backoff", "retryAfter", delay)

					return ctx.Err()
				}

				delay *= 2
			}

			return nil
		}
	}
}

// safeToResend reports whether putting this request back on the wire can be
// undone by the server. Sends carry a random_id Telegram deduplicates against,
// and most of the remaining surface either reads or states a final value (set
// the title, set the permissions) and lands on the same result however often it
// arrives. What is listed below does neither: it CREATES something the account
// keeps, with no token the server could match a resend against, so a resend
// landing after the first attempt was already applied leaves a duplicate
// nothing in the protocol lets the client notice or undo — a second identical
// chat or folder, a second entry in the profile-photo history, a second working
// invite link that is never returned to the caller and so can never be revoked.
//
// updateDialogFilter is the one entry that is not creation-only: the same
// request edits and deletes a folder too, and those carry an explicit id, so
// they are idempotent and lose the resend for nothing. Held back anyway,
// because the type is what a switch can see and the cost of the extra caution
// is one lost retry on a call that states a final value.
//
// This is a DENY-LIST of what has been found, not a proof that nothing else
// qualifies: MTProto marks no request as non-idempotent, so nothing here can be
// derived, and a new tool wrapping another creating method has to be added by
// hand. Cross-checking it against the tools carrying writeAnnotations (the
// repository's own "creates a new entity, not idempotent" category) is the
// cheapest way to look for a gap, but not a sufficient one: an annotation can
// itself be wrong, which is how the invite-link request below arrived here from
// a tool marked read-only.
//
// Two things it deliberately leaves out. The value-setting writes above are
// idempotent in their value but not in the chat history: editChatTitle,
// editPhoto and addChatUser each post a service message per call, so a resend
// leaves a second one. That is visible and deletable, unlike the entries here.
// And this closes only the trigger this middleware adds — gotd's invokeConn
// resends any method on a dead connection, which is not this middleware's to
// fix.
func safeToResend(input bin.Encoder) bool {
	switch input.(type) {
	case *tg.ChannelsCreateChannelRequest,
		*tg.MessagesCreateChatRequest,
		*tg.MessagesUpdateDialogFilterRequest,
		*tg.PhotosUploadProfilePhotoRequest,
		*tg.MessagesExportChatInviteRequest:
		return false
	default:
		return true
	}
}

// logServerErrorRetry emits one WARN per retried attempt, carrying the raw
// error string so a post-mortem names the DC the server failed to reach —
// gotd's Error() renders INTERDC_102_CALL_ERROR as "INTERDC_CALL_ERROR (102)",
// splitting the argument out of the type.
func logServerErrorRetry(logger *slog.Logger, rpcErr *tgerr.Error, delay time.Duration, attempt int) {
	logger.Warn("Telegram internal server error — backing off before retry",
		"rpcError", rpcErr.Message, "retryAfter", delay,
		"attempt", attempt+1, "maxAttempts", maxServerErrorAttempts)
}
