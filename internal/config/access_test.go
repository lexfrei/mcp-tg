package config_test

import (
	"slices"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/lexfrei/mcp-tg/internal/config"
)

func setValidCredentials(t *testing.T) {
	t.Helper()

	t.Setenv("TELEGRAM_APP_ID", "12345")
	t.Setenv("TELEGRAM_APP_HASH", "test")
}

func TestLoad_AccessDefaultsImposeNothing(t *testing.T) {
	setValidCredentials(t)
	t.Setenv("TELEGRAM_ALLOWED_PEERS", "")
	t.Setenv("TELEGRAM_READ_ONLY", "")
	t.Setenv("TELEGRAM_TOOLS", "")
	t.Setenv("TELEGRAM_DISABLED_TOOLS", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	access := cfg.Access
	if access.ReadOnly || access.AllowedPeers != nil || access.Tools != nil || access.DisabledTools != nil {
		t.Errorf("Access = %+v, want the zero value", access)
	}
}

func TestLoad_AccessListsAreSplitAndTrimmed(t *testing.T) {
	setValidCredentials(t)
	t.Setenv("TELEGRAM_ALLOWED_PEERS", " @durov , -1001234567890,,me_friend ")
	t.Setenv("TELEGRAM_TOOLS", "tg_messages_list,tg_messages_send")
	t.Setenv("TELEGRAM_DISABLED_TOOLS", "tg_messages_delete")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPeers := []string{"@durov", "-1001234567890", "me_friend"}
	if !slices.Equal(cfg.Access.AllowedPeers, wantPeers) {
		t.Errorf("AllowedPeers = %q, want %q", cfg.Access.AllowedPeers, wantPeers)
	}

	if !slices.Equal(cfg.Access.Tools, []string{"tg_messages_list", "tg_messages_send"}) {
		t.Errorf("Tools = %q", cfg.Access.Tools)
	}

	if !slices.Equal(cfg.Access.DisabledTools, []string{"tg_messages_delete"}) {
		t.Errorf("DisabledTools = %q", cfg.Access.DisabledTools)
	}
}

// A list that is set but holds only separators is an operator asking for a
// boundary and getting none; it must fail rather than read as "unset".
func TestLoad_AccessListWithNoEntriesFails(t *testing.T) {
	for _, name := range []string{"TELEGRAM_ALLOWED_PEERS", "TELEGRAM_TOOLS", "TELEGRAM_DISABLED_TOOLS"} {
		t.Run(name, func(t *testing.T) {
			setValidCredentials(t)
			t.Setenv(name, " , ,")

			_, err := config.Load()
			if !errors.Is(err, config.ErrAccessListEmpty) {
				t.Fatalf("err = %v, want ErrAccessListEmpty", err)
			}
		})
	}
}

func TestLoad_ReadOnly(t *testing.T) {
	setValidCredentials(t)
	t.Setenv("TELEGRAM_READ_ONLY", "true")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Access.ReadOnly {
		t.Error("ReadOnly = false, want true")
	}
}

func TestLoad_ReadOnlyInvalidFails(t *testing.T) {
	setValidCredentials(t)
	t.Setenv("TELEGRAM_READ_ONLY", "sometimes")

	_, err := config.Load()
	if !errors.Is(err, config.ErrInvalidReadOnly) {
		t.Fatalf("err = %v, want ErrInvalidReadOnly", err)
	}
}
