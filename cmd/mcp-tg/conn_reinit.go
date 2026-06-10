package main

import (
	"context"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// newConnReinitMiddleware recovers a connection whose initConnection state the
// server has forgotten. Telegram then answers every request with
// CONNECTION_LAYER_INVALID (or CONNECTION_NOT_INITED), and gotd's regular
// invoke path never re-inits — the connection stays broken until the process
// restarts. Mirror the recovery gotd implements for CDN connections only:
// retry the failed query once, wrapped in initConnection so the server
// re-learns the connection state (gotd adds the outer invokeWithLayer to
// every request already).
//
// The device parameters must match what the client sends on connect; the
// client is built without a custom telegram.Options.Device, so the gotd
// defaults replicated here stay in sync by construction.
func newConnReinitMiddleware(appID int) telegram.MiddlewareFunc {
	var device telegram.DeviceConfig
	device.SetDefaults()

	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			err := next.Invoke(ctx, input, output)
			if !tgerr.Is(err, "CONNECTION_LAYER_INVALID", "CONNECTION_NOT_INITED") {
				return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
			}

			query, isObject := input.(bin.Object)
			if !isObject {
				return err //nolint:wrapcheck // cannot wrap a non-Object query into initConnection.
			}

			return next.Invoke(ctx, &tg.InitConnectionRequest{
				APIID:          appID,
				DeviceModel:    device.DeviceModel,
				SystemVersion:  device.SystemVersion,
				AppVersion:     device.AppVersion,
				SystemLangCode: device.SystemLangCode,
				LangPack:       device.LangPack,
				LangCode:       device.LangCode,
				Query:          query,
			}, output)
		}
	}
}
