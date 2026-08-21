package tools

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrPathOutsideRoots is returned when a file path is not within any directory
// listed in TELEGRAM_FILE_ROOTS.
var ErrPathOutsideRoots = errors.New(
	"file path is not within any TELEGRAM_FILE_ROOTS directory")

// validatePathAgainstRoots checks that filePath is under one of the configured
// root directories. An empty list means no restriction: the operator has not
// asked for a boundary, and inventing one would break every existing setup.
// This replaced the MCP roots capability (deprecated by SEP-2577), which only
// ever protected sessions whose client declared roots; a configured allowlist
// holds for every client of the shared daemon.
//
// Both sides are compared after symlink resolution: a link under a root must
// not smuggle in the tree it points to, and a root that is itself a symlink
// (macOS /tmp -> /private/tmp) must admit its resolved tree. The check is
// advisory against confused callers, not a sandbox — a link swapped between
// validation and use is out of scope.
func validatePathAgainstRoots(roots []string, filePath string) error {
	if len(roots) == 0 {
		return nil
	}

	// A `..` component is rejected before anything else. filepath.Abs Cleans
	// it lexically, but the kernel resolves symlinks first: `root/link/../x`
	// validates as `root/x` while the I/O lands next to the link's target.
	// No lexical verdict is safe for such a spelling, and no legitimate
	// caller needs it — every in-root path has a `..`-free spelling.
	if hasDotDot(filePath) {
		return ErrPathOutsideRoots
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return errors.Wrap(err, "resolving absolute path")
	}

	realPath, err := resolveSymlinks(absPath)
	if err != nil {
		return errors.Wrap(err, "resolving symlinks")
	}

	for _, root := range roots {
		realRoot, rootErr := resolveSymlinks(root)
		if rootErr != nil {
			continue
		}

		if isUnderDir(realPath, realRoot) {
			return nil
		}
	}

	return ErrPathOutsideRoots
}

// resolveSymlinks canonicalizes path even when its tail does not exist yet (a
// download directory about to be created): the nearest existing ancestor is
// resolved and the remainder is reattached lexically.
func resolveSymlinks(path string) (string, error) {
	suffix := ""
	current := path

	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}

		if !errors.Is(err, fs.ErrNotExist) {
			return "", errors.Wrapf(err, "resolving %q", current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.Wrapf(err, "no existing ancestor for %q", path)
		}

		suffix = filepath.Join(filepath.Base(current), suffix)
		current = parent
	}
}

const dotDot = ".."

func hasDotDot(p string) bool {
	for part := range strings.SplitSeq(filepath.ToSlash(p), "/") {
		if part == dotDot {
			return true
		}
	}

	return false
}

func isUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	return rel != dotDot && !strings.HasPrefix(rel, dotDot+string(filepath.Separator))
}
