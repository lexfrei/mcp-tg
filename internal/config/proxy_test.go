package config_test

import (
	"bytes"
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

// Both share-link spellings Telegram hands out, with a hex secret and a
// base64 fake-TLS one.
func TestLoad_MTProxyLinks(t *testing.T) {
	cases := map[string][]byte{
		"tg://proxy?server=proxy.example.org&port=443&secret=dd00ff":       {0xdd, 0x00, 0xff},
		"https://t.me/proxy?server=proxy.example.org&port=443&secret=7gAB": {0xee, 0x00, 0x01},
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
	} {
		_, err := loadWithProxy(t, value)
		if !errors.Is(err, config.ErrInvalidProxy) {
			t.Errorf("%q: err = %v, want ErrInvalidProxy", value, err)
		}
	}
}
