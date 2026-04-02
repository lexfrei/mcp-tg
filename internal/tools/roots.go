package tools

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrPathOutsideRoots is returned when a file path is not within any client root.
var ErrPathOutsideRoots = errors.New("file path is not within any client root directory")

// validatePathAgainstRoots checks that the given file path is within one of the
// client's declared root directories. If the client doesn't declare roots or
// the ListRoots call fails, the path is allowed (graceful degradation).
func validatePathAgainstRoots(ctx context.Context, session *mcp.ServerSession, filePath string) error {
	if session == nil {
		return nil
	}

	roots, err := session.ListRoots(ctx, nil)
	if err != nil {
		if isMethodNotFound(err) {
			return nil
		}

		return errors.Wrap(err, "listing client roots")
	}

	if len(roots.Roots) == 0 {
		return nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return errors.Wrap(err, "resolving absolute path")
	}

	for _, root := range roots.Roots {
		rootPath := rootToPath(root)
		if rootPath == "" {
			continue
		}

		if isUnderDir(absPath, rootPath) {
			return nil
		}
	}

	return ErrPathOutsideRoots
}

func rootToPath(root *mcp.Root) string {
	parsed, err := url.Parse(root.URI)
	if err != nil {
		return ""
	}

	if parsed.Scheme != "file" {
		return ""
	}

	return parsed.Path
}

// isMethodNotFound checks if the error is a JSON-RPC "method not found" (-32601),
// which means the client doesn't support the roots capability.
func isMethodNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "method not found")
}

func isUnderDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(rel, "..")
}
