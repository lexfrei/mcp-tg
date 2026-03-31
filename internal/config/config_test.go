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
