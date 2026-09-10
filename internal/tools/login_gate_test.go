package tools

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// pendingAuth is an account that always needs a phone and a code.
type pendingAuth struct{ signedIn bool }

func (p *pendingAuth) Status(context.Context) (*auth.Status, error) {
	return &auth.Status{Authorized: p.signedIn}, nil
}

func (*pendingAuth) SendCode(context.Context, string, auth.SendCodeOptions) (tg.AuthSentCodeClass, error) {
	return &tg.AuthSentCode{PhoneCodeHash: "hash", Type: &tg.AuthSentCodeTypeApp{Length: 5}}, nil
}

func (*pendingAuth) ResendCode(context.Context, string, string) (tg.AuthSentCodeClass, error) {
	return &tg.AuthSentCode{PhoneCodeHash: "hash", Type: &tg.AuthSentCodeTypeApp{Length: 5}}, nil
}

func (p *pendingAuth) SignIn(context.Context, string, string, string) (*tg.AuthAuthorization, error) {
	p.signedIn = true

	return &tg.AuthAuthorization{}, nil
}

func (*pendingAuth) Password(context.Context, string) (*tg.AuthAuthorization, error) {
	return &tg.AuthAuthorization{}, nil
}

func newTestGate() *LoginGate {
	login := telegram.NewLogin(&pendingAuth{}, telegram.Credentials{},
		slog.New(slog.DiscardHandler), func() {})

	return NewLoginGate(login)
}

// callWithVersion builds a tool call carrying a protocol revision and an
// elicitation-capable client in its per-request metadata, which is where the
// gate reads both from.
func callWithVersion(version string, params *mcp.CallToolParamsRaw) *mcp.CallToolRequest {
	return callFrom(version, true, params)
}

// callFrom builds the same call, with elicitation support as a choice.
func callFrom(version string, canElicit bool, params *mcp.CallToolParamsRaw) *mcp.CallToolRequest {
	if params == nil {
		params = &mcp.CallToolParamsRaw{Name: "tg_dialogs_list"}
	}

	meta := map[string]any{mcp.MetaKeyProtocolVersion: version}
	if canElicit {
		meta[mcp.MetaKeyClientCapabilities] = map[string]any{"elicitation": map[string]any{}}
	}

	params.SetMeta(meta)

	return &mcp.CallToolRequest{Params: params}
}

func requestedField(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if result == nil {
		t.Fatal("expected an input request, got none")
	}

	if len(result.InputRequests) != 1 {
		t.Fatalf("expected exactly one input request, got %d", len(result.InputRequests))
	}

	for field := range result.InputRequests {
		return field
	}

	return ""
}

// One step per round trip: the first call asks for the phone and nothing else,
// and says so in RequestState so the answer can be matched to the step.
func TestLoginGate_AsksOneStepPerCall(t *testing.T) {
	gate := newTestGate()

	result, err := gate.Admit(context.Background(), callWithVersion("2026-07-28", nil))
	if err != nil {
		t.Fatalf("admit: %v", err)
	}

	if got := requestedField(t, result); got != string(telegram.LoginStepPhone) {
		t.Errorf("asked for %q, want %q", got, telegram.LoginStepPhone)
	}

	if result.RequestState != string(telegram.LoginStepPhone) {
		t.Errorf("RequestState is %q, want %q", result.RequestState, telegram.LoginStepPhone)
	}

	if len(result.Content) != 0 {
		t.Error("a result carrying input requests must carry no content")
	}
}

// The retry's answer moves the login on: phone answered, code asked for.
func TestLoginGate_ConsumesTheAnswerOnRetry(t *testing.T) {
	gate := newTestGate()
	ctx := context.Background()

	_, err := gate.Admit(ctx, callWithVersion("2026-07-28", nil))
	if err != nil {
		t.Fatalf("first admit: %v", err)
	}

	retry := callWithVersion("2026-07-28", &mcp.CallToolParamsRaw{
		Name:         "tg_dialogs_list",
		RequestState: string(telegram.LoginStepPhone),
		InputResponses: mcp.InputResponseMap{
			string(telegram.LoginStepPhone): &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{string(telegram.LoginStepPhone): "+15550100"},
			},
		},
	})

	result, retryErr := gate.Admit(ctx, retry)
	if retryErr != nil {
		t.Fatalf("retry admit: %v", retryErr)
	}

	if got := requestedField(t, result); got != string(telegram.LoginStepCode) {
		t.Errorf("asked for %q, want %q", got, telegram.LoginStepCode)
	}
}

// Once the login finishes, the gate steps aside and the tool runs.
func TestLoginGate_AdmitsOnceAuthorized(t *testing.T) {
	login := telegram.NewLogin(&pendingAuth{signedIn: true}, telegram.Credentials{},
		slog.New(slog.DiscardHandler), func() {})
	gate := NewLoginGate(login)
	ctx := context.Background()

	err := login.Probe(ctx)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	result, admitErr := gate.Admit(ctx, callWithVersion("2026-07-28", nil))
	if admitErr != nil || result != nil {
		t.Fatalf("an authorized account must admit the call, got result=%v err=%v", result, admitErr)
	}
}

