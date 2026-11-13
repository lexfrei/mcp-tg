package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
)

// newConnectionLossMiddleware resends a query once when the connection to
// Telegram closed under it after the server had acknowledged it. gotd leaves
// that case to the caller because the server may already have run the query.
// Here that is safe for the same reason the server-error resend is: a read
// lands on the same answer, a send carries a random_id Telegram deduplicates
// against, and the creating requests safeToResend holds back are held back
// here too, marked so the tools layer asks the caller to look before repeating.
//
// No delay before the resend. The client replaces a closed connection on its
// own, and an invoke that lands on the closed engine comes back as
// rpc.ErrEngineClosed, which gotd parks until the replacement is up — the
// resend simply rides that.
//
// One resend, not a loop: a connection that dies twice under one query is a
// network that is not coming back fast enough for an agent blocked on the
// call. The second loss surfaces marked, so it reads as the connection's
// doing rather than the caller's.
func newConnectionLossMiddleware(logger *slog.Logger) telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			err := next.Invoke(ctx, input, output)
			if err == nil || !tgclient.LostInFlight(ctx, err) {
				return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
			}

			if !safeToResend(input) {
				return errors.Mark(errors.Mark(err, tgclient.ErrLostInFlight), tgclient.ErrNotResent)
			}

			logger.Warn("Telegram connection closed under an acknowledged query — resending once",
				"request", requestName(input))

			err = next.Invoke(ctx, input, output)
			if err == nil || !tgclient.LostInFlight(ctx, err) {
				return err //nolint:wrapcheck // pass-through: the resend's own answer is Telegram's verdict.
			}

			return errors.Mark(err, tgclient.ErrLostInFlight)
		}
	}
}

// typeNamer is the method every generated schema type carries; bin.Object does
// not promise it, so it is asserted separately.
type typeNamer interface {
	TypeName() string
}

// requestName renders a query the way the API docs name it (messages.getHistory),
// which is what a post-mortem greps for; the Go type is the fallback for an
// encoder that is not a schema object.
func requestName(input bin.Encoder) string {
	if named, isNamed := input.(typeNamer); isNamed {
		return named.TypeName()
	}

	return fmt.Sprintf("%T", input)
}
