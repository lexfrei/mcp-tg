package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// notifyProgress sends a progress notification if a session and progress token are available.
func notifyProgress(ctx context.Context, session *mcp.ServerSession, token any, current, total float64, message string) {
	if session == nil || token == nil {
		return
	}

	_ = session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
		ProgressToken: token,
		Progress:      current,
		Total:         total,
		Message:       message,
	})
}