// A declined prompt is an error, not an empty answer that re-asks forever.
func TestLoginGate_DeclineIsAnError(t *testing.T) {
	gate := newTestGate()
	ctx := context.Background()

	_, err := gate.Admit(ctx, callWithVersion("2026-07-28", nil))
	if err != nil {
		t.Fatalf("first admit: %v", err)
	}

	declines := map[string]*mcp.ElicitResult{
		"declined":     {Action: "decline"},
		"cancelled":    {Action: "cancel"},
		"accepted nil": {Action: "accept"},
		"empty":        {Action: "accept", Content: map[string]any{string(telegram.LoginStepPhone): "   "}},
	}

	for name, answer := range declines {
		t.Run(name, func(t *testing.T) {
			retry := callWithVersion("2026-07-28", &mcp.CallToolParamsRaw{
				Name:           "tg_dialogs_list",
				RequestState:   string(telegram.LoginStepPhone),
				InputResponses: mcp.InputResponseMap{string(telegram.LoginStepPhone): answer},
			})

			_, declineErr := gate.Admit(ctx, retry)
			if !errors.Is(declineErr, ErrLoginDeclined) {
				t.Errorf("got %v, want ErrLoginDeclined", declineErr)
			}
		})
	}
}

// The revision decides how the caller is asked, and getting it wrong is not
// symmetric: prompting a client that forbids it fails the call outright. A
// request that names no revision is treated as the newest, which is what the
// SDK itself assumes.
func TestLoginGate_BranchesOnTheProtocolRevision(t *testing.T) {
	cases := map[string]struct {
		version    string
		wantPrompt bool
	}{
		"current":  {version: "2026-07-28", wantPrompt: true},
		"newer":    {version: "2027-01-01", wantPrompt: true},
		"unstated": {version: "", wantPrompt: true},
		"previous": {version: "2025-11-25", wantPrompt: false},
		"oldest":   {version: "2024-11-05", wantPrompt: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gate := newTestGate()

			// No session to prompt through, so the elicitation branch reports
			// that it cannot ask; the input-request branch never needs one.
			result, err := gate.Admit(context.Background(), callWithVersion(tc.version, nil))

			if tc.wantPrompt {
				if err != nil || len(result.InputRequests) == 0 {
					t.Fatalf("expected an input request, got result=%v err=%v", result, err)
				}

				return
			}

			if !errors.Is(err, ErrCannotPrompt) {
				t.Fatalf("expected the elicitation branch, got result=%v err=%v", result, err)
			}
		})
	}
}

// One account logs in once. A caller arriving while another is mid-prompt is
// told to wait rather than opening a second, competing prompt.
func TestLoginGate_RefusesAConcurrentLogin(t *testing.T) {
	gate := newTestGate()
	gate.prompting.Store(true) // another call is prompting right now

	_, err := gate.Admit(context.Background(), callWithVersion("2025-11-25", nil))
	if !errors.Is(err, ErrLoginInProgress) {
		t.Fatalf("got %v, want ErrLoginInProgress", err)
	}
}

// A client that cannot be elicited cannot answer an input request either, so
// both branches say the same thing: this client cannot log you in, use the
// terminal. Left to the SDK the newer branch fails with no remedy named.
func TestLoginGate_CannotPromptWithoutElicitation(t *testing.T) {
	gate := newTestGate()

	_, err := gate.Admit(context.Background(), callFrom("2026-07-28", false, nil))
	if !errors.Is(err, ErrCannotPrompt) {
		t.Fatalf("got %v, want ErrCannotPrompt", err)
	}
}

// A 2FA password may be padded on purpose; a phone or a code never is.
func TestAnswerFor_TrimsEverythingButThePassword(t *testing.T) {
	cases := map[telegram.LoginStep]string{
		telegram.LoginStepPhone:    "+15550100",
		telegram.LoginStepCode:     "12345",
		telegram.LoginStepPassword: "  hunter2  ",
	}

	for step, want := range cases {
		answer := &mcp.ElicitResult{
			Action:  "accept",
			Content: map[string]any{string(step): "  " + want + "  "},
		}

		if step == telegram.LoginStepPassword {
			answer.Content[string(step)] = want
		}

		got, err := answerFor(answer, step)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}

		if got != want {
			t.Errorf("%s: got %q, want %q", step, got, want)
		}
	}
}

// The version-meta tool answers "is the daemon alive, and which build" and
// touches no account, so a pending login must not turn an operator's version
// check into a phone prompt. Every other tool is gated.
func TestAddTool_GatesEveryToolButTheVersionOne(t *testing.T) {
	cases := map[string]struct {
		tool     string
		wantsRun bool
	}{
		"version tool runs":      {tool: ServerVersionToolName, wantsRun: true},
		"telegram tool is gated": {tool: "tg_dialogs_list", wantsRun: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)

			ran := false
			handler := func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
				ran = true

				return &mcp.CallToolResult{}, struct{}{}, nil
			}

			AddTool(server, BoolFieldRegistry{}, newTestGate(), &mcp.Tool{Name: tc.tool}, handler)

			result := callGatedTool(t, server, tc.tool)

			if ran != tc.wantsRun {
				t.Fatalf("handler ran = %v, want %v", ran, tc.wantsRun)
			}

			if tc.wantsRun {
				return
			}

			// The client fulfils the login round trips itself, so what comes
			// back is the outcome: the prompt was refused, so the call failed
			// and the tool never ran.
			if !result.IsError {
				t.Error("a gated tool must fail when the login it asks for is refused")
			}
		})
	}
}

// callGatedTool drives one tool call through a real server, which is the only
// way the registered wrapper is exercised.
func callGatedTool(t *testing.T, server *mcp.Server, tool string) *mcp.CallToolResult {
	t.Helper()

	ctx := context.Background()

	ct, st := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ClientOptions{
		// Refuse every prompt: this test asks which tools are gated at all, and
		// the client resolves the login round trips on its own.
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})

	cs, connErr := client.Connect(ctx, ct, nil)
	if connErr != nil {
		t.Fatalf("client connect: %v", connErr)
	}

	defer func() { _ = cs.Close() }()

	res, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool})
	if callErr != nil {
		t.Fatalf("call tool: %v", callErr)
	}

	return res
}
