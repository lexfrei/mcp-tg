package config

import (
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
)

// Proxy kinds.
const (
	ProxySOCKS5  = "socks5"
	ProxyMTProxy = "mtproxy"
)

// ErrInvalidProxy is returned when TELEGRAM_PROXY cannot be parsed.
var ErrInvalidProxy = errors.New(
	"TELEGRAM_PROXY must be socks5://[user:password@]host:port, " +
		"or an MTProxy link: tg://proxy?server=HOST&port=PORT&secret=SECRET (t.me/proxy?... works too)",
)

// Proxy is the proxy every connection to Telegram goes through.
type Proxy struct {
	Kind     string
	Addr     string
	User     string
	Password string
	Secret   []byte
}

// loadProxy reads TELEGRAM_PROXY. It belongs to the core settings because the
// login subcommand reaches Telegram too, and on a network that needs a proxy
// it would otherwise never get to ask for the code.
func loadProxy() (*Proxy, error) {
	return parseProxy(os.Getenv("TELEGRAM_PROXY"))
}

func parseProxy(raw string) (*Proxy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil //nolint:nilnil // no proxy is the documented default.
	}

	link, err := url.Parse(raw)
	if err != nil {
		return nil, errors.Wrap(ErrInvalidProxy, err.Error())
	}

	switch {
	case link.Scheme == ProxySOCKS5:
		return parseSOCKS5(link)
	case link.Scheme == "tg" && link.Host == "proxy",
		(link.Scheme == "https" || link.Scheme == "http") && link.Host == "t.me" && link.Path == "/proxy":
		return parseMTProxy(link.Query())
	default:
		return nil, ErrInvalidProxy
	}
}

func parseSOCKS5(link *url.URL) (*Proxy, error) {
	if link.Port() == "" || link.Hostname() == "" {
		return nil, errors.Wrap(ErrInvalidProxy, "socks5 proxy needs host and port")
	}

	proxy := &Proxy{Kind: ProxySOCKS5, Addr: link.Host}

	if link.User != nil {
		proxy.User = link.User.Username()
		proxy.Password, _ = link.User.Password()
	}

	return proxy, nil
}

func parseMTProxy(query url.Values) (*Proxy, error) {
	server, port, secret := query.Get("server"), query.Get("port"), query.Get("secret")
	if server == "" || port == "" || secret == "" {
		return nil, errors.Wrap(ErrInvalidProxy, "an MTProxy link needs server, port and secret")
	}

	decoded, err := decodeSecret(secret)
	if err != nil {
		return nil, err
	}

	return &Proxy{Kind: ProxyMTProxy, Addr: net.JoinHostPort(server, port), Secret: decoded}, nil
}

// decodeSecret accepts the two spellings MTProxy links use: hex, and the
// URL-safe base64 of the "ee" fake-TLS secrets.
func decodeSecret(secret string) ([]byte, error) {
	decoded, err := hex.DecodeString(secret)
	if err == nil {
		return decoded, nil
	}

	decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(secret, "="))
	if err == nil {
		return decoded, nil
	}

	return nil, errors.Wrap(ErrInvalidProxy, "the secret is neither hex nor base64")
}
