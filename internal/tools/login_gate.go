package tools

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/cockroachdb/errors"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// multiRoundTripVersion is the MCP revision that removed server-initiated
// elicitation in favour of input requests returned from the handler.
const multiRoundTripVersion = "2026-07-28"

// ErrLoginInProgress is returned on the elicitation branch to a caller that
// arrives while another call is already prompting, since that branch holds the
// prompt open for the length of the call. The input-request branch needs no
// such claim: it answers immediately, and a stale step is dropped when it
// comes back.
var ErrLoginInProgress = errors.New("a telegram login is already in progress in another call, retry once it finishes")

// ErrLoginDeclined is returned when the client refuses a login prompt or
// answers it with nothing.
var ErrLoginDeclined = errors.New("the telegram login prompt was declined or left empty")

// ErrCannotPrompt is returned when the client cannot be asked for credentials
// at all — it declared no elicitation capability, or the protocol forbids the
// server asking.
var ErrCannotPrompt = errors.New(
	"cannot prompt for the telegram login through this client — " +
		"run `mcp-tg login` in a terminal, then restart the server")

// LoginGate runs the Telegram login on the first tool call that needs it.
//
// It lives inside the tool handler rather than in a middleware because only
// the tool dispatch can produce an "input required" result: the SDK stamps
// that on results returned from callTool, deeper than any middleware, and the
// field that says so is unexported.
type LoginGate struct {
	login     *telegram.Login
	prompting atomic.Bool
}

// NewLoginGate returns nil when login is nil, so a server built without one
// (every registration test) wraps no handlers at all.
func NewLoginGate(login *telegram.Login) *LoginGate {
	if login == nil {
		return nil
	}

	return &LoginGate{login: login}
}

// Admit reports whether the call may proceed. A nil result and a nil error
// mean the account is authorized and the tool should run; a non-nil result
// carries the next login prompt back to the client.
func (gate *LoginGate) Admit(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if gate.login.Done() {
		return nil, nil //nolint:nilnil // nil result with nil error means "run the tool" by contract.
	}

	if supportsMultiRoundTrip(req) {
		return gate.admitByInputRequest(ctx, req)
	}

	return gate.admitByElicitation(ctx, req)
}

// admitByInputRequest asks for one step per round trip: the client answers and
// retries the whole tool call, which lands here again with the answer.
func (gate *LoginGate) admitByInputRequest(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// The client fulfils input requests by elicitation, so one that cannot be
	// elicited cannot answer. Saying so here keeps both branches giving the
	// same answer; left to the SDK it surfaces as a round-trip failure that
	// names no remedy.
	if caps := req.ClientCapabilities(); caps == nil || caps.Elicitation == nil {
		return nil, ErrCannotPrompt
	}

	step := telegram.LoginStep(req.Params.RequestState)

	answer, err := answerFor(req.Params.InputResponses[string(step)], step)
	if err != nil {
		return nil, err
	}

	prompt, err := gate.login.Next(ctx, step, answer)
	if err != nil {
		return nil, err //nolint:wrapcheck // the flow's errors already say what went wrong.
	}

	if prompt == nil {
		return nil, nil //nolint:nilnil // authorized: run the tool.
	}

	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{string(prompt.Step): elicitParams(prompt)},
		RequestState:  string(prompt.Step),
	}, nil
}

// admitByElicitation drives the whole login inside this one call, which is
// what clients before 2026-07-28 support.
func (gate *LoginGate) admitByElicitation(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Claimed before anything else, so a second caller is told what is actually
	// happening rather than why this one cannot ask.
	if !gate.prompting.CompareAndSwap(false, true) {
		return nil, ErrLoginInProgress
	}
	defer gate.prompting.Store(false)

	if req.Session == nil {
		return nil, ErrCannotPrompt
	}

	answer := ""

	for step := telegram.LoginStep(""); ; {
		prompt, err := gate.login.Next(ctx, step, answer)
		if err != nil {
			return nil, err //nolint:wrapcheck // the flow's errors already say what went wrong.
		}

		if prompt == nil {
			return nil, nil //nolint:nilnil // authorized: run the tool.
		}

		result, err := req.Session.Elicit(ctx, elicitParams(prompt))
		if err != nil {
			// A dead context says nothing about the client; telling its owner to
			// go run a terminal login would be advice for a problem they do not
			// have. Everything else means the asking itself failed.
			if ctx.Err() != nil {
				return nil, errors.Wrap(err, "asking for the telegram login")
			}

			// Wrapped, not just marked: Mark keeps the cause's message, and the
			// caller needs the way out, not only the SDK's "does not support".
			//nolint:wrapcheck // Mark adds the sentinel; Wrap supplies the remedy.
			return nil, errors.Mark(errors.Wrap(err, ErrCannotPrompt.Error()), ErrCannotPrompt)
		}

		answer, err = answerFor(result, prompt.Step)
		if err != nil {
			return nil, err
		}

		step = prompt.Step
	}
}

// supportsMultiRoundTrip decides how the caller can be asked. An unstated
// version counts as the newest, which is what the SDK assumes when a request
// carries no initialize params. Reading it the other way would have the server
// elicit at a client that refuses to be elicited, which fails the call
// outright; guessing the other direction merely falls into the SDK's shim.
func supportsMultiRoundTrip(req *mcp.CallToolRequest) bool {
	version := req.ProtocolVersion()
	if version == "" {
		return true
	}

	return version >= multiRoundTripVersion
}

// answerFor reads one elicitation answer. Anything other than an accepted,
// non-empty string is a decline: the SDK lets an "accept" through with no
// content at all, and a re-prompt on empty input would loop against a client
// that answers instantly.
func answerFor(response mcp.InputResponse, step telegram.LoginStep) (string, error) {
	if response == nil {
		return "", nil
	}

	result, ok := response.(*mcp.ElicitResult)
	if !ok || result.Action != "accept" || result.Content == nil {
		return "", ErrLoginDeclined
	}

	value, ok := result.Content[string(step)].(string)
	if !ok {
		return "", ErrLoginDeclined
	}

	// A 2FA password may legitimately be padded; a phone or code never is.
	if step != telegram.LoginStepPassword {
		value = strings.TrimSpace(value)
	}

	if strings.TrimSpace(value) == "" {
		return "", ErrLoginDeclined
	}

	return value, nil
}

// elicitParams asks for one string under the step's own name.
func elicitParams(prompt *telegram.Prompt) *mcp.ElicitParams {
	field := string(prompt.Step)

	return &mcp.ElicitParams{
		Message: prompt.Message,
		RequestedSchema: &jsonschema.Schema{
			Type:     "object",
			Required: []string{field},
			Properties: map[string]*jsonschema.Schema{
				field: {Type: "string", Description: prompt.Message},
			},
		},
	}
}
