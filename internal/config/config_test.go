package config_test

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/config"
)

func TestLoad_MissingAppID(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "")
	t.Setenv("TELEGRAM_APP_HASH", "test")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing TELEGRAM_APP_ID")
	}
}

func TestLoad_InvalidAppID(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "notanumber")
	t.Setenv("TELEGRAM_APP_HASH", "test")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid TELEGRAM_APP_ID")
	}
}

func TestLoad_NegativeAppID(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "-1")
	t.Setenv("TELEGRAM_APP_HASH", "test")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for negative TELEGRAM_APP_ID")
	}
}

func TestLoad_MissingAppHash(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for missing TELEGRAM_APP_HASH")
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("TELEGRAM_PHONE", "+1234567890")
	t.Setenv("TELEGRAM_PASSWORD", "secret")
	t.Setenv("MCP_HTTP_PORT", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.AppID != 12345 {
		t.Errorf("AppID = %d, want 12345", cfg.AppID)
	}

	if cfg.AppHash != "testhash" {
		t.Errorf("AppHash = %q, want %q", cfg.AppHash, "testhash")
	}

	if cfg.Phone != "+1234567890" {
		t.Errorf("Phone = %q, want %q", cfg.Phone, "+1234567890")
	}

	if !cfg.HasPassword() {
		t.Error("HasPassword() = false, want true")
	}
}

func TestLoad_InvalidHTTPPort(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "99999")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid MCP_HTTP_PORT")
	}
}

func TestHTTPEnabled(t *testing.T) {
	cfg := &config.Config{HTTPPort: "8080", HTTPHost: "0.0.0.0"}
	if !cfg.HTTPEnabled() {
		t.Error("HTTPEnabled() = false, want true")
	}

	if cfg.HTTPAddr() != "0.0.0.0:8080" {
		t.Errorf("HTTPAddr() = %q, want %q", cfg.HTTPAddr(), "0.0.0.0:8080")
	}
}

func TestHTTPDisabled(t *testing.T) {
	cfg := &config.Config{}
	if cfg.HTTPEnabled() {
		t.Error("HTTPEnabled() = true, want false")
	}
}

func TestLoad_HTTPOnlyDefaultsFalse(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "8787")
	t.Setenv("MCP_HTTP_ONLY", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.HTTPOnly {
		t.Error("HTTPOnly = true, want false when MCP_HTTP_ONLY is unset")
	}
}

func TestLoad_HTTPOnlyWithPort(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "8787")
	t.Setenv("MCP_HTTP_ONLY", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.HTTPOnly {
		t.Error("HTTPOnly = false, want true when MCP_HTTP_ONLY=true")
	}
}

func TestLoad_HTTPOnlyWithoutPortFails(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "")
	t.Setenv("MCP_HTTP_ONLY", "true")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error: HTTP-only mode requires MCP_HTTP_PORT")
	}
}

func TestLoad_HTTPOnlyInvalidValueFails(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "8787")
	t.Setenv("MCP_HTTP_ONLY", "maybe")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid MCP_HTTP_ONLY value")
	}
}

// TestLoadForLogin_IgnoresTransportEnv pins that the login path is not blocked
// by server-only transport settings: an env that fails Load() (HTTP-only with
// no port) must still load cleanly for login, since login never starts HTTP.
func TestLoadForLogin_IgnoresTransportEnv(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_ONLY", "true")
	t.Setenv("MCP_HTTP_PORT", "")

	_, loadErr := config.Load()
	if loadErr == nil {
		t.Fatal("precondition: Load() must reject HTTP-only without a port")
	}

	cfg, err := config.LoadForLogin()
	if err != nil {
		t.Fatalf("LoadForLogin must ignore transport env, got: %v", err)
	}

	if cfg.AppID != 12345 || cfg.AppHash != "testhash" {
		t.Errorf("LoadForLogin credentials = %d/%q, want 12345/testhash", cfg.AppID, cfg.AppHash)
	}
}

// TestLoadForLogin_InvalidHTTPPortIgnored pins that even a malformed
// MCP_HTTP_PORT (which Load() rejects) does not block the login path.
func TestLoadForLogin_InvalidHTTPPortIgnored(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("MCP_HTTP_PORT", "not-a-port")

	_, loadErr := config.Load()
	if loadErr == nil {
		t.Fatal("precondition: Load() must reject an invalid MCP_HTTP_PORT")
	}

	_, err := config.LoadForLogin()
	if err != nil {
		t.Fatalf("LoadForLogin must ignore an invalid MCP_HTTP_PORT, got: %v", err)
	}
}

func TestHasPassword(t *testing.T) {
	cfg := &config.Config{}
	if cfg.HasPassword() {
		t.Error("HasPassword() = true, want false")
	}

	cfg.Password = "test"
	if !cfg.HasPassword() {
		t.Error("HasPassword() = false, want true")
	}
}

func TestLoad_InsecureStorageDefaultsFalse(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("TELEGRAM_SESSION_INSECURE", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.InsecureStorage {
		t.Error("InsecureStorage = true, want false when TELEGRAM_SESSION_INSECURE is unset")
	}
}

func TestLoad_InsecureStorageTrue(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("TELEGRAM_SESSION_INSECURE", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.InsecureStorage {
		t.Error("InsecureStorage = false, want true when TELEGRAM_SESSION_INSECURE=true")
	}
}

func TestLoad_InsecureStorageInvalidFails(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "testhash")
	t.Setenv("TELEGRAM_SESSION_INSECURE", "maybe")

	_, err := config.Load()
	if err == nil {
		t.Fatal("expected error for invalid TELEGRAM_SESSION_INSECURE value")
	}
}
