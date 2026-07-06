package middleware

import "sync/atomic"

// SessionHealth tracks whether the Telegram session's auth key is still valid.
//
// The gotd invoker middleware, which sees every MTProto API error, marks the
// session revoked the moment Telegram answers with an auth-key error; the MCP
// session guard reads that flag to fast-fail tool calls with one clear message
// instead of forwarding a raw AUTH_KEY_UNREGISTERED per call. A revoked session
// cannot recover on its own in headless mode — only a fresh interactive login
// restores it — so the flag is one-way (healthy → revoked) for the process
// lifetime; a successful re-login happens in a freshly started process.
//
// Safe for concurrent use.
type SessionHealth struct {
	armed   atomic.Bool
	claimed atomic.Bool
	revoked atomic.Bool
	code    atomic.Pointer[string]
}

// NewSessionHealth returns a healthy, unarmed SessionHealth.
func NewSessionHealth() *SessionHealth {
	return &SessionHealth{}
}

// Arm enables revocation tracking. Call it once the initial authentication has
// succeeded. Before that, the startup Auth().IfNecessary probe (users.getUsers
// on self) answers AUTH_KEY_UNREGISTERED for a not-yet-authorized or absent
// session, and that expected pre-login 401 must not be mistaken for a
// revocation — otherwise a successful interactive login would still leave every
// tool blocked. MarkRevoked is a no-op until Arm is called.
func (health *SessionHealth) Arm() {
	health.armed.Store(true)
}

// MarkRevoked records that an armed session was revoked by the given MTProto
// error code. It reports whether this call performed the healthy→revoked
// transition, which is true for exactly one caller, so that caller can log the
// event once. It is a no-op (returns false) before Arm: pre-auth probe 401s
// must not poison the session. The code from the winning transition is
// retained; later codes do not overwrite it.
func (health *SessionHealth) MarkRevoked(code string) bool {
	if !health.armed.Load() {
		return false
	}

	// claimed picks the single winner; the winner stores the code and only then
	// publishes revoked. So any reader that observes Revoked()==true is
	// guaranteed to also see Code() — sync/atomic operations are sequentially
	// consistent, so code.Store happens-before revoked.Store is visible.
	if !health.claimed.CompareAndSwap(false, true) {
		return false
	}

	captured := code
	health.code.Store(&captured)
	health.revoked.Store(true)

	return true
}

// Revoked reports whether the session has been marked revoked.
func (health *SessionHealth) Revoked() bool {
	return health.revoked.Load()
}

// Code returns the MTProto error code that revoked the session, or "" while the
// session is still healthy.
func (health *SessionHealth) Code() string {
	if captured := health.code.Load(); captured != nil {
		return *captured
	}

	return ""
}
