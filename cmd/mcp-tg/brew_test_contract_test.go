package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The Homebrew formula's `test do` block (see .goreleaser.yaml) runs the binary
// with the credentials cleared and asserts on THREE things at once:
//
//	shell_output("TELEGRAM_APP_ID= TELEGRAM_APP_HASH= #{bin}/mcp-tg 2>&1", 1)
//	assert_match "TELEGRAM_APP_ID is required", ...
//
//	1. the process terminates at all — a credential-less start must not reach
//	   the network, or brew test hangs on a live MTProto connection;
//	2. it exits with code 1;
//	3. stderr carries that exact sentence.
//
// Nothing else pins any of it: the tap is where it breaks, and nobody looks
// there. This test pins all three, using the subprocess re-entry pattern
// because the path ends in os.Exit.
const brewContractHelper = "TEST_BREW_CONTRACT_HELPER"

func TestBrewFormulaContract_NoCredentialsExitsOneWithoutNetwork(t *testing.T) {
	if os.Getenv(brewContractHelper) == "1" {
		main()

		return
	}

	// A credential-less start must fail on config, long before any dialling.
	// The timeout is the "does not hang" half of the contract.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=TestBrewFormulaContract_NoCredentialsExitsOneWithoutNetwork")
	cmd.Env = append(os.Environ(),
		brewContractHelper+"=1",
		"TELEGRAM_APP_ID=",
		"TELEGRAM_APP_HASH=",
	)

	out, err := cmd.CombinedOutput()

	if ctx.Err() != nil {
		t.Fatalf("the binary did not terminate without credentials — brew test would hang: %s", out)
	}

	exitErr, ok := err.(*exec.ExitError) //nolint:errorlint // ExitError is the concrete type carrying the code.
	if !ok {
		t.Fatalf("expected a non-zero exit, got err=%v output=%s", err, out)
	}

	if code := exitErr.ExitCode(); code != 1 {
		t.Errorf("exit code = %d, want 1 — .goreleaser.yaml's brew test asserts on 1", code)
	}

	if !strings.Contains(string(out), "TELEGRAM_APP_ID is required") {
		t.Errorf("output does not carry the sentence the brew test greps for:\n%s", out)
	}
}
