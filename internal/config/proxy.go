package config

import (
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/mtproxy"
)

// Proxy kinds.
const (
	ProxySOCKS5  = "socks5"
	ProxyMTProxy = "mtproxy"
)

// ErrInvalidProxy is returned when TELEGRAM_PROXY cannot be parsed.
var ErrInvalidProxy = errors.New(
	"TELEGRAM_PROXY must be socks5://[user:password@]host:port, " +
		"or an MTProxy link: tg://proxy?server=HOST&port=PORT&secret=SECRET " +
		"(https://t.me/proxy?... works too)",
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

	link, err := parseURL(raw)
	if err != nil {
		return nil, err
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

// parseURL parses the value without letting it into the error. url.Parse
// quotes the whole raw value in its error, a SOCKS5 password included, and
// main logs startup errors where the service managers keep them. Its own
// reason is not safe either: a "/", "?" or "#" in a password ends the
// authority early, and the reason then quotes the password as a port. So the
// error carries a fixed text only.
func parseURL(raw string) (*url.URL, error) {
	link, err := url.Parse(raw)
	if err != nil {
		return nil, errors.Wrap(ErrInvalidProxy, "the value is not a valid URL")
	}

	return link, nil
}

func parseSOCKS5(link *url.URL) (*Proxy, error) {
	if link.Port() == "" || link.Hostname() == "" {
		return nil, errors.Wrap(ErrInvalidProxy, "socks5 proxy needs host and port")
	}

	if hasTrailer(link) {
		return nil, errors.Wrap(ErrInvalidProxy, "a socks5 proxy takes no path, query or fragment; "+
			"percent-encode /, ? and # in the password")
	}

	err := validatePort(link.Port())
	if err != nil {
		return nil, err
	}

	proxy := &Proxy{Kind: ProxySOCKS5, Addr: link.Host}

	if link.User != nil {
		proxy.User = link.User.Username()
		proxy.Password, _ = link.User.Password()
	}

	// The dialer sends credentials only with a user, so a lone password would
	// be dropped and the connection would go out without authentication.
	if proxy.User == "" && proxy.Password != "" {
		return nil, errors.Wrap(ErrInvalidProxy, "a socks5 password needs a user")
	}

	return proxy, nil
}

// hasTrailer reports anything after host:port. A "/" in a password ends the
// authority early, so a digit-only password would load as the proxy port with
// the rest left in the path; a SOCKS5 link has nothing there.
func hasTrailer(link *url.URL) bool {
	return (link.Path != "" && link.Path != "/") || link.RawQuery != "" || link.ForceQuery || link.Fragment != ""
}

func parseMTProxy(query url.Values) (*Proxy, error) {
	server, port, secret := query.Get("server"), query.Get("port"), query.Get("secret")
	if server == "" || port == "" || secret == "" {
		return nil, errors.Wrap(ErrInvalidProxy, "an MTProxy link needs server, port and secret")
	}

	err := validatePort(port)
	if err != nil {
		return nil, err
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
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(secret, "="))
	}

	if err == nil {
		return decoded, checkSecret(decoded)
	}

	return nil, errors.Wrap(ErrInvalidProxy, "the secret is neither hex nor base64")
}

// checkSecret runs the secret through gotd's own parser, so a secret gotd
// would refuse fails here rather than at the first dial. gotd's error quotes
// the secret, so only the reason is kept.
func checkSecret(secret []byte) error {
	_, err := mtproxy.ParseSecret(secret)
	if err != nil {
		return errors.Wrap(ErrInvalidProxy, "the secret is not a valid MTProxy secret (16 bytes, optionally tagged)")
	}

	return nil
}

func validatePort(port string) error {
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > maxPort {
		return errors.Wrap(ErrInvalidProxy, "the port must be a number from 1 to 65535")
	}

	return nil
}
