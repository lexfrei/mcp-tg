package telegram

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// fakeLoginAPI scripts the gotd auth client. Each method records its call and
// returns whatever the test loaded; a nil func means "not expected".
type fakeLoginAPI struct {
	status   func() (*auth.Status, error)
	sendCode func(phone string) (tg.AuthSentCodeClass, error)
	resend   func(phone, hash string) (tg.AuthSentCodeClass, error)
	signIn   func(phone, code, hash string) (*tg.AuthAuthorization, error)
	password func(password string) (*tg.AuthAuthorization, error)
	calls    []string
}

func (f *fakeLoginAPI) Status(context.Context) (*auth.Status, error) {
	f.calls = append(f.calls, "status")
	if f.status == nil {
		return &auth.Status{}, nil
	}

	return f.status()
}

func (f *fakeLoginAPI) SendCode(_ context.Context, phone string, _ auth.SendCodeOptions) (tg.AuthSentCodeClass, error) {
	f.calls = append(f.calls, "sendCode:"+phone)

	return f.sendCode(phone)
}

func (f *fakeLoginAPI) ResendCode(_ context.Context, phone, hash string) (tg.AuthSentCodeClass, error) {
	f.calls = append(f.calls, "resend:"+hash)

	return f.resend(phone, hash)
}

func (f *fakeLoginAPI) SignIn(_ context.Context, phone, code, hash string) (*tg.AuthAuthorization, error) {
	f.calls = append(f.calls, "signIn:"+code+"@"+hash)

	return f.signIn(phone, code, hash)
}

func (f *fakeLoginAPI) Password(_ context.Context, password string) (*tg.AuthAuthorization, error) {
	f.calls = append(f.calls, "password:"+password)

	return f.password(password)
}

func sentCode(hash string) (tg.AuthSentCodeClass, error) {
	return &tg.AuthSentCode{PhoneCodeHash: hash, Type: &tg.AuthSentCodeTypeApp{Length: 5}}, nil
}

func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newTestLogin(api *fakeLoginAPI, env Credentials) (*Login, *int) {
	doneCalls := 0
	login := NewLogin(api, env, quietLogger(), func() { doneCalls++ })

	return login, &doneCalls
}

func mustPrompt(t *testing.T, p *Prompt, err error, step LoginStep) *Prompt {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p == nil {
		t.Fatalf("expected a prompt for %q, got authorized", step)
	}

	if p.Step != step {
		t.Fatalf("expected prompt for %q, got %q (%s)", step, p.Step, p.Message)
	}

	return p
}

func mustDone(t *testing.T, login *Login, p *Prompt, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if p != nil {
		t.Fatalf("expected authorized, got prompt for %q", p.Step)
	}

	if !login.Done() {
		t.Fatal("Done() must report true once the flow finished")
	}
}

// The saved-session case: Status answers authorized, nothing else is called,
// and the completion hook fires exactly once.
func TestLogin_ProbeAuthorizedFinishes(t *testing.T) {
	api := &fakeLoginAPI{status: func() (*auth.Status, error) {
		return &auth.Status{Authorized: true, User: &tg.User{ID: 7}}, nil
	}}
	login, doneCalls := newTestLogin(api, Credentials{})

	err := login.Probe(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}

	if !login.Done() || *doneCalls != 1 {
		t.Fatalf("expected done with one completion call, got done=%v calls=%d", login.Done(), *doneCalls)
	}

	err = login.Probe(context.Background())
	if err != nil || *doneCalls != 1 {
		t.Fatalf("a second probe must be a no-op, got err=%v calls=%d", err, *doneCalls)
	}
}

