package main

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/cockroachdb/errors"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/lexfrei/mcp-tg/internal/middleware"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// scriptedAuth is a Telegram account that is not logged in and accepts one
// phone and one code, unless statusErr makes it unreachable.
type scriptedAuth struct {
	authorized bool
	statusErr  error
}

func (s *scriptedAuth) Status(context.Context) (*auth.Status, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}

	return &auth.Status{Authorized: s.authorized}, nil
}

func (*scriptedAuth) SendCode(context.Context, string, auth.SendCodeOptions) (tg.AuthSentCodeClass, error) {
	return &tg.AuthSentCode{PhoneCodeHash: "hash", Type: &tg.AuthSentCodeTypeApp{Length: 5}}, nil
}

func (*scriptedAuth) ResendCode(context.Context, string, string) (tg.AuthSentCodeClass, error) {
	return &tg.AuthSentCode{PhoneCodeHash: "hash", Type: &tg.AuthSentCodeTypeApp{Length: 5}}, nil
}

func (s *scriptedAuth) SignIn(_ context.Context, _, _, _ string) (*tg.AuthAuthorization, error) {
	s.authorized = true

	return &tg.AuthAuthorization{}, nil
}

func (*scriptedAuth) Password(context.Context, string) (*tg.AuthAuthorization, error) {
	return &tg.AuthAuthorization{}, nil
}

// TestToolCall_LogsInOnTheCurrentProtocolRevision is the regression pin for a
// server that could not log in at all on the newest protocol revision.
//
// The login used to start from notifications/initialized, which the
// 2026-07-28 handshake does not send: the server waited forever and answered
// every tool call with "still authenticating", saved session or not. Nothing
// caught it, because no test drove a real client against the real server.
//
// This drives the input-request half of the gate: the call comes back asking
// for the phone, the client answers and retries, and so on until the tool
// runs. The elicitation half cannot be reached from here — the SDK gives no
// way to hold a client to an older revision, since server/discover always
// succeeds and marks the session initialized — so the branch itself is pinned
// in internal/tools instead.
func TestToolCall_LogsInOnTheCurrentProtocolRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	authDone := make(chan struct{})
	health := middleware.NewSessionHealth()
	login := tgclient.NewLogin(&scriptedAuth{}, tgclient.Credentials{},
		slog.New(slog.DiscardHandler), func() {
			health.Arm()
			close(authDone)
		})

	server := buildServer(
		testutil.NoopClient{}, "/tmp/mcp-tg/downloads", nil,
		tgclient.NewSubscriptionBroker(), authDone, health, login, discardLogger(),
	)

	ct, st := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ClientOptions{
		ElicitationHandler: answerLoginPrompt,
	})

	cs, connErr := client.Connect(ctx, ct, nil)
	if connErr != nil {
		t.Fatalf("client connect: %v", connErr)
	}

	defer func() { _ = cs.Close() }()

	// The revision that broke the old design: no initialize, no initialized.
	if got := cs.InitializeResult().ProtocolVersion; got != "2026-07-28" {
		t.Fatalf("negotiated %q, want 2026-07-28", got)
	}

	select {
	case <-authDone:
		t.Fatal("the login cannot be finished before any client asks for it")
	default:
	}

	res, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: "tg_dialogs_list"})
	if callErr != nil {
		t.Fatalf("call tool: %v", callErr)
	}

	if res.IsError {
		for _, content := range res.Content {
			if text, ok := content.(*mcp.TextContent); ok {
				t.Fatalf("tool returned an error result: %s", text.Text)
			}
		}

		t.Fatalf("tool returned an error result: %v", res.Content)
	}

	select {
	case <-authDone:
	default:
		t.Error("the login must have completed by the time the tool ran")
	}
}

// settleLogin decides whether the process starts at all, so each of its three
// answers is pinned. Swapping any two would let a server refuse to start on
// every machine without a saved session, or start believing it is logged in.
func TestSettleLogin_DecidesWhetherTheServerStarts(t *testing.T) {
	sessionError := errors.New("dial tcp: i/o timeout")

	cases := map[string]struct {
		api       *scriptedAuth
		env       tgclient.Credentials
		wantStart bool
		wantDone  bool
	}{
		"saved session": {
			api:       &scriptedAuth{authorized: true},
			wantStart: true,
			wantDone:  true,
		},
		"credentials in the environment": {
			api:       &scriptedAuth{},
			env:       tgclient.Credentials{Phone: "+15550100", Code: "12345"},
			wantStart: true,
			wantDone:  true,
		},
		"nothing configured": {
			// Not a failure: a client can still supply the credentials on the
			// first tool call, which is the whole point of the lazy login.
			api:       &scriptedAuth{},
			wantStart: true,
			wantDone:  false,
		},
		"telegram unreachable": {
			// Says nothing about the session, so it is not "please log in" —
			// it is a startup failure and must surface as one.
			api:       &scriptedAuth{statusErr: sessionError},
			wantStart: false,
			wantDone:  false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			login := tgclient.NewLogin(tc.api, tc.env, slog.New(slog.DiscardHandler), func() {})

			err := settleLogin(context.Background(), login, discardLogger())

			if tc.wantStart && err != nil {
				t.Fatalf("the server must start, got %v", err)
			}

			if !tc.wantStart {
				if err == nil {
					t.Fatal("the server must not start")
				}

				if !errors.Is(err, sessionError) {
					t.Errorf("the real cause must survive, got %v", err)
				}
			}

			if login.Done() != tc.wantDone {
				t.Errorf("login done = %v, want %v", login.Done(), tc.wantDone)
			}
		})
	}
}

// answerLoginPrompt answers whichever single field the prompt asks for. The
// gate names the field after the login step, so the schema identifies it.
func answerLoginPrompt(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	answers := map[string]string{
		string(tgclient.LoginStepPhone):    "+15550100",
		string(tgclient.LoginStepCode):     "12345",
		string(tgclient.LoginStepPassword): "hunter2",
	}

	for _, field := range promptFields(req.Params.RequestedSchema) {
		value, known := answers[field]
		if !known {
			continue
		}

		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{field: value}}, nil
	}

	return &mcp.ElicitResult{Action: "decline"}, nil
}

// promptFields lists the schema's property names. The schema crosses the wire
// as plain JSON, so it arrives as a map rather than the type the server built.
func promptFields(schema any) []string {
	switch s := schema.(type) {
	case *jsonschema.Schema:
		return slices.Collect(maps.Keys(s.Properties))
	case map[string]any:
		props, ok := s["properties"].(map[string]any)
		if !ok {
			return nil
		}

		return slices.Collect(maps.Keys(props))
	default:
		return nil
	}
}
