package main

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/session"
	"github.com/lexfrei/keychain"

	"github.com/lexfrei/mcp-tg/internal/config"
)

// keychainService is the service name under which the Telegram session is
// stored in the OS keychain. The per-session account key is the configured
// session-file path, so TELEGRAM_SESSION_FILE still selects which session even
// in keychain mode.
const keychainService = "mcp-tg"

// errKeychainUnavailable is returned when secure storage was requested but no OS
// keychain backend is reachable (no Secret Service in a container or headless
// Linux, a locked collection, or the macOS framework failed to load). It names
// the explicit opt-out.
var errKeychainUnavailable = errors.New(
	"secure keychain storage unavailable — pass --insecure-storage (login) or " +
		"set TELEGRAM_SESSION_INSECURE=true (server) to use a plaintext session file",
)

// secretStore is the subset of *keychain.Keychain used here, injectable so the
// storage is testable without a real OS keychain.
type secretStore interface {
	Set(service, account string, secret []byte, opts ...keychain.Option) error
	Get(service, account string) ([]byte, error)
}

// keychainStorage implements gotd's session.Storage on top of the OS keychain
// (macOS Keychain, Linux/*BSD Secret Service, Windows Credential Manager) via
// github.com/lexfrei/keychain.
type keychainStorage struct {
	store   secretStore
	service string
	account string
}

var _ session.Storage = (*keychainStorage)(nil)

func (k *keychainStorage) LoadSession(_ context.Context) ([]byte, error) {
	data, err := k.store.Get(k.service, k.account)
	if errors.Is(err, keychain.ErrNotFound) {
		return nil, session.ErrNotFound
	}

	if err != nil {
		return nil, errors.Wrap(err, "keychain load")
	}

	return data, nil
}

func (k *keychainStorage) StoreSession(_ context.Context, data []byte) error {
	return errors.Wrap(k.store.Set(k.service, k.account, data), "keychain store")
}

// newKeychainStorage builds a keychain-backed session store and probes the
// backend once, so an unreachable or locked keychain fails fast with actionable
// guidance rather than at first use. A missing item (ErrNotFound) means the
// backend works but no session exists yet — the normal pre-login state.
func newKeychainStorage(store secretStore, service, account string) (session.Storage, error) {
	if _, err := store.Get(service, account); err != nil && !errors.Is(err, keychain.ErrNotFound) {
		return nil, errors.Wrap(errKeychainUnavailable, err.Error())
	}

	return &keychainStorage{store: store, service: service, account: account}, nil
}

// newSessionStorage picks the session backend: the OS keychain by default, or a
// plaintext file only when insecure storage was explicitly requested.
//
// WithSecurityCLI keeps the macOS item in the stable apple-tool access
// partition, so an unsigned, frequently rebuilt daemon keeps reading what an
// earlier build wrote. It is a no-op on Linux and Windows, where secrets are
// user-scoped and already rebuild-stable.
func newSessionStorage(cfg *config.Config, insecure bool) (session.Storage, error) {
	if insecure {
		return &session.FileStorage{Path: cfg.SessionFile}, nil
	}

	store := keychain.New(keychain.WithSecurityCLI())

	return newKeychainStorage(store, keychainService, cfg.SessionFile)
}