// A non-401 error from Status says nothing about the session. It must neither
// finish nor mark the flow as needing a login — the phone step re-probes.
func TestLogin_ProbeTransientErrorStaysPending(t *testing.T) {
	probes := 0
	api := &fakeLoginAPI{status: func() (*auth.Status, error) {
		probes++
		if probes == 1 {
			return nil, errors.New("dial tcp: i/o timeout")
		}

		return &auth.Status{Authorized: true}, nil
	}}
	login, _ := newTestLogin(api, Credentials{})

	err := login.Probe(context.Background())
	if err == nil {
		t.Fatal("a transient Status error must be reported")
	}

	if login.Done() {
		t.Fatal("a transient error must not count as authorized")
	}

	// The first step re-probes and finds the session valid after all.
	p, err := login.Next(context.Background(), "", "")

	mustDone(t, login, p, err)

	for _, c := range api.calls {
		if c[:4] == "send" {
			t.Fatalf("no code must be sent when the re-probe finds a session, calls: %v", api.calls)
		}
	}
}

// Fully configured through the environment: no prompt at all, in order.
func TestLogin_EnvCredentialsNeverPrompt(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(_, code, hash string) (*tg.AuthAuthorization, error) {
			if code != "12345" || hash != "h1" {
				t.Fatalf("SignIn got code=%q hash=%q", code, hash)
			}

			return nil, auth.ErrPasswordAuthNeeded
		},
		password: func(p string) (*tg.AuthAuthorization, error) {
			if p != "s3cret " {
				t.Fatalf("password must reach the API verbatim, got %q", p)
			}

			return &tg.AuthAuthorization{}, nil
		},
	}
	login, doneCalls := newTestLogin(api, Credentials{Phone: "+15550100", Code: "12345", Password: "s3cret "})

	p, err := login.Next(context.Background(), "", "")
	mustDone(t, login, p, err)

	if *doneCalls != 1 {
		t.Fatalf("expected one completion call, got %d", *doneCalls)
	}
}

// A one-time code from the environment is tried once. When Telegram rejects
// it the flow asks a human instead of looping on the same stale value.
func TestLogin_EnvCodeIsUsedOnce(t *testing.T) {
	signIns := 0
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(_, code, _ string) (*tg.AuthAuthorization, error) {
			signIns++
			if code == "stale" {
				return nil, tgerr.New(400, "PHONE_CODE_INVALID")
			}

			return &tg.AuthAuthorization{}, nil
		},
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100", Code: "stale"})

	p, err := login.Next(context.Background(), "", "")
	mustPrompt(t, p, err, LoginStepCode)

	p, err = login.Next(context.Background(), LoginStepCode, "")
	mustPrompt(t, p, err, LoginStepCode)

	if signIns != 1 {
		t.Fatalf("the stale env code must be tried exactly once, SignIn was called %d times", signIns)
	}

	p, err = login.Next(context.Background(), LoginStepCode, "fresh")
	mustDone(t, login, p, err)
}

// Interactive path with 2FA: three prompts in Flow.Run's order.
func TestLogin_PhoneCodePasswordOrder(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn:   func(string, string, string) (*tg.AuthAuthorization, error) { return nil, auth.ErrPasswordAuthNeeded },
		password: func(string) (*tg.AuthAuthorization, error) { return &tg.AuthAuthorization{}, nil },
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	p, err := login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepPhone)

	p, err = login.Next(ctx, LoginStepPhone, " +15550100 ")
	mustPrompt(t, p, err, LoginStepCode)

	p, err = login.Next(ctx, LoginStepCode, "12345")
	mustPrompt(t, p, err, LoginStepPassword)

	p, err = login.Next(ctx, LoginStepPassword, "pw")
	mustDone(t, login, p, err)

	want := []string{"status", "sendCode:+15550100", "signIn:12345@h1", "password:pw"}
	if len(api.calls) != len(want) {
		t.Fatalf("calls: got %v want %v", api.calls, want)
	}

	for i := range want {
		if api.calls[i] != want[i] {
			t.Fatalf("call %d: got %q want %q (all: %v)", i, api.calls[i], want[i], api.calls)
		}
	}
}

