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
