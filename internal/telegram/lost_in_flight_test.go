package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
)

// gotd reports a connection closed under an acknowledged query as the plain
// cancellation of its own context. The one thing that tells it apart from the
// caller cancelling is that the caller's context is still live.
func TestLostInFlight_CancellationBelowALiveCaller(t *testing.T) {
	err := errors.Wrap(errors.Wrap(context.Canceled, "engine forcibly closed"), "rpcDoRequest")

	if !LostInFlight(context.Background(), err) {
		t.Error("a cancellation under a live caller must classify as a lost query")
	}
}

func TestLostInFlight_CallerCancellationIsNot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if LostInFlight(ctx, errors.Wrap(context.Canceled, "rpcDoRequest")) {
		t.Error("a caller's own cancellation must not classify as a lost query")
	}
}

func TestLostInFlight_OtherErrorsAreNot(t *testing.T) {
	for name, err := range map[string]error{
		"nil":               nil,
		"rpc error":         tgerr.New(500, "RPC_CALL_FAIL"),
		"deadline exceeded": errors.Wrap(context.DeadlineExceeded, "rpcDoRequest"),
		"unrelated":         errors.New("read: connection reset by peer"),
	} {
		t.Run(name, func(t *testing.T) {
			if LostInFlight(context.Background(), err) {
				t.Errorf("%v must not classify as a lost query", err)
			}
		})
	}
}
