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

func TestServerVersionTool_Definition(t *testing.T) {
	tool := ServerVersionTool()

	if tool.Name != ServerVersionToolName {
		t.Errorf("Tool name = %q, want %q", tool.Name, ServerVersionToolName)
	}

	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("Tool must be annotated as read-only")
	}
}
