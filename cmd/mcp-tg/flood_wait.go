package main

import (
	"context"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const maxFloodRetries = 3

func newFloodWaitMiddleware() telegram.MiddlewareFunc {
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

				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			return nil
		}
	}
}
