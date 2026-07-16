package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const maxFloodRetries = 3

// newFloodWaitMiddleware makes at most maxFloodRetries ATTEMPTS at a
// FLOOD_WAIT-rejected call, honouring the server-specified delay between them —
// so the constant bounds calls, not retries: 3 attempts means the original plus
// two retries, which is why the test asserts maxFloodRetries-1 WARN lines.
// Each retried FLOOD_WAIT logs one WARN carrying retryAfter so a rate-limit is
// visible at the default log level
// (issue: it used to sit at DEBUG, invisible in a post-mortem). A context
// cancelled mid-backoff also logs at WARN before surfacing ctx.Err(); the raw
// flood error after the last attempt is passed through for the tools layer to
// turn into ErrFloodWait.
func newFloodWaitMiddleware(logger *slog.Logger) telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			for attempt := range maxFloodRetries {
				err := next.Invoke(ctx, input, output)
				if err == nil {
					return nil
				}

				wait, isFlood := tgerr.AsFloodWait(err)
				if !isFlood || attempt == maxFloodRetries-1 {
					return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
				}

				logger.Warn("Telegram FLOOD_WAIT — backing off before retry",
					"retryAfter", wait, "attempt", attempt+1, "maxAttempts", maxFloodRetries)

				select {
				case <-time.After(wait):
				case <-ctx.Done():
					logger.Warn("context cancelled during FLOOD_WAIT backoff", "retryAfter", wait)

					return ctx.Err()
				}
			}

			return nil
		}
	}
}