// A wrong code re-asks the same step with the same hash; nothing is resent.
func TestLogin_WrongCodeReasksWithoutResend(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(_, code, _ string) (*tg.AuthAuthorization, error) {
			if code != "ok" {
				return nil, tgerr.New(400, "PHONE_CODE_INVALID")
			}

			return &tg.AuthAuthorization{}, nil
		},
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100"})
	ctx := context.Background()

	p, err := login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepCode)

	p, err = login.Next(ctx, LoginStepCode, "bad")
	p = mustPrompt(t, p, err, LoginStepCode)

	if p.Message == promptMessage(LoginStepCode, "") {
		t.Fatal("the re-ask must say why")
	}

	p, err = login.Next(ctx, LoginStepCode, "ok")
	mustDone(t, login, p, err)

	for _, c := range api.calls {
		if c == "sendCode:+15550100" {
			continue
		}

		if len(c) > 6 && c[:6] == "resend" {
			t.Fatalf("a wrong code must not trigger a resend, calls: %v", api.calls)
		}
	}
}

// An expired code is resent with the stored hash; the phone is not asked again.
func TestLogin_ExpiredCodeResends(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		resend:   func(_, hash string) (tg.AuthSentCodeClass, error) { return sentCode(hash + "-2") },
		signIn: func(_, _, hash string) (*tg.AuthAuthorization, error) {
			if hash == "h1" {
				return nil, tgerr.New(400, "PHONE_CODE_EXPIRED")
			}

			return &tg.AuthAuthorization{}, nil
		},
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100"})
	ctx := context.Background()

	p, err := login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepCode)

	p, err = login.Next(ctx, LoginStepCode, "old")
	mustPrompt(t, p, err, LoginStepCode)

	p, err = login.Next(ctx, LoginStepCode, "new")
	mustDone(t, login, p, err)

	if api.calls[len(api.calls)-1] != "signIn:new@h1-2" {
		t.Fatalf("the second sign-in must use the resent hash, calls: %v", api.calls)
	}
}

// Three rejected attempts reset the flow to the phone with an error, so an
// agent retrying in a loop cannot walk into Telegram's flood limits.
func TestLogin_ThreeFailuresResetToPhone(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(string, string, string) (*tg.AuthAuthorization, error) {
			return nil, tgerr.New(400, "PHONE_CODE_INVALID")
		},
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	_, _ = login.Next(ctx, "", "")
	_, _ = login.Next(ctx, LoginStepPhone, "+15550100")

	var (
		p   *Prompt
		err error
	)

	for range 3 {
		p, err = login.Next(ctx, LoginStepCode, "bad")
	}

	if !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("expected ErrTooManyLoginAttempts on the third failure, got prompt=%v err=%v", p, err)
	}

	p, err = login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepPhone)
}

// A response tagged with a step the flow already left is stale: it is not
// applied, and the caller gets the current step back.
func TestLogin_StaleStepReturnsCurrentPrompt(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	_, _ = login.Next(ctx, LoginStepPhone, "+15550100")

	p, err := login.Next(ctx, LoginStepPhone, "+15550199")
	mustPrompt(t, p, err, LoginStepCode)

	if len(api.calls) != 2 {
		t.Fatalf("a stale phone answer must not send a second code, calls: %v", api.calls)
	}
}

func TestLogin_SignUpRequiredResetsToPhone(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(string, string, string) (*tg.AuthAuthorization, error) {
			return nil, errors.Wrap(&auth.SignUpRequired{}, "check")
		},
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100"})
	ctx := context.Background()

	_, _ = login.Next(ctx, "", "")

	_, err := login.Next(ctx, LoginStepCode, "12345")
	if !errors.Is(err, ErrSignUpNotSupported) {
		t.Fatalf("expected ErrSignUpNotSupported, got %v", err)
	}

	p, err := login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepPhone)
}

// SendCode can answer with a ready authorization; no code step then.
func TestLogin_SentCodeSuccessShortCircuits(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) {
			return &tg.AuthSentCodeSuccess{Authorization: &tg.AuthAuthorization{}}, nil
		},
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100"})

	p, err := login.Next(context.Background(), "", "")
	mustDone(t, login, p, err)
}

