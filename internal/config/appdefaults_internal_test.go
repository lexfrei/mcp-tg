package config

import "testing"

func TestLoadAppID_FallsBackToLdflagsDefault(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "")

	prev := defaultAppID
	defaultAppID = "77777"
	t.Cleanup(func() { defaultAppID = prev })

	id, err := loadAppID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != 77777 {
		t.Errorf("loadAppID() = %d, want 77777 (ldflags default)", id)
	}
}

func TestLoadAppID_EnvOverridesLdflagsDefault(t *testing.T) {
	t.Setenv("TELEGRAM_APP_ID", "12345")

	prev := defaultAppID
	defaultAppID = "77777"
	t.Cleanup(func() { defaultAppID = prev })

	id, err := loadAppID()
	if err != nil || id != 12345 {
		t.Errorf("loadAppID() = %d, %v; want 12345 (env overrides ldflags default)", id, err)
	}
}

func TestLoadAppHash_FallsBackToLdflagsDefault(t *testing.T) {
	t.Setenv("TELEGRAM_APP_HASH", "")

	prev := defaultAppHash
	defaultAppHash = "builtinhash"
	t.Cleanup(func() { defaultAppHash = prev })

	hash, err := loadAppHash()
	if err != nil || hash != "builtinhash" {
		t.Errorf("loadAppHash() = %q, %v; want builtinhash (ldflags default)", hash, err)
	}
}
