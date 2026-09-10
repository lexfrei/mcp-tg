package telegram

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// LoginStep names the input a login is waiting for.
type LoginStep string

// The three inputs Telegram can ask a user for, in Flow.Run's order.
const (
	LoginStepPhone    LoginStep = "phone"
	LoginStepCode     LoginStep = "code"
	LoginStepPassword LoginStep = "password"
)

// Prompt is a request for one input from whoever can supply it.
type Prompt struct {
	Step    LoginStep
	Message string
}

// Credentials are the values the environment supplied at startup. Each is
// tried once: a login code is single-use, a rejected password will not become
// right by being resent, and a phone that took the flow to a dead end is not
// worth sending another code to.
type Credentials struct {
	Phone    string
	Code     string
	Password string
}

// LoginAPI is the slice of gotd's auth.Client the flow drives. *auth.Client
// satisfies it; tests script it.
type LoginAPI interface {
	Status(ctx context.Context) (*auth.Status, error)
	SendCode(ctx context.Context, phone string, options auth.SendCodeOptions) (tg.AuthSentCodeClass, error)
	ResendCode(ctx context.Context, phone, phoneCodeHash string) (tg.AuthSentCodeClass, error)
	SignIn(ctx context.Context, phone, code, phoneCodeHash string) (*tg.AuthAuthorization, error)
	Password(ctx context.Context, password string) (*tg.AuthAuthorization, error)
}

// ErrSignUpNotSupported means the phone has no Telegram account. Creating one
// is a different flow with its own terms of service, and this server does not
// offer it.
var ErrSignUpNotSupported = errors.New("this phone number has no telegram account; sign-up is not supported here")

// ErrLoginInputRequired means the flow reached a step nothing configured can
// answer; a human has to.
var ErrLoginInputRequired = errors.New("telegram login needs input")

// ErrTooManyLoginAttempts is returned once a step has been answered wrongly
// maxLoginAttempts times; the flow starts over from the phone.
var ErrTooManyLoginAttempts = errors.New("too many failed telegram login attempts, starting over")

// maxLoginAttempts bounds wrong answers per step before the flow resets, so a
// caller retrying in a loop stops short of Telegram's flood limits.
const maxLoginAttempts = 3

// Login is a process-wide, step-wise Telegram login: SendCode → SignIn →
// Password, driven one answer at a time. It exists because MCP 2026-07-28
// removed server-initiated elicitation — a tool handler can only ask for one
// input per round trip and must resume from remembered state on the retry —
// and gotd's Flow.Run is a single blocking call with no such seam.
type Login struct {
	api    LoginAPI
	env    Credentials
	logger *slog.Logger
	onDone func()

	mu       sync.Mutex
	step     LoginStep
	probed   bool
	phone    string
	codeHash string
	envUsed  map[LoginStep]bool
	attempts map[LoginStep]int

	done     atomic.Bool
	doneOnce sync.Once
}

// NewLogin starts at the phone step. onDone runs exactly once, after the
// account is authorized, whichever path got it there.
func NewLogin(api LoginAPI, env Credentials, logger *slog.Logger, onDone func()) *Login {
	return &Login{
		api:      api,
		env:      env,
		logger:   logger,
		onDone:   onDone,
		step:     LoginStepPhone,
		envUsed:  make(map[LoginStep]bool),
		attempts: make(map[LoginStep]int),
	}
}

// Done reports whether the account is authorized. Lock-free, since every tool
// call reads it.
func (login *Login) Done() bool {
	return login.done.Load()
}

// Probe asks Telegram whether the stored session is authorized. Only
// "authorized" is latched: an unauthorized answer leaves the flow pending, and
// any other error is returned with the state unchanged, because a network
// failure says nothing about the session.
func (login *Login) Probe(ctx context.Context) error {
	login.mu.Lock()
	defer login.mu.Unlock()

	_, err := login.probeLocked(ctx)

	return err
}

// RunUnattended drives the flow as far as the environment carries it. It ends
// authorized, with ErrLoginInputRequired naming the first step nobody
// configured, or with the error that stopped it.
func (login *Login) RunUnattended(ctx context.Context) error {
	prompt, err := login.Next(ctx, "", "")
	if err != nil {
		return err
	}

	if prompt != nil {
		//nolint:wrapcheck // Mark adds the sentinel; the message names the missing step.
		return errors.Mark(errors.Newf("telegram login needs the %s and nothing configured supplies it", prompt.Step),
			ErrLoginInputRequired)
	}

	return nil
}

