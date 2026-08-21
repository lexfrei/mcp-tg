package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cockroachdb/errors"
)

func TestValidatePathAgainstRoots_EmptyRootsAllowsEverything(t *testing.T) {
	err := validatePathAgainstRoots(nil, "/anywhere/at/all")
	if err != nil {
		t.Errorf("err = %v, want nil when no roots are configured", err)
	}
}

func TestValidatePathAgainstRoots_PathUnderARootPasses(t *testing.T) {
	root := t.TempDir()

	err := validatePathAgainstRoots([]string{root}, filepath.Join(root, "sub", "file.jpg"))
	if err != nil {
		t.Errorf("err = %v, want nil for a path under the root", err)
	}
}

func TestValidatePathAgainstRoots_SecondRootWorksToo(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()

	err := validatePathAgainstRoots([]string{first, second}, filepath.Join(second, "file.jpg"))
	if err != nil {
		t.Errorf("err = %v, want nil for a path under the second root", err)
	}
}

func TestValidatePathAgainstRoots_PathOutsideRootsFails(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	err := validatePathAgainstRoots([]string{root}, filepath.Join(outside, "file.jpg"))
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Errorf("err = %v, want ErrPathOutsideRoots", err)
	}
}

// A root must match on directory boundaries: /tmp/foo does not admit
// /tmp/foobar, even though it is a string prefix.
func TestValidatePathAgainstRoots_SiblingPrefixDoesNotPass(t *testing.T) {
	root := t.TempDir()

	err := validatePathAgainstRoots([]string{root}, root+"-sibling/file.jpg")
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Errorf("err = %v, want ErrPathOutsideRoots for a sibling prefix", err)
	}
}

func TestValidatePathAgainstRoots_RelativePathResolvesAgainstCwd(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	err := validatePathAgainstRoots([]string{root}, "file.jpg")
	if err != nil {
		t.Errorf("err = %v, want a relative path resolved under the cwd root to pass", err)
	}
}

func TestValidatePathAgainstRoots_DotDotEscapeFails(t *testing.T) {
	root := t.TempDir()

	err := validatePathAgainstRoots([]string{root}, filepath.Join(root, "..", "escape.jpg"))
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Errorf("err = %v, want ErrPathOutsideRoots for a .. escape", err)
	}
}

// A symlink under a root must not smuggle in the tree it points to: the check
// runs on the resolved path, not the spelled one.
func TestValidatePathAgainstRoots_SymlinkEscapeFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "link")
	symErr := os.Symlink(outside, link)
	if symErr != nil {
		t.Fatalf("creating symlink: %v", symErr)
	}

	err := validatePathAgainstRoots([]string{root}, filepath.Join(link, "secret.jpg"))
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Errorf("err = %v, want ErrPathOutsideRoots for a path through an escaping symlink", err)
	}
}

// The dot-dot spelling of the same escape: link/../secret Cleans lexically to
// root/secret while the kernel resolves link first and lands outside. No
// lexical verdict is safe for such a path, so it must be rejected outright.
func TestValidatePathAgainstRoots_DotDotThroughSymlinkFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()

	link := filepath.Join(root, "link")
	symErr := os.Symlink(outside, link)
	if symErr != nil {
		t.Fatalf("creating symlink: %v", symErr)
	}

	err := validatePathAgainstRoots([]string{root}, root+"/link/../secret.txt")
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Errorf("err = %v, want ErrPathOutsideRoots for a dot-dot spelling through a symlink", err)
	}
}

// The converse: a root that is itself a symlink (macOS /tmp -> /private/tmp)
// must admit paths spelled through the resolved location.
func TestValidatePathAgainstRoots_SymlinkedRootAdmitsItsRealTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}

	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")

	symErr := os.Symlink(target, link)
	if symErr != nil {
		t.Fatalf("creating symlink: %v", symErr)
	}

	err := validatePathAgainstRoots([]string{link}, filepath.Join(target, "file.jpg"))
	if err != nil {
		t.Errorf("err = %v, want nil for the resolved spelling of a symlinked root", err)
	}
}

// A download directory that does not exist yet must still validate: the
// nearest existing ancestor is resolved and the rest is judged lexically.
func TestValidatePathAgainstRoots_NotYetExistingSubdirPasses(t *testing.T) {
	root := t.TempDir()

	err := validatePathAgainstRoots([]string{root}, filepath.Join(root, "new", "deep", "dir"))
	if err != nil {
		t.Errorf("err = %v, want nil for a not-yet-created subdirectory of a root", err)
	}
}

// The rejection must happen in the handler before any Telegram RPC: the error
// identity proves it, since a reached upload would fail as a telegram error,
// not as this validation sentinel.
func TestMediaUploadHandler_RejectsPathOutsideFileRoots(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "escape.jpg")

	handler := NewMediaUploadHandler(&mockClient{}, []string{root})

	result, _, err := handler(context.Background(), nil, MediaUploadParams{Path: outside})
	if !errors.Is(err, ErrPathOutsideRoots) {
		t.Fatalf("err = %v, want ErrPathOutsideRoots", err)
	}

	if result == nil || !result.IsError {
		t.Error("expected the result to be flagged as an error")
	}
}
