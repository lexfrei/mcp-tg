package main

import (
	"bytes"
	"context"
	"sync"

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

// freshLoginStorage makes `mcp-tg login` authenticate from scratch while
// protecting any existing working session from a failed attempt.
//
// LoadSession always reports no session, so gotd's auth flow runs in full and
// never reuses a stale or revoked key — some revoked states (notably
// AUTH_KEY_DUPLICATED) break a connection that reuses the key and would fail the
// login before it could prompt.
//
// StoreSession does NOT write through. gotd persists the freshly minted MTProto
// auth key as soon as the connection handshake completes — before the
// interactive code/2FA step — so writing it straight to the backend would
// replace the previous, still-authorized session with an unauthorized key the
// moment login is attempted; a wrong code or an abort would then leave the user
// logged out. Instead the session is staged in memory and only flushed by
// Commit, which the caller invokes after Auth().IfNecessary succeeds.
type freshLoginStorage struct {
	dst session.Storage

	// mu guards staged/dirty: gotd may call StoreSession from a background
	// goroutine (e.g. a salt refresh) while the caller runs Commit.
	mu     sync.Mutex
	staged []byte
	dirty  bool
}

var _ session.Storage = (*freshLoginStorage)(nil)

func (*freshLoginStorage) LoadSession(context.Context) ([]byte, error) {
	return nil, session.ErrNotFound
}

func (fresh *freshLoginStorage) StoreSession(_ context.Context, data []byte) error {
	// Keep the latest bytes only; the real backend is untouched until Commit.
	fresh.mu.Lock()
	defer fresh.mu.Unlock()

	fresh.staged = bytes.Clone(data)
	fresh.dirty = true

	return nil
}

// Commit flushes the staged session to the real backend. The caller must invoke
// it only after authentication has succeeded, so that a failed or aborted login
// leaves the previous session intact. It is a no-op if nothing was staged.
func (fresh *freshLoginStorage) Commit(ctx context.Context) error {
	fresh.mu.Lock()
	staged, dirty := fresh.staged, fresh.dirty
	fresh.mu.Unlock()

	if !dirty {
		return nil
	}

	// staged is an owned bytes.Clone; StoreSession replaces the field with a new
	// slice rather than mutating this one, so using the snapshot after unlock is
	// safe and the backend write does not hold the lock.
	return errors.Wrap(fresh.dst.StoreSession(ctx, staged), "store fresh session")
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