// Next feeds one answer to the flow and returns the next prompt, or nil once
// the account is authorized. An empty step means "whatever the flow is waiting
// for"; a step the flow has already left is stale and its answer is dropped.
// An empty answer takes the step's value from the environment if that value
// has not been tried yet, otherwise the prompt comes back unchanged. Steps
// the environment can answer are consumed in a row, so a human is only ever
// asked for what nothing else supplied.
func (login *Login) Next(ctx context.Context, step LoginStep, answer string) (*Prompt, error) {
	login.mu.Lock()
	defer login.mu.Unlock()

	if login.done.Load() {
		return nil, nil //nolint:nilnil // nil prompt with nil error is the "authorized" answer by contract.
	}

	// A pending flow re-checks the session before asking anyone: the startup
	// probe may have failed on a network error rather than on a real 401.
	if !login.probed {
		authorized, err := login.probeLocked(ctx)
		if err != nil || authorized {
			return nil, err
		}
	}

	if step == "" {
		step = login.step
	}

	if step != login.step {
		login.logger.Warn("telegram login answer for a step the flow already left",
			"answered", step, "current", login.step)

		return login.prompt(""), nil
	}

	return login.drive(ctx, step, answer)
}

// drive applies the answer and keeps going while the environment can answer
// the step the flow moved to, so a human is only asked for what nothing else
// supplies. Caller holds login.mu.
func (login *Login) drive(ctx context.Context, step LoginStep, answer string) (*Prompt, error) {
	for {
		if answer == "" {
			answer = login.takeEnv()
			if answer == "" {
				return login.prompt(""), nil
			}
		}

		prompt, err := login.advance(ctx, answer)
		if err != nil || prompt == nil || prompt.Step == step {
			return prompt, err
		}

		step, answer = prompt.Step, ""
	}
}

func (login *Login) takeEnv() string {
	if login.envUsed[login.step] {
		return ""
	}

	var value string

	switch login.step {
	case LoginStepPhone:
		value = login.env.Phone
	case LoginStepCode:
		value = login.env.Code
	case LoginStepPassword:
		value = login.env.Password
	}

	if value != "" {
		login.envUsed[login.step] = true
	}

	return value
}

func (login *Login) advance(ctx context.Context, answer string) (*Prompt, error) {
	switch login.step {
	case LoginStepPhone:
		return login.sendCode(ctx, strings.TrimSpace(answer))
	case LoginStepCode:
		return login.signIn(ctx, strings.TrimSpace(answer))
	case LoginStepPassword:
		// Verbatim: a 2FA password may legitimately begin or end with spaces.
		return login.checkPassword(ctx, answer)
	default:
		//nolint:wrapcheck // a programming error in this file, not a wrapped cause.
		return nil, errors.Newf("telegram login: unknown step %q", login.step)
	}
}

func (login *Login) sendCode(ctx context.Context, phone string) (*Prompt, error) {
	sent, err := login.api.SendCode(ctx, phone, auth.SendCodeOptions{})
	if err != nil {
		return login.stepFailed(err, "PHONE_NUMBER_INVALID", "that phone number was rejected")
	}

	return login.codeSent(phone, sent)
}

func (login *Login) codeSent(phone string, sent tg.AuthSentCodeClass) (*Prompt, error) {
	switch code := sent.(type) {
	case *tg.AuthSentCode:
		login.phone, login.codeHash = phone, code.PhoneCodeHash
		login.step = LoginStepCode
		login.logger.Info("telegram login code sent", "via", code.Type.TypeName(), "timeout", code.Timeout)

		return login.prompt(""), nil
	case *tg.AuthSentCodeSuccess:
		// The success carries an authorization, and one of its two shapes says
		// the phone has no account. Latching that as a login would arm the
		// revocation guard, and the first real call would then be reported as
		// a revoked session rather than a missing account.
		if _, ok := code.Authorization.(*tg.AuthAuthorization); !ok {
			login.reset()

			return nil, ErrSignUpNotSupported
		}

		login.finish()

		return nil, nil //nolint:nilnil // authorized without a code step.
	default:
		//nolint:wrapcheck // a shape Telegram is not documented to return; nothing to wrap.
		return nil, errors.Newf("telegram login: unexpected sent code type %T", sent)
	}
}

func (login *Login) signIn(ctx context.Context, code string) (*Prompt, error) {
	_, err := login.api.SignIn(ctx, login.phone, code, login.codeHash)
	if err == nil {
		login.finish()

		return nil, nil //nolint:nilnil // authorized.
	}

	if errors.Is(err, auth.ErrPasswordAuthNeeded) {
		login.step = LoginStepPassword
		login.logger.Info("telegram login needs the 2FA password")

		return login.prompt(""), nil
	}

	var signUp *auth.SignUpRequired
	if errors.As(err, &signUp) {
		login.reset()

		return nil, ErrSignUpNotSupported
	}

	if tgerr.Is(err, "PHONE_CODE_EXPIRED") {
		return login.resendCode(ctx)
	}

	return login.stepFailed(err, "PHONE_CODE_INVALID", "that code was rejected")
}

