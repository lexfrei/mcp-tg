package tools

import (
	"strings"
	"testing"
)

func TestServerVersionHandler_ReturnsBuildInfo(t *testing.T) {
	handler := NewServerVersionHandler("1.2.3", "abc1234deadbeef", "go1.26.2")

	_, result, err := handler(t.Context(), nil, ServerVersionParams{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", result.Version, "1.2.3")
	}

	if result.Revision != "abc1234deadbeef" {
		t.Errorf("Revision = %q, want %q", result.Revision, "abc1234deadbeef")
	}

	if result.GoVersion != "go1.26.2" {
		t.Errorf("GoVersion = %q, want %q", result.GoVersion, "go1.26.2")
	}
}

func TestServerVersionHandler_OutputFormat(t *testing.T) {
	handler := NewServerVersionHandler("1.2.3", "abc1234deadbeef", "go1.26.2")

	_, result, err := handler(t.Context(), nil, ServerVersionParams{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !strings.Contains(result.Output, "1.2.3") {
		t.Errorf("Output %q must contain version %q", result.Output, "1.2.3")
	}

	if !strings.Contains(result.Output, "abc1234d") {
		t.Errorf("Output %q must contain short revision (first 8 chars of SHA)", result.Output)
	}

	if strings.Contains(result.Output, "abc1234deadbeef") {
		t.Errorf("Output %q must use short revision, not full SHA", result.Output)
	}
}

// Boundary: SHA exactly shortRevisionLen long must pass through unchanged.
// The predicate is strict-greater (`> shortRevisionLen`), so an 8-char SHA
// is rendered fully without a slice operation.
func TestServerVersionHandler_RevisionExactlyEightChars(t *testing.T) {
	handler := NewServerVersionHandler("1.2.3", "abcdefgh", "go1.26.2")

	_, result, err := handler(t.Context(), nil, ServerVersionParams{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.Revision != "abcdefgh" {
		t.Errorf("Revision = %q, want %q", result.Revision, "abcdefgh")
	}

	if !strings.Contains(result.Output, "abcdefgh") {
		t.Errorf("Output %q must contain the full 8-char SHA", result.Output)
	}
}

// Short revisions (already < 8 chars) must pass through unchanged rather than
// crashing on the slice operation.
func TestServerVersionHandler_ShortRevisionUnshortened(t *testing.T) {
	handler := NewServerVersionHandler("1.2.3", "abc", "go1.26.2")

	_, result, err := handler(t.Context(), nil, ServerVersionParams{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if result.Revision != "abc" {
		t.Errorf("Revision = %q, want %q", result.Revision, "abc")
	}

	if !strings.Contains(result.Output, "abc") {
		t.Errorf("Output %q must contain the short revision", result.Output)
	}
}

// FormatVersionLine is the single source of the build-identity line for both
// the tg_server_version tool and the `mcp-tg --version` CLI; pin its exact
// format and revision truncation so neither caller can drift.
func TestFormatVersionLine(t *testing.T) {
	cases := []struct {
		version, revision, want string
	}{
		{"1.2.3", "abc1234deadbeef", "mcp-tg 1.2.3 (abc1234d)"}, // long SHA truncated to 8
		{"1.2.3", "abcdefgh", "mcp-tg 1.2.3 (abcdefgh)"},        // exactly 8, unchanged
		{"1.2.3", "abc", "mcp-tg 1.2.3 (abc)"},                  // shorter than 8, unchanged
		{"dev", "unknown", "mcp-tg dev (unknown)"},              // the default build strings
	}

	for _, tc := range cases {
		if got := FormatVersionLine(tc.version, tc.revision); got != tc.want {
			t.Errorf("FormatVersionLine(%q, %q) = %q, want %q", tc.version, tc.revision, got, tc.want)
		}
	}
}

// The handler's Output must be exactly what FormatVersionLine produces, so the
// tool and the CLI can never disagree.
func TestServerVersionHandler_OutputIsFormatVersionLine(t *testing.T) {
	_, result, err := NewServerVersionHandler("1.2.3", "abc1234deadbeef", "go1.26.2")(
		t.Context(), nil, ServerVersionParams{})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if want := FormatVersionLine("1.2.3", "abc1234deadbeef"); result.Output != want {
		t.Errorf("handler Output = %q, want FormatVersionLine %q", result.Output, want)
	}
}

func TestServerVersionTool_Definition(t *testing.T) {
	tool := ServerVersionTool()

	if tool.Name != ServerVersionToolName {
		t.Errorf("Tool name = %q, want %q", tool.Name, ServerVersionToolName)
	}

	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("Tool must be annotated as read-only")
	}
}
