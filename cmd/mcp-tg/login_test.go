package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func TestLoginDisplayName(t *testing.T) {
	cases := []struct {
		name string
		user *tg.User
		want string
	}{
		{"full name", &tg.User{FirstName: "Ada", LastName: "Lovelace"}, "Ada Lovelace"},
		{"first only", &tg.User{FirstName: "Ada"}, "Ada"},
		{"username fallback", &tg.User{Username: "ada"}, "@ada"},
		{"empty fallback", &tg.User{}, loginFallbackName},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loginDisplayName(tc.user); got != tc.want {
				t.Errorf("loginDisplayName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoginRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"mcp-tg"}, false},
		{"login subcommand", []string{"mcp-tg", "login"}, true},
		{"login with extra", []string{"mcp-tg", "login", "--verbose"}, true},
		{"other subcommand", []string{"mcp-tg", "serve"}, false},
		{"empty", []string{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loginRequested(tc.args); got != tc.want {
				t.Errorf("loginRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestSessionDestination(t *testing.T) {
	secure := sessionDestination(false, "/home/x/.mcp-tg/session.json")
	if !strings.Contains(secure, "keychain") || strings.Contains(secure, "file ") {
		t.Errorf("secure destination = %q, want a keychain description without a file path", secure)
	}

	// The path is the keychain account key; the success line must name it so a
	// user running multiple sessions can tell which item was written.
	if !strings.Contains(secure, "/home/x/.mcp-tg/session.json") {
		t.Errorf("secure destination = %q, want the account key included", secure)
	}

	insecure := sessionDestination(true, "/home/x/.mcp-tg/session.json")
	if insecure != "file /home/x/.mcp-tg/session.json" {
		t.Errorf("insecure destination = %q, want the file path", insecure)
	}
}

func TestHasInsecureFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"absent", []string{"mcp-tg", "login"}, false},
		{"present", []string{"mcp-tg", "login", "--insecure-storage"}, true},
		{"present with other flags", []string{"mcp-tg", "login", "-x", "--insecure-storage"}, true},
		{"lookalike not matched", []string{"mcp-tg", "login", "--insecure"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasInsecureFlag(tc.args); got != tc.want {
				t.Errorf("hasInsecureFlag(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func newTestAuthenticator(input, password string) *ttyAuthenticator {
	return &ttyAuthenticator{
		in:       bufio.NewReader(strings.NewReader(input)),
		out:      &bytes.Buffer{},
		readPass: func() (string, error) { return password, nil },
	}
}

func TestTTYAuthenticator_PhoneAndCodeReadTrimmedLines(t *testing.T) {
	aut := newTestAuthenticator("  +12025550123 \n  54321 \n", "")

	phone, err := aut.Phone(t.Context())
	if err != nil {
		t.Fatalf("Phone: %v", err)
	}

	if phone != "+12025550123" {
		t.Errorf("Phone = %q, want %q", phone, "+12025550123")
	}

	code, err := aut.Code(t.Context(), nil)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}

	if code != "54321" {
		t.Errorf("Code = %q, want %q", code, "54321")
	}
}

func TestTTYAuthenticator_PasswordUsesHiddenReaderAndTrims(t *testing.T) {
	aut := newTestAuthenticator("", "  s3cret \n")

	pwd, err := aut.Password(t.Context())
	if err != nil {
		t.Fatalf("Password: %v", err)
	}

	if pwd != "s3cret" {
		t.Errorf("Password = %q, want %q", pwd, "s3cret")
	}
}

func TestTTYAuthenticator_SignUpUnsupported(t *testing.T) {
	aut := newTestAuthenticator("", "")

	_, err := aut.SignUp(t.Context())
	if err == nil {
		t.Error("SignUp should be unsupported (existing accounts only)")
	}
}

func TestRunLogin_RequiresTTY(t *testing.T) {
	// go test runs with stdin detached from any terminal, so runLogin must
	// bail out before touching config or the network.
	err := runLogin()
	if !errors.Is(err, errNotATTY) {
		t.Errorf("runLogin without a TTY = %v, want errNotATTY", err)
	}
}
