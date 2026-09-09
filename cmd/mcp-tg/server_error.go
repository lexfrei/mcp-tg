package main

import (
	"context"
	"log/slog"
	"time"

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
	// start but keeps resending until a 60s budget is spent. The shorter budget
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
// TDLib routes every code-500 query to NetQueryDelayer, which doubles a 1s
// timeout and resends until a 60s budget is spent; Telethon retries
// InterdcCallError, InterdcCallRichError and ServerError after a 2s sleep;
// Pyrogram retries InternalServerError after 0.5s. None of them re-route the
// query — that is the 303 *_MIGRATE_X path, which gotd already handles.
//
// Resending is safe for SENDS, which is the write this exists for. Every send
// operation carries a crypto-random random_id which Telegram deduplicates
// against, and a retry re-encodes the SAME request value, so the same random_id
// goes back on the wire and a message the server had in fact created before
// failing is not duplicated.
//
// That covers sends, not writes in general. channels.createChannel and
// messages.createChat carry no such token, so a resend landing after the server
// applied the first attempt can create a second chat. The exposure is accepted
// rather than solved, and it predates this middleware: gotd's own invokeConn
// already re-sends any query whose connection died, and TDLib delays every 500
// the same way regardless of method.
//
// baseDelay is a parameter rather than a bare constant so tests can drive the
// exhaustion path without sleeping for the real schedule; production passes
// serverErrorBaseDelay.
func newServerErrorMiddleware(logger *slog.Logger, baseDelay time.Duration) telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			delay := baseDelay

			for attempt := range maxServerErrorAttempts {
				err := next.Invoke(ctx, input, output)
				if err == nil {
					return nil
				}

				rpcErr, isServerErr := tgclient.AsServerError(err)
				if !isServerErr || attempt == maxServerErrorAttempts-1 {
					return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
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

// logServerErrorRetry emits one WARN per retried attempt, carrying the raw
// error string so a post-mortem names the DC the server failed to reach —
// gotd's Error() renders INTERDC_102_CALL_ERROR as "INTERDC_CALL_ERROR (102)",
// splitting the argument out of the type.
func logServerErrorRetry(logger *slog.Logger, rpcErr *tgerr.Error, delay time.Duration, attempt int) {
	logger.Warn("Telegram internal server error — backing off before retry",
		"rpcError", rpcErr.Message, "retryAfter", delay,
		"attempt", attempt+1, "maxAttempts", maxServerErrorAttempts)
}