// The same success can instead carry "this phone has no account". Latching
// that as a login is worse than refusing it: the revocation guard would be
// armed, and the first real call would be reported as a revoked session rather
// than a missing account.
func TestLogin_SentCodeSuccessCanRequireSignUp(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) {
			return &tg.AuthSentCodeSuccess{Authorization: &tg.AuthAuthorizationSignUpRequired{}}, nil
		},
	}
	login, doneCalls := newTestLogin(api, Credentials{Phone: "+15550100"})

	_, err := login.Next(context.Background(), "", "")
	if !errors.Is(err, ErrSignUpNotSupported) {
		t.Fatalf("expected ErrSignUpNotSupported, got %v", err)
	}

	if login.Done() || *doneCalls != 0 {
		t.Fatalf("a sign-up requirement is not a login, got done=%v calls=%d", login.Done(), *doneCalls)
	}
}

func TestLogin_AuthRestartResets(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(string, string, string) (*tg.AuthAuthorization, error) {
			return nil, tgerr.New(500, "AUTH_RESTART")
		},
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	_, _ = login.Next(ctx, LoginStepPhone, "+15550100")

	p, err := login.Next(ctx, LoginStepCode, "12345")
	mustPrompt(t, p, err, LoginStepPhone)
}

// A restart is an attempt too. Without counting it, a server answering
// AUTH_RESTART forever would keep the elicitation branch asking forever: that
// branch loops inside a single call, so nothing else bounds it.
func TestLogin_RepeatedAuthRestartGivesUp(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(string, string, string) (*tg.AuthAuthorization, error) {
			return nil, tgerr.New(500, "AUTH_RESTART")
		},
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	var err error

	for range maxLoginAttempts {
		_, _ = login.Next(ctx, LoginStepPhone, "+15550100")
		_, err = login.Next(ctx, LoginStepCode, "12345")
	}

	if !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("expected the flow to give up, got %v", err)
	}
}

// Giving up starts the flow over, so the next caller gets its own three tries
// rather than inheriting a spent budget. Without clearing the counter, one
// restart after a give-up would refuse immediately and keep refusing for the
// life of the process.
func TestLogin_GivingUpClearsTheAttemptBudget(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
		signIn: func(string, string, string) (*tg.AuthAuthorization, error) {
			return nil, tgerr.New(500, "AUTH_RESTART")
		},
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	var err error

	for range maxLoginAttempts {
		_, _ = login.Next(ctx, LoginStepPhone, "+15550100")
		_, err = login.Next(ctx, LoginStepCode, "12345")
	}

	if !errors.Is(err, ErrTooManyLoginAttempts) {
		t.Fatalf("setup: expected the flow to give up, got %v", err)
	}

	_, _ = login.Next(ctx, LoginStepPhone, "+15550100")

	p, err := login.Next(ctx, LoginStepCode, "12345")
	mustPrompt(t, p, err, LoginStepPhone)
}

// Unattended = no human anywhere: the flow goes as far as the environment
// carries it and then reports which input is missing.
func TestLogin_RunUnattendedStopsAtFirstMissingInput(t *testing.T) {
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return sentCode("h1") },
	}
	login, _ := newTestLogin(api, Credentials{Phone: "+15550100"})

	err := login.RunUnattended(context.Background())
	if !errors.Is(err, ErrLoginInputRequired) {
		t.Fatalf("expected ErrLoginInputRequired, got %v", err)
	}

	if login.Done() {
		t.Fatal("must not be done without the code")
	}

	if api.calls[len(api.calls)-1] != "sendCode:+15550100" {
		t.Fatalf("the phone from the environment must have been used, calls: %v", api.calls)
	}
}

// Whatever is not the flow's business (a rate limit, a network error) comes
// back untouched and leaves the state where it was.
func TestLogin_UnknownErrorLeavesStateAlone(t *testing.T) {
	flood := tgerr.New(420, "FLOOD_WAIT_30")
	api := &fakeLoginAPI{
		sendCode: func(string) (tg.AuthSentCodeClass, error) { return nil, flood },
	}
	login, _ := newTestLogin(api, Credentials{})
	ctx := context.Background()

	_, err := login.Next(ctx, LoginStepPhone, "+15550100")
	if !errors.Is(err, flood) {
		t.Fatalf("expected the flood error to pass through, got %v", err)
	}

	p, err := login.Next(ctx, "", "")
	mustPrompt(t, p, err, LoginStepPhone)
}
