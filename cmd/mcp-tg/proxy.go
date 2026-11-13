package main

import (
	"context"
	"net"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/dcs"
	"golang.org/x/net/proxy"

	"github.com/lexfrei/mcp-tg/internal/config"
)

// errSOCKSNoContext is returned if x/net's SOCKS5 dialer ever stops
// implementing proxy.ContextDialer: gotd dials with a context, and dropping
// it would make connection attempts uncancellable.
var errSOCKSNoContext = errors.New("the SOCKS5 dialer does not support contexts")

// newResolver returns the gotd resolver that routes every Telegram
// connection through the configured proxy, or nil to keep gotd's default.
func newResolver(settings *config.Proxy) (dcs.Resolver, error) {
	if settings == nil {
		return nil, nil //nolint:nilnil // nil is gotd's "use the default resolver".
	}

	if settings.Kind == config.ProxyMTProxy {
		resolver, err := dcs.MTProxy(settings.Addr, settings.Secret, dcs.MTProxyOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "configuring MTProxy")
		}

		return resolver, nil
	}

	dial, err := socks5Dialer(settings)
	if err != nil {
		return nil, err
	}

	return dcs.Plain(dcs.PlainOptions{Dial: dial}), nil
}

func socks5Dialer(settings *config.Proxy) (dcs.DialFunc, error) {
	var auth *proxy.Auth
	if settings.User != "" {
		auth = &proxy.Auth{User: settings.User, Password: settings.Password}
	}

	dialer, err := proxy.SOCKS5("tcp", settings.Addr, auth, &net.Dialer{})
	if err != nil {
		return nil, errors.Wrap(err, "configuring SOCKS5 proxy")
	}

	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errSOCKSNoContext
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, dialErr := contextDialer.DialContext(ctx, network, addr)
		if dialErr != nil {
			return nil, errors.Wrap(dialErr, "dialing through SOCKS5 proxy")
		}

		return conn, nil
	}, nil
}
