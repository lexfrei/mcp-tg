package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"

	"github.com/lexfrei/mcp-tg/internal/config"
)

const loginCommand = "login"

// errNotATTY is returned when `login` is invoked without an interactive
// terminal. The login code is delivered by Telegram at runtime, so it can only
// be entered interactively; there is no non-interactive path.
var errNotATTY = errors.New(
	"login requires an interactive terminal — run the binary directly in a terminal, " +
		"or `docker run -it ... login` (note -t)",
)

// loginRequested reports whether the process was invoked as `mcp-tg login`.
func loginRequested(args []string) bool {
	return len(args) > 1 && args[1] == loginCommand
}

const insecureStorageFlag = "--insecure-storage"

// hasInsecureFlag reports whether `--insecure-storage` was passed to `login`.
func hasInsecureFlag(args []string) bool {
	return slices.Contains(args, insecureStorageFlag)
}

// ttyAuthenticator implements auth.UserAuthenticator by reading credentials from
// a terminal: the phone and the login code as plain lines, the 2FA password
// without echo. Credentials never leave this process — nothing is routed through
// MCP, elicitation, tool calls, or any transcript.
type ttyAuthenticator struct {
	in       *bufio.Reader
	out      io.Writer
	readPass func() (string, error)
}

var _ auth.UserAuthenticator = (*ttyAuthenticator)(nil)

func (a *ttyAuthenticator) Phone(_ context.Context) (string, error) {
	return a.line("Telegram phone number (E.164, e.g. +12025550123): ")
}

func (a *ttyAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return a.line("Login code (sent to your Telegram): ")
}

func (a *ttyAuthenticator) Password(_ context.Context) (string, error) {
	fmt.Fprint(a.out, "2FA password (hidden): ")

	pwd, err := a.readPass()

	fmt.Fprintln(a.out)

	if err != nil {
		return "", errors.Wrap(err, "reading password")
	}

	return strings.TrimSpace(pwd), nil
}

func (a *ttyAuthenticator) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (a *ttyAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, errors.New("sign up is not supported, use an existing Telegram account")
}

func (a *ttyAuthenticator) line(label string) (string, error) {
	fmt.Fprint(a.out, label)

	text, err := a.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.Wrap(err, "reading input")
	}

	return strings.TrimSpace(text), nil
}

// runLogin performs an interactive Telegram login and writes the session file
// the server later reuses. It is the credential-safe counterpart to the server:
// stdin/TTY only, no MCP surface. The TTY check is first so a misuse (piped
// stdin, `docker run` without -t) fails fast before any config or network work.
func runLogin() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errNotATTY
	}

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		return errors.Wrap(cfgErr, "invalid configuration")
	}

	insecure := cfg.InsecureStorage || hasInsecureFlag(os.Args)

	storage, storageErr := newSessionStorage(cfg, insecure)
	if storageErr != nil {
		return storageErr
	}

	if insecure {
		if mkdirErr := os.MkdirAll(filepath.Dir(cfg.SessionFile), 0o700); mkdirErr != nil {
			return errors.Wrap(mkdirErr, "creating session directory")
		}

		ensureSessionPerms(cfg.SessionFile)
	}

	authenticator := &ttyAuthenticator{
		in:  bufio.NewReader(os.Stdin),
		out: os.Stderr,
		readPass: func() (string, error) {
			raw, err := term.ReadPassword(int(os.Stdin.Fd()))

			return string(raw), err
		},
	}

	client := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: storage,
		Device:         mcpDevice(),
	})

	return errors.Wrap(client.Run(context.Background(), func(ctx context.Context) error {
		flow := auth.NewFlow(authenticator, auth.SendCodeOptions{})
		if authErr := client.Auth().IfNecessary(ctx, flow); authErr != nil {
			return errors.Wrap(authErr, "authentication failed")
		}

		self, selfErr := client.Self(ctx)
		if selfErr != nil {
			return errors.Wrap(selfErr, "fetching account after login")
		}

		fmt.Fprintf(os.Stderr, "Logged in as %s (id %d). Session saved to %s\n",
			loginDisplayName(self), self.ID, sessionDestination(insecure, cfg.SessionFile))

		return nil
	}), "login")
}

// sessionDestination describes where the session was written, for the login
// success line. In secure mode the session lives in the OS keychain (the file
// path is only its lookup key, not a file); in insecure mode it is the file.
func sessionDestination(insecure bool, sessionFile string) string {
	if insecure {
		return "file " + sessionFile
	}

	return "the OS keychain (service " + keychainService + ")"
}

func loginDisplayName(self *tg.User) string {
	if name := strings.TrimSpace(self.FirstName + " " + self.LastName); name != "" {
		return name
	}

	if self.Username != "" {
		return "@" + self.Username
	}

	return "your account"
}