func (login *Login) resendCode(ctx context.Context) (*Prompt, error) {
	sent, err := login.api.ResendCode(ctx, login.phone, login.codeHash)
	if err != nil {
		return nil, errors.Wrap(err, "resending the expired code")
	}

	if s, ok := sent.(*tg.AuthSentCode); ok {
		login.codeHash = s.PhoneCodeHash
	}

	login.logger.Warn("telegram login code expired, a new one was sent")

	return login.countAttempt("the code expired, a new one was sent")
}

func (login *Login) checkPassword(ctx context.Context, password string) (*Prompt, error) {
	_, err := login.api.Password(ctx, password)
	if err == nil {
		login.finish()

		return nil, nil //nolint:nilnil // authorized.
	}

	if errors.Is(err, auth.ErrPasswordInvalid) {
		return login.countAttempt("that password was rejected")
	}

	return login.stepFailed(err, "", "")
}

// stepFailed sorts a step's error: the named rejection re-asks the step,
// AUTH_RESTART starts over, anything else is somebody else's problem.
func (login *Login) stepFailed(err error, rejection, reason string) (*Prompt, error) {
	if rejection != "" && tgerr.Is(err, rejection) {
		return login.countAttempt(reason)
	}

	if tgerr.Is(err, "AUTH_RESTART") {
		login.logger.Warn("telegram asked to restart the login")

		// Counted, not just obeyed: a server answering this to every SendCode
		// would otherwise have the elicitation branch asking forever, since
		// that branch loops inside one call and has no round-trip budget to
		// stop it.
		// Counted on the step the reset lands on, since that is where the next
		// one will read it: counting the step being left would start from zero
		// every time and never reach the limit.
		attempts := login.attempts[LoginStepPhone] + 1
		login.reset()

		if attempts >= maxLoginAttempts {
			// Counter cleared with the rest of the state, as countAttempt does:
			// giving up starts the flow over, so the next caller gets its own
			// three tries rather than inheriting a spent budget.
			return nil, ErrTooManyLoginAttempts
		}

		login.attempts[LoginStepPhone] = attempts

		return login.prompt("telegram asked to start over"), nil
	}

	return nil, err
}

func (login *Login) countAttempt(reason string) (*Prompt, error) {
	login.attempts[login.step]++
	login.logger.Warn("telegram login step rejected", "step", login.step, "attempt", login.attempts[login.step])

	if login.attempts[login.step] >= maxLoginAttempts {
		login.reset()

		return nil, ErrTooManyLoginAttempts
	}

	return login.prompt(reason), nil
}

// probeLocked reports whether the session is authorized, finishing the flow
// when it is. Caller holds login.mu.
func (login *Login) probeLocked(ctx context.Context) (bool, error) {
	if login.done.Load() {
		return true, nil
	}

	status, err := login.api.Status(ctx)
	if err != nil {
		return false, errors.Wrap(err, "checking the telegram session")
	}

	if !status.Authorized {
		login.probed = true

		return false, nil
	}

	var id int64
	if status.User != nil {
		id = status.User.ID
	}

	login.logger.Info("telegram session authorized", "user_id", id)
	login.finish()

	return true, nil
}

func (login *Login) finish() {
	login.done.Store(true)
	login.doneOnce.Do(func() {
		login.logger.Info("telegram login complete")

		if login.onDone != nil {
			login.onDone()
		}
	})
}

// reset returns to the phone step. Values already taken from the environment
// stay used, including the phone: the flow reaches here because that phone led
// nowhere, so re-sending to it would repeat the dead end instead of asking
// someone for a better one.
func (login *Login) reset() {
	login.step = LoginStepPhone
	login.phone, login.codeHash = "", ""
	login.attempts = make(map[LoginStep]int)
}

func (login *Login) prompt(reason string) *Prompt {
	return &Prompt{Step: login.step, Message: promptMessage(login.step, reason)}
}

func promptMessage(step LoginStep, reason string) string {
	var base string

	switch step {
	case LoginStepPhone:
		base = "Enter your Telegram phone number (E.164 format, e.g. +12025551234)"
	case LoginStepCode:
		base = "Enter the Telegram authentication code sent to your device"
	case LoginStepPassword:
		base = "Enter your Telegram 2FA password"
	}

	if reason == "" {
		return base
	}

	return strings.ToUpper(reason[:1]) + reason[1:] + ". " + base
}
