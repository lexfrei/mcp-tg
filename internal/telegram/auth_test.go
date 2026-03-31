package telegram_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestEnvCodeAuthenticator_Phone_WithValue(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("+1234567890", "", "")

	phone, err := auth.Phone(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if phone != "+1234567890" {
		t.Errorf("Phone() = %q, want %q", phone, "+1234567890")
	}
}

func TestEnvCodeAuthenticator_Phone_Empty(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "", "")

	_, err := auth.Phone(context.Background())
	if err == nil {
		t.Fatal("expected error for empty phone")
	}
}

func TestEnvCodeAuthenticator_Password_WithValue(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "secret", "")

	pwd, err := auth.Password(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pwd != "secret" {
		t.Errorf("Password() = %q, want %q", pwd, "secret")
	}
}

func TestEnvCodeAuthenticator_Password_Empty(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "", "")

	_, err := auth.Password(context.Background())
	if err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestEnvCodeAuthenticator_Code_FromEnv(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "", "12345")

	code, err := auth.Code(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != "12345" {
		t.Errorf("Code() = %q, want %q", code, "12345")
	}
}

func TestEnvCodeAuthenticator_Code_FromReader(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticatorWithInput("", "", "", strings.NewReader("67890\n"))

	code, err := auth.Code(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != "67890" {
		t.Errorf("Code() = %q, want %q", code, "67890")
	}
}

func TestEnvCodeAuthenticator_Code_EmptyReader(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticatorWithInput("", "", "", strings.NewReader(""))

	_, err := auth.Code(context.Background())
	if err == nil {
		t.Fatal("expected error for empty reader")
	}
}

func TestEnvCodeAuthenticator_SignUp(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "", "")

	_, err := auth.SignUp(context.Background())
	if err == nil {
		t.Fatal("expected error from SignUp")
	}
}

func TestEnvCodeAuthenticator_AcceptTermsOfService(t *testing.T) {
	auth := telegram.NewEnvCodeAuthenticator("", "", "")

	err := auth.AcceptTermsOfService(context.Background(), telegram.EmptyTermsOfService())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
