package tools

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// shortRevisionLen is the number of leading characters of the git commit SHA
// shown in the human-readable Output line. Eight is enough to disambiguate
// commits in any realistically-sized repo while staying glanceable.
const shortRevisionLen = 8

// ServerVersionToolName is the MCP tool name. Exported so tests and any
// future tool-allowlist (e.g. auth-bypass) can reference it without
// duplicating the literal.
const ServerVersionToolName = "tg_server_version"

// ServerVersionParams takes no arguments — version metadata is fixed at
// build time and supplied by the handler factory.
type ServerVersionParams struct{}

// ServerVersionResult is the output of the tg_server_version tool.
type ServerVersionResult struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	GoVersion string `json:"goVersion"`
	Output    string `json:"output"`
}

// NewServerVersionHandler creates a handler that reports the build metadata
// passed in — typically the package-level `version` and `revision` strings
// injected via -ldflags from the Containerfile, plus runtime.Version().
// Taking these as parameters (instead of reading from the cmd package
// directly) avoids an internal-tool import cycle and keeps the handler
// trivially testable.
func NewServerVersionHandler(
	version, revision, goVersion string,
) mcp.ToolHandlerFor[ServerVersionParams, ServerVersionResult] {
	return func(
		_ context.Context,
		_ *mcp.CallToolRequest,
		_ ServerVersionParams,
	) (*mcp.CallToolResult, ServerVersionResult, error) {
		shortRev := revision
		if len(shortRev) > shortRevisionLen {
			shortRev = shortRev[:shortRevisionLen]
		}

		return nil, ServerVersionResult{
			Version:   version,
			Revision:  revision,
			GoVersion: goVersion,
			Output:    fmt.Sprintf("mcp-tg %s (%s)", version, shortRev),
		}, nil
	}
}

// ServerVersionTool returns the MCP tool definition for tg_server_version.
func ServerVersionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        ServerVersionToolName,
		Description: "Get the mcp-tg server build metadata: semver tag, git commit SHA, and Go runtime version",
		Annotations: readOnlyAnnotations(),
	}
}
