package telegram

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// ErrPhoneRequired is returned when no phone number is configured.
var ErrPhoneRequired = errors.New("TELEGRAM_PHONE is required for authentication")

// ErrPasswordRequired is returned when 2FA is needed but no password is set.
var ErrPasswordRequired = errors.New("TELEGRAM_PASSWORD is required for 2FA")

// ErrNoAuthCode is returned when no authentication code is provided.
var ErrNoAuthCode = errors.New("no authentication code provided")

// ErrSignUpNotSupported is returned when sign-up is attempted.
var ErrSignUpNotSupported = errors.New("sign up is not supported, use an existing Telegram account")

// EnvCodeAuthenticator implements auth.UserAuthenticator using environment variables
// with fallback to stderr/stdin prompts.
type EnvCodeAuthenticator struct {
	phone    string
	password string
	code     string
	input    io.Reader
}

// NewEnvCodeAuthenticator creates an authenticator that reads credentials from config.
func NewEnvCodeAuthenticator(phone, password, code string) *EnvCodeAuthenticator {
	return &EnvCodeAuthenticator{
		phone:    phone,
		password: password,
		code:     code,
		input:    os.Stdin,
	}
}

// NewEnvCodeAuthenticatorWithInput creates an authenticator with a custom input reader.
func NewEnvCodeAuthenticatorWithInput(phone, password, code string, input io.Reader) *EnvCodeAuthenticator {
	return &EnvCodeAuthenticator{
		phone:    phone,
		password: password,
		code:     code,
		input:    input,
	}
}

// EmptyTermsOfService returns an empty tg.HelpTermsOfService for testing.
func EmptyTermsOfService() tg.HelpTermsOfService {
	return tg.HelpTermsOfService{}
}

// Phone returns the phone number for authentication.
func (a *EnvCodeAuthenticator) Phone(_ context.Context) (string, error) {
	if a.phone != "" {
		return a.phone, nil
	}

	return "", ErrPhoneRequired
}

// Password returns the 2FA password.
func (a *EnvCodeAuthenticator) Password(_ context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}

	return "", ErrPasswordRequired
}

// Code returns the authentication code from env var or stdin prompt.
func (a *EnvCodeAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	if a.code != "" {
		return a.code, nil
	}

	_, _ = os.Stderr.WriteString("Enter authentication code: ")

	scanner := bufio.NewScanner(a.input)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}

	scanErr := scanner.Err()
	if scanErr != nil {
		return "", errors.Wrap(scanErr, "reading auth code")
	}

	return "", ErrNoAuthCode
}

// AcceptTermsOfService always accepts the ToS.
func (a *EnvCodeAuthenticator) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

// SignUp is not supported; we require an existing account.
func (a *EnvCodeAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, ErrSignUpNotSupported
}
