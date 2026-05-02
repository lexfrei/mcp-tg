package telegram

import (
	"context"
	"log"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrPhoneRequired is returned when no phone number is configured.
var ErrPhoneRequired = errors.New("TELEGRAM_PHONE is required for authentication")

// ErrPasswordRequired is returned when 2FA is needed but no password is set.
var ErrPasswordRequired = errors.New("TELEGRAM_PASSWORD is required for 2FA")

// ErrNoAuthCode is returned when no authentication code is provided.
var ErrNoAuthCode = errors.New("no authentication code provided")

// ErrSignUpNotSupported is returned when sign-up is attempted.
var ErrSignUpNotSupported = errors.New("sign up is not supported, use an existing Telegram account")

// ErrElicitDeclined is returned when the user declines the elicitation prompt.
var ErrElicitDeclined = errors.New("user declined authentication prompt")

// Authenticator implements auth.UserAuthenticator using a cascade:
// env var → MCP elicitation → error.
// Stdin fallback is NOT used because MCP stdio transport owns stdin.
type Authenticator struct {
	phone    string
	password string
	code     string
	session  *mcp.ServerSession
}

// NewAuthenticator creates an authenticator with env-based credentials.
func NewAuthenticator(phone, password, code string) *Authenticator {
	return &Authenticator{
		phone:    phone,
		password: password,
		code:     code,
	}
}

// SetSession sets the MCP server session for elicitation support.
func (aut *Authenticator) SetSession(session *mcp.ServerSession) {
	aut.session = session
}

// Phone returns the phone number via cascade: env → elicitation → error.
func (aut *Authenticator) Phone(ctx context.Context) (string, error) {
	if aut.phone != "" {
		return aut.phone, nil
	}

	phone, err := aut.elicitString(ctx, "Enter your Telegram phone number (E.164 format, e.g. +12025551234)", "phone")
	if err == nil && phone != "" {
		return phone, nil
	}

	return "", ErrPhoneRequired
}

// Password returns the 2FA password via cascade: env → elicitation → error.
func (aut *Authenticator) Password(ctx context.Context) (string, error) {
	if aut.password != "" {
		return aut.password, nil
	}

	pwd, err := aut.elicitString(ctx, "Enter your Telegram 2FA password", "password")
	if err == nil && pwd != "" {
		return pwd, nil
	}

	return "", ErrPasswordRequired
}

// Code returns the auth code via cascade: env → elicitation → error.
func (aut *Authenticator) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	if aut.code != "" {
		return aut.code, nil
	}

	code, err := aut.elicitString(ctx, "Enter the Telegram authentication code sent to your device", "code")
	if err == nil && code != "" {
		return code, nil
	}

	return "", ErrNoAuthCode
}

// AcceptTermsOfService always accepts the ToS and logs the action.
func (aut *Authenticator) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	log.Printf("auto-accepted Telegram ToS (ID: %s)", tos.ID.Data)

	return nil
}

// SignUp is not supported; we require an existing account.
func (aut *Authenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, ErrSignUpNotSupported
}

// EmptyTermsOfService returns an empty tg.HelpTermsOfService for testing.
func EmptyTermsOfService() tg.HelpTermsOfService {
	return tg.HelpTermsOfService{}
}

func (aut *Authenticator) elicitString(ctx context.Context, message, fieldName string) (string, error) {
	if aut.session == nil {
		return "", errors.New("no MCP session available for elicitation")
	}

	//nolint:goconst // "type" is a JSON Schema keyword; extracting a constant adds no clarity over the literal.
	result, err := aut.session.Elicit(ctx, &mcp.ElicitParams{
		Message: message,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				fieldName: map[string]any{
					"type":        "string",
					"description": message,
				},
			},
			"required": []string{fieldName},
		},
	})
	if err != nil {
		return "", errors.Wrap(err, "elicitation failed")
	}

	if result == nil {
		return "", errors.New("elicitation returned empty result")
	}

	if result.Action != "accept" {
		return "", ErrElicitDeclined
	}

	val, ok := result.Content[fieldName]
	if !ok {
		return "", errors.New("field not found in elicitation response")
	}

	str, ok := val.(string)
	if !ok {
		return "", errors.New("unexpected type in elicitation response")
	}

	return strings.TrimSpace(str), nil
}
