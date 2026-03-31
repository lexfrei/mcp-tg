package telegram_test

import (
	"context"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestAuthenticator_Phone_WithValue(t *testing.T) {
	aut := telegram.NewAuthenticator("+1234567890", "", "")

	phone, err := aut.Phone(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if phone != "+1234567890" {
		t.Errorf("Phone() = %q, want %q", phone, "+1234567890")
	}
}

func TestAuthenticator_Phone_NoSessionNoEnv(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "")

	_, err := aut.Phone(context.Background())
	if err == nil {
		t.Fatal("expected error when no phone and no session")
	}
}

func TestAuthenticator_Password_WithValue(t *testing.T) {
	aut := telegram.NewAuthenticator("", "secret", "")

	pwd, err := aut.Password(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pwd != "secret" {
		t.Errorf("Password() = %q, want %q", pwd, "secret")
	}
}

func TestAuthenticator_Password_NoSessionNoEnv(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "")

	_, err := aut.Password(context.Background())
	if err == nil {
		t.Fatal("expected error when no password and no session")
	}
}

func TestAuthenticator_Code_FromEnv(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "12345")

	code, err := aut.Code(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if code != "12345" {
		t.Errorf("Code() = %q, want %q", code, "12345")
	}
}

func TestAuthenticator_Code_NoSessionNoEnv(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "")

	_, err := aut.Code(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no code and no session")
	}
}

func TestAuthenticator_SignUp(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "")

	_, err := aut.SignUp(context.Background())
	if err == nil {
		t.Fatal("expected error from SignUp")
	}
}

func TestAuthenticator_AcceptTermsOfService(t *testing.T) {
	aut := telegram.NewAuthenticator("", "", "")

	err := aut.AcceptTermsOfService(context.Background(), telegram.EmptyTermsOfService())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
