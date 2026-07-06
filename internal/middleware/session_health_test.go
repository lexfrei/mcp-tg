package middleware

import (
	"sync"
	"testing"
)

func TestSessionHealth_StartsHealthy(t *testing.T) {
	health := NewSessionHealth()

	if health.Revoked() {
		t.Error("new SessionHealth should not be revoked")
	}

	if code := health.Code(); code != "" {
		t.Errorf("new SessionHealth code = %q, want empty", code)
	}
}

func TestSessionHealth_MarkRevokedIgnoredBeforeArm(t *testing.T) {
	health := NewSessionHealth()

	// The startup IfNecessary probe answers AUTH_KEY_UNREGISTERED for a
	// not-yet-authorized session; before Arm that must not revoke anything.
	if health.MarkRevoked("AUTH_KEY_UNREGISTERED") {
		t.Error("MarkRevoked before Arm should report false (no-op)")
	}

	if health.Revoked() {
		t.Error("session must stay healthy until Arm is called")
	}

	health.Arm()

	if !health.MarkRevoked("AUTH_KEY_UNREGISTERED") {
		t.Error("MarkRevoked after Arm should report the transition")
	}

	if !health.Revoked() {
		t.Error("session must be revoked after Arm + MarkRevoked")
	}
}

func TestSessionHealth_MarkRevokedReportsFirstTransitionOnly(t *testing.T) {
	health := NewSessionHealth()
	health.Arm()

	if !health.MarkRevoked("AUTH_KEY_UNREGISTERED") {
		t.Fatal("first MarkRevoked should report the healthy→revoked transition (true)")
	}

	if health.MarkRevoked("SESSION_REVOKED") {
		t.Error("second MarkRevoked should report false (already revoked)")
	}

	if !health.Revoked() {
		t.Error("Revoked() should be true after MarkRevoked")
	}

	// The code from the first transition is retained; later codes do not overwrite it.
	if code := health.Code(); code != "AUTH_KEY_UNREGISTERED" {
		t.Errorf("Code() = %q, want %q (first code wins)", code, "AUTH_KEY_UNREGISTERED")
	}
}

func TestSessionHealth_MarkRevokedConcurrentSingleWinner(t *testing.T) {
	health := NewSessionHealth()
	health.Arm()

	const goroutines = 32

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		wins  int
		start = make(chan struct{})
	)

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			<-start

			if health.MarkRevoked("AUTH_KEY_UNREGISTERED") {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}

	close(start)
	wg.Wait()

	if wins != 1 {
		t.Errorf("exactly one goroutine should win the transition, got %d", wins)
	}

	// The winner publishes revoked only after storing the code, so once the
	// session reads revoked it must also expose a non-empty code — never the
	// transient Revoked()==true / Code()=="" window.
	if health.Revoked() && health.Code() == "" {
		t.Error("Revoked() is true but Code() is empty — code must be published before revoked")
	}
}
