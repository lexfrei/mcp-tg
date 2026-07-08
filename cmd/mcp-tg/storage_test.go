package main

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/session"
	"github.com/lexfrei/keychain"

	"github.com/lexfrei/mcp-tg/internal/config"
)

// fakeStore is an in-memory secretStore for tests. When getErr is set, Get
// returns it (to simulate an unavailable/locked backend).
type fakeStore struct {
	data   map[string][]byte
	getErr error
}

func newFakeStore() *fakeStore { return &fakeStore{data: map[string][]byte{}} }

func (f *fakeStore) Set(service, account string, secret []byte, _ ...keychain.Option) error {
	f.data[service+"/"+account] = secret

	return nil
}

func (f *fakeStore) Get(service, account string) ([]byte, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}

	value, ok := f.data[service+"/"+account]
	if !ok {
		return nil, keychain.ErrNotFound
	}

	return value, nil
}

func TestKeychainStorage_RoundTrip(t *testing.T) {
	store, err := newKeychainStorage(newFakeStore(), "svc", "acct")
	if err != nil {
		t.Fatalf("newKeychainStorage: %v", err)
	}

	if err := store.StoreSession(t.Context(), []byte("session-bytes")); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}

	got, err := store.LoadSession(t.Context())
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}

	if string(got) != "session-bytes" {
		t.Errorf("LoadSession = %q, want %q", got, "session-bytes")
	}
}

func TestKeychainStorage_EmptyReturnsSessionErrNotFound(t *testing.T) {
	store, err := newKeychainStorage(newFakeStore(), "svc", "acct")
	if err != nil {
		t.Fatalf("newKeychainStorage: %v", err)
	}

	_, err = store.LoadSession(t.Context())
	if !errors.Is(err, session.ErrNotFound) {
		t.Errorf("LoadSession on empty keychain = %v, want session.ErrNotFound", err)
	}
}

func TestFreshLoginStorage_StagesUntilCommit(t *testing.T) {
	backing := newFakeStore()
	backing.data["svc/acct"] = []byte("old-working-session")

	inner, err := newKeychainStorage(backing, "svc", "acct")
	if err != nil {
		t.Fatalf("newKeychainStorage: %v", err)
	}

	fresh := &freshLoginStorage{dst: inner}

	// A full login must run even though a session exists.
	if _, loadErr := fresh.LoadSession(t.Context()); !errors.Is(loadErr, session.ErrNotFound) {
		t.Errorf("LoadSession = %v, want session.ErrNotFound (always fresh)", loadErr)
	}

	// gotd persists the new key mid-login (before auth completes); that must NOT
	// touch the backend yet, or a failed login would clobber the old session.
	if storeErr := fresh.StoreSession(t.Context(), []byte("new-session")); storeErr != nil {
		t.Fatalf("StoreSession: %v", storeErr)
	}

	if got := string(backing.data["svc/acct"]); got != "old-working-session" {
		t.Errorf("backend overwritten before commit: %q", got)
	}

	// Commit (only after successful auth) flushes the staged session through.
	if commitErr := fresh.Commit(t.Context()); commitErr != nil {
		t.Fatalf("Commit: %v", commitErr)
	}

	if got := string(backing.data["svc/acct"]); got != "new-session" {
		t.Errorf("backend after commit = %q, want the new session", got)
	}
}

func TestFreshLoginStorage_FailedLoginKeepsOldSession(t *testing.T) {
	backing := newFakeStore()
	backing.data["svc/acct"] = []byte("old-working-session")

	inner, err := newKeychainStorage(backing, "svc", "acct")
	if err != nil {
		t.Fatalf("newKeychainStorage: %v", err)
	}

	fresh := &freshLoginStorage{dst: inner}

	// A failed login: gotd stages a new unauthorized key, but auth fails so the
	// caller never calls Commit. The previous working session must survive.
	if storeErr := fresh.StoreSession(t.Context(), []byte("unauthorized-key")); storeErr != nil {
		t.Fatalf("StoreSession: %v", storeErr)
	}

	if got := string(backing.data["svc/acct"]); got != "old-working-session" {
		t.Errorf("failed login clobbered the session: %q", got)
	}

	// Commit with nothing staged is a no-op and never errors.
	empty := &freshLoginStorage{dst: inner}
	if commitErr := empty.Commit(t.Context()); commitErr != nil {
		t.Errorf("Commit with nothing staged: %v", commitErr)
	}

	if got := string(backing.data["svc/acct"]); got != "old-working-session" {
		t.Errorf("no-op commit changed the backend: %q", got)
	}
}

func TestNewSessionStorage_SecureByDefault(t *testing.T) {
	cfg := &config.Config{SessionFile: "/tmp/mcp-tg/session.json"}

	store, err := newSessionStorage(cfg, false)
	if err != nil {
		// A host with no reachable keychain (headless CI) returns
		// errKeychainUnavailable — acceptable: the secure default never silently
		// falls back to a plaintext file.
		if !errors.Is(err, errKeychainUnavailable) {
			t.Fatalf("unexpected error: %v", err)
		}

		return
	}

	if _, isFile := store.(*session.FileStorage); isFile {
		t.Error("secure default must not use the plaintext FileStorage")
	}
}

func TestNewSessionStorage_InsecureUsesFile(t *testing.T) {
	cfg := &config.Config{SessionFile: "/tmp/mcp-tg/session.json"}

	store, err := newSessionStorage(cfg, true)
	if err != nil {
		t.Fatalf("newSessionStorage(insecure): %v", err)
	}

	if _, ok := store.(*session.FileStorage); !ok {
		t.Errorf("insecure storage = %T, want *session.FileStorage", store)
	}
}

func TestNewKeychainStorage_UnavailableBackendErrors(t *testing.T) {
	failing := &fakeStore{getErr: keychain.ErrUnavailable}

	_, err := newKeychainStorage(failing, "svc", "acct")
	if !errors.Is(err, errKeychainUnavailable) {
		t.Errorf("unavailable backend = %v, want errKeychainUnavailable", err)
	}
}

func TestNewKeychainStorage_NotFoundMeansAvailable(t *testing.T) {
	store, err := newKeychainStorage(newFakeStore(), "svc", "acct")
	if err != nil {
		t.Fatalf("ErrNotFound means keychain works (no session yet), got: %v", err)
	}

	if _, ok := store.(*keychainStorage); !ok {
		t.Errorf("storage = %T, want *keychainStorage", store)
	}
}
