package tools

import (
	"context"
	"sync"

	"github.com/lexfrei/mcp-tg/internal/telegram"
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

// progressForwarder adapts byte-level upload progress to MCP progress
// notifications and emits a terminal 100% once the upload completes. It is
// created only when the caller supplied a progress token; without one
// newProgressForwarder returns nil and callback() yields nil, so the wrapper
// skips the extra per-call uploader entirely.
type progressForwarder struct {
	session   *mcp.ServerSession
	token     any
	message   string
	mu        sync.Mutex
	lastTotal float64
}

func newProgressForwarder(session *mcp.ServerSession, token any, message string) *progressForwarder {
	if token == nil {
		return nil
	}

	return &progressForwarder{session: session, token: token, message: message}
}

// callback returns the UploadProgress to hand to the telegram layer, or nil
// when there is no progress token to report against.
func (fwd *progressForwarder) callback() telegram.UploadProgress {
	if fwd == nil {
		return nil
	}

	return fwd.report
}

// report forwards one byte-level progress update. Safe for concurrent calls
// from the uploader's worker pool. Mid-stream monotonicity is intentionally NOT
// enforced — callbacks from different workers on a multi-part upload can arrive
// out of order — only done() guarantees a clean terminal 100%.
func (fwd *progressForwarder) report(ctx context.Context, uploaded, total int64) {
	fwd.mu.Lock()
	fwd.lastTotal = float64(total)
	fwd.mu.Unlock()

	notifyProgress(ctx, fwd.session, fwd.token, float64(uploaded), float64(total), fwd.message)
}

// terminalTotal returns the total to report at completion: the last total seen,
// or 1 as a fallback when no chunk ever fired (e.g. an empty upload, where gotd
// returns on EOF before calling Chunk) so completion still reads as 100% rather
// than 0/0.
func (fwd *progressForwarder) terminalTotal() float64 {
	fwd.mu.Lock()
	defer fwd.mu.Unlock()

	if fwd.lastTotal == 0 {
		return 1
	}

	return fwd.lastTotal
}

// done emits a terminal 100% notification once the upload+send completes. No-op
// on a nil forwarder (no progress token was supplied). For a single-part upload
// this may duplicate the last in-band report; callers that dedupe should expect
// that.
func (fwd *progressForwarder) done(ctx context.Context, message string) {
	if fwd == nil {
		return
	}

	total := fwd.terminalTotal()
	notifyProgress(ctx, fwd.session, fwd.token, total, total, message)
}
