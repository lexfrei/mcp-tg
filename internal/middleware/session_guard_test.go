package middleware

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSessionGuard_AllowsWhenHealthy(t *testing.T) {
	health := NewSessionHealth()
	handler := NewSessionGuard(health, nil)(noopResult)

	_, err := handler(context.Background(), "tools/call", nil)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (handler must run while session is healthy)", err)
	}
}

func TestSessionGuard_BlocksToolCallWhenRevoked(t *testing.T) {
	health := NewSessionHealth()
	health.Arm()
	health.MarkRevoked("AUTH_KEY_UNREGISTERED")

	handler := NewSessionGuard(health, nil)(noopResult)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: nonBypassToolName}}

	_, err := handler(context.Background(), "tools/call", req)
	if !errors.Is(err, ErrSessionRevoked) {
		t.Errorf("got error %v, want ErrSessionRevoked", err)
	}
}

func TestSessionGuard_BypassedToolReachesHandlerWhenRevoked(t *testing.T) {
	health := NewSessionHealth()
	health.Arm()
	health.MarkRevoked("AUTH_KEY_UNREGISTERED")

	handler := NewSessionGuard(health, []string{bypassToolName})(noopResult)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: bypassToolName}}

	_, err := handler(context.Background(), "tools/call", req)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (bypassed tool must reach handler even when revoked)", err)
	}
}

func TestSessionGuard_AllowsProtocolAndListWhenRevoked(t *testing.T) {
	health := NewSessionHealth()
	health.Arm()
	health.MarkRevoked("AUTH_KEY_UNREGISTERED")

	handler := NewSessionGuard(health, nil)(noopResult)

	for _, method := range []string{"initialize", "tools/list", "resources/list", "prompts/list"} {
		_, err := handler(context.Background(), method, nil)
		if !errors.Is(err, errNoop) {
			t.Errorf("method %q: got error %v, want errNoop (must pass through)", method, err)
		}
	}
}
