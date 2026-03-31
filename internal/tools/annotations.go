package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

// readOnlyAnnotations returns annotations for tools that only read data.
func readOnlyAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(true),
	}
}

// idempotentAnnotations returns annotations for tools that modify state but are idempotent.
func idempotentAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		IdempotentHint:  true,
		DestructiveHint: new(false),
		OpenWorldHint:   new(true),
	}
}

// destructiveAnnotations returns annotations for tools that perform destructive operations.
func destructiveAnnotations() *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		DestructiveHint: new(true),
		OpenWorldHint:   new(true),
	}
}
