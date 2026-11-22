package config_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/lexfrei/mcp-tg/internal/config"
)

func loadWithProxy(t *testing.T, value string) (*config.Config, error) {
	t.Helper()

	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "test")
	t.Setenv("TELEGRAM_PROXY", value)

	cfg, err := config.Load()
	if err != nil {
		return nil, errors.Wrap(err, "loading config")
	}

	return cfg, nil
}

func TestLoad_NoProxyByDefault(t *testing.T) {
	cfg, err := loadWithProxy(t, "")
	if err != nil || cfg.Proxy != nil {
		t.Fatalf("cfg.Proxy = %+v, err = %v; want no proxy", cfg.Proxy, err)
	}
}

func TestLoad_SOCKS5Proxy(t *testing.T) {
	cfg, err := loadWithProxy(t, "socks5://alice:s3cret@127.0.0.1:1080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	proxy := cfg.Proxy
	if proxy.Kind != config.ProxySOCKS5 || proxy.Addr != "127.0.0.1:1080" || proxy.User != "alice" || proxy.Password != "s3cret" {
		t.Errorf("proxy = %+v", proxy)
	}
}

// Real-length secrets: a "dd" secured one (tag + 16 bytes) and an "ee"
// fake-TLS one (tag + 16 bytes + the cloak host).
var (
	securedSecret = append([]byte{0xdd}, bytes.Repeat([]byte{0x11}, 16)...)
	fakeTLSSecret = append(append([]byte{0xee}, bytes.Repeat([]byte{0x22}, 16)...), "example.com"...)
)

// Both share-link spellings Telegram hands out, with a hex secret and a
// base64 fake-TLS one.
func TestLoad_MTProxyLinks(t *testing.T) {
	cases := map[string][]byte{
		"tg://proxy?server=proxy.example.org&port=443&secret=" + hex.EncodeToString(securedSecret): securedSecret,
		"https://t.me/proxy?server=proxy.example.org&port=443&secret=" +
			base64.RawURLEncoding.EncodeToString(fakeTLSSecret): fakeTLSSecret,
	}

	for link, secret := range cases {
		cfg, err := loadWithProxy(t, link)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", link, err)

			continue
		}

		if cfg.Proxy.Kind != config.ProxyMTProxy || cfg.Proxy.Addr != "proxy.example.org:443" ||
			!bytes.Equal(cfg.Proxy.Secret, secret) {
			t.Errorf("%s: proxy = %+v", link, cfg.Proxy)
		}
	}
}

// LoadForLogin must see the proxy as well: on a network that needs one, the
// login could not reach Telegram to send the code otherwise.
func TestLoadForLogin_ReadsTheProxy(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "test")
	t.Setenv("TELEGRAM_PROXY", "socks5://127.0.0.1:1080")

	cfg, err := config.LoadForLogin()
	if err != nil || cfg.Proxy == nil {
		t.Fatalf("cfg.Proxy = %+v, err = %v", cfg.Proxy, err)
	}
}

func TestLoad_InvalidProxyFails(t *testing.T) {
	for _, value := range []string{
		"http://127.0.0.1:8080",
		"socks5://127.0.0.1",
		"tg://proxy?server=x&port=443",
		"tg://proxy?server=x&port=443&secret=!!!",
		"socks5://127.0.0.1:0",
		"socks5://127.0.0.1:99999",
		"tg://proxy?server=x&port=abc&secret=" + hex.EncodeToString(securedSecret),
		"tg://proxy?server=x&port=65536&secret=" + hex.EncodeToString(securedSecret),
		"t.me/proxy?server=x&port=443&secret=" + hex.EncodeToString(securedSecret),
	} {
		_, err := loadWithProxy(t, value)
		if !errors.Is(err, config.ErrInvalidProxy) {
			t.Errorf("%q: err = %v, want ErrInvalidProxy", value, err)
		}
	}
}

// A value that fails to parse must not put its credentials into the error:
// main logs the error, and the service managers keep that log.
func TestLoad_InvalidProxyKeepsSecretsOutOfTheError(t *testing.T) {
	cases := []struct {
		value  string
		secret string
	}{
		// url.Parse quotes the whole raw input, userinfo included.
		{"socks5://alice:hunter2@host:10x80", "hunter2"},
		{"socks5://alice:hunter2%zz@host:1080", "hunter2"},
		// A secret too short for MTProxy: gotd quotes it in its own error.
		{"tg://proxy?server=x&port=443&secret=ab12", "ab12"},
		{"tg://proxy?server=x&port=443&secret=ab12", string([]byte{0xab, 0x12})},
		{"tg://proxy?server=x&port=443&secret=ab12", `\xab\x12`},
		// A fake-TLS-length secret with a tag gotd does not know.
		{"tg://proxy?server=x&port=443&secret=ff" + strings.Repeat("33", 16) + "41", "333333"},
	}

	for _, tc := range cases {
		_, err := loadWithProxy(t, tc.value)
		if !errors.Is(err, config.ErrInvalidProxy) {
			t.Errorf("%q: err = %v, want ErrInvalidProxy", tc.value, err)

			continue
		}

		if strings.Contains(err.Error(), tc.secret) {
			t.Errorf("%q: the error carries the secret %q: %v", tc.value, tc.secret, err)
		}
	}
}

// The hint must name only the spellings that load.
func TestErrInvalidProxy_NamesOnlyWorkingSpellings(t *testing.T) {
	text := config.ErrInvalidProxy.Error()
	if strings.Contains(strings.ReplaceAll(text, "https://t.me/proxy", ""), "t.me/proxy") {
		t.Errorf("hint names a scheme-less t.me/proxy link, which does not load: %s", text)
	}
}
