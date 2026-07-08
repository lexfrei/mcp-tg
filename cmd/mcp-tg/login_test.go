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

func TestTTYAuthenticator_PasswordReturnedExactly(t *testing.T) {
	// A 2FA password must reach Telegram byte-for-byte: trimming would corrupt a
	// password that intentionally has leading/trailing whitespace, and
	// term.ReadPassword already returns it without the line terminator.
	const secret = "  s3 cret  "

	aut := newTestAuthenticator("", secret)

	pwd, err := aut.Password(t.Context())
	if err != nil {
		t.Fatalf("Password: %v", err)
	}

	if pwd != secret {
		t.Errorf("Password = %q, want %q (no trimming)", pwd, secret)
	}
}

func TestTTYAuthenticator_SignUpUnsupported(t *testing.T) {
	aut := newTestAuthenticator("", "")

	_, err := aut.SignUp(t.Context())
	if err == nil {
		t.Error("SignUp should be unsupported (existing accounts only)")
	}
}

// errReader fails every Read, to drive the non-EOF read-error branch of line().
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestTTYAuthenticator_PasswordReadErrorWrapped(t *testing.T) {
	errBoom := errors.New("tty read failed")
	aut := &ttyAuthenticator{
		in:       bufio.NewReader(strings.NewReader("")),
		out:      &bytes.Buffer{},
		readPass: func() (string, error) { return "", errBoom },
	}

	_, err := aut.Password(t.Context())
	if !errors.Is(err, errBoom) {
		t.Errorf("Password error = %v, want wrapped %v", err, errBoom)
	}
}

func TestTTYAuthenticator_LineReadErrorWrapped(t *testing.T) {
	errBoom := errors.New("stdin broke")
	aut := &ttyAuthenticator{
		in:       bufio.NewReader(errReader{err: errBoom}),
		out:      &bytes.Buffer{},
		readPass: func() (string, error) { return "", nil },
	}

	// Phone() reads a line; a non-EOF read error must surface (EOF is tolerated).
	_, err := aut.Phone(t.Context())
	if !errors.Is(err, errBoom) {
		t.Errorf("Phone error = %v, want wrapped %v", err, errBoom)
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
