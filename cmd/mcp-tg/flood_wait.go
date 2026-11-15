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

// maxFloodSleep is the longest server-named delay slept out inside a call.
// Every guarded RPC is a synchronous tool call, and Telegram names delays in
// hours for some methods; a longer one goes back at once, where the tools
// layer words it as "retry after Ns". The startup login opts out
// (withUncappedFloodWait). The cap covers the 22-25 s floods observed on a
// live account (2026-07) during the dialog warm, which are worth waiting out
// because the warm is shared.
const maxFloodSleep = 30 * time.Second

// newFloodWaitMiddleware makes at most maxFloodRetries ATTEMPTS at a
// FLOOD_WAIT-rejected call, honouring the server-specified delay between them —
// so the constant bounds calls, not retries: 3 attempts means the original plus
// two retries, which is why the test asserts maxFloodRetries-1 WARN lines.
// A delay above maxFloodSleep is not slept at all.
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

				if wait > maxFloodSleep && !floodWaitUncapped(ctx) {
					logger.Warn("Telegram FLOOD_WAIT longer than the in-call cap — returning it to the caller",
						"retryAfter", wait, "cap", maxFloodSleep)

					return err //nolint:wrapcheck // pass-through: the tools layer words it as a retry hint.
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

type uncappedFloodWaitKey struct{}

// withUncappedFloodWait marks ctx so the middleware sleeps out every
// server-named delay, maxFloodSleep notwithstanding. Only the startup login
// uses it, because a capped FLOOD_WAIT there fails the start: headless, a
// service manager then restarts straight into the same flood; on stdio the
// client is left with a dead server. A tool call that arrives during an
// uncapped startup sleep waits for the login.
func withUncappedFloodWait(ctx context.Context) context.Context {
	return context.WithValue(ctx, uncappedFloodWaitKey{}, true)
}

func floodWaitUncapped(ctx context.Context) bool {
	uncapped, _ := ctx.Value(uncappedFloodWaitKey{}).(bool)

	return uncapped
}
