package middleware

import (
	"sync"
	"sync/atomic"
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

	const (
		writers = 32
		readers = 8
	)

	var (
		writeGroup sync.WaitGroup
		readGroup  sync.WaitGroup
		mu         sync.Mutex
		wins       int
		violation  atomic.Bool
		stop       atomic.Bool
		start      = make(chan struct{})
	)

	// Readers spin during the race and assert the invariant in the actual
	// concurrent window: if Revoked() is ever observed true, Code() must already
	// be non-empty (the winner stores the code before publishing revoked). A
	// post-hoc check after Wait() cannot catch the transient window this rules out.
	readGroup.Add(readers)

	for range readers {
		go func() {
			defer readGroup.Done()

			<-start

			for !stop.Load() {
				if health.Revoked() && health.Code() == "" {
					violation.Store(true)

					return
				}
			}
		}()
	}

	writeGroup.Add(writers)

	for range writers {
		go func() {
			defer writeGroup.Done()

			<-start

			if health.MarkRevoked("AUTH_KEY_UNREGISTERED") {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}

	close(start)
	writeGroup.Wait()
	stop.Store(true)
	readGroup.Wait()

	if wins != 1 {
		t.Errorf("exactly one goroutine should win the transition, got %d", wins)
	}

	if violation.Load() {
		t.Error("observed Revoked()==true with empty Code() during the race — code must be published before revoked")
	}

	if health.Revoked() && health.Code() == "" {
		t.Error("Revoked() is true but Code() is empty — code must be published before revoked")
	}
}
