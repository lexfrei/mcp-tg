package tools

import (
	"context"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProgressForwarder_GatesOnToken(t *testing.T) {
	if newProgressForwarder(nil, nil, "msg") != nil {
		t.Error("nil token must yield a nil forwarder")
	}

	fwd := newProgressForwarder(nil, "tok", "msg")
	if fwd == nil || fwd.callback() == nil {
		t.Fatal("a progress token must yield a forwarder with a non-nil callback")
	}

	// A nil session must make report/done no-ops, not panics.
	fwd.callback()(t.Context(), 5, 10)
	fwd.done(t.Context(), "done")
}

func TestProgressForwarder_NilReceiverSafe(t *testing.T) {
	var fwd *progressForwarder

	if fwd.callback() != nil {
		t.Error("nil forwarder must yield a nil callback")
	}

	fwd.done(t.Context(), "done") // must not panic
}

func TestProgressForwarder_TerminalTotal(t *testing.T) {
	fwd := newProgressForwarder(nil, "tok", "msg")

	// No chunk ever fired (e.g. an empty upload): fall back to 1 so the
	// terminal notification reads as 100%, not 0/0.
	if got := fwd.terminalTotal(); got != 1 {
		t.Errorf("terminalTotal with no report = %v, want 1", got)
	}

	fwd.report(t.Context(), 500, 1000)

	if got := fwd.terminalTotal(); got != 1000 {
		t.Errorf("terminalTotal after report = %v, want 1000", got)
	}
}

func requestWithToken(token any) *mcp.CallToolRequest {
	params := &mcp.CallToolParamsRaw{}
	if token != nil {
		params.SetProgressToken(token)
	}

	return &mcp.CallToolRequest{Params: params}
}

func TestSendFileHandler_WiresProgressWhenTokenPresent(t *testing.T) {
	mock := &mockClient{message: &telegram.Message{ID: 1}}
	handler := NewMessagesSendFileHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken("tok"),
		MessagesSendFileParams{ParseMode: "plain", Peer: "@x", Path: "/tmp/x"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastSendOpts.Progress == nil {
		t.Error("Progress callback must be wired when a progress token is present")
	}
}

func TestSendFileHandler_NoProgressWithoutToken(t *testing.T) {
	mock := &mockClient{message: &telegram.Message{ID: 1}}
	handler := NewMessagesSendFileHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken(nil),
		MessagesSendFileParams{ParseMode: "plain", Peer: "@x", Path: "/tmp/x"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastSendOpts.Progress != nil {
		t.Error("Progress callback must be nil without a progress token")
	}
}

func TestMediaSendAlbumHandler_WiresProgressWhenTokenPresent(t *testing.T) {
	mock := &mockClient{messages: []telegram.Message{{ID: 1}}}
	handler := NewMediaSendAlbumHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken("tok"),
		MediaSendAlbumParams{ParseMode: "plain", Peer: "@x", Paths: []string{"/tmp/x"}})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastSendOpts.Progress == nil {
		t.Error("Progress callback must be wired when a progress token is present")
	}
}

func TestMediaSendAlbumHandler_NoProgressWithoutToken(t *testing.T) {
	mock := &mockClient{messages: []telegram.Message{{ID: 1}}}
	handler := NewMediaSendAlbumHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken(nil),
		MediaSendAlbumParams{ParseMode: "plain", Peer: "@x", Paths: []string{"/tmp/x"}})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastSendOpts.Progress != nil {
		t.Error("Progress callback must be nil without a progress token")
	}
}

func TestMediaUploadHandler_WiresProgressWhenTokenPresent(t *testing.T) {
	mock := &mockClient{uploaded: &telegram.UploadedFile{Name: "x", Size: 1}}
	handler := NewMediaUploadHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken("tok"),
		MediaUploadParams{Path: "/tmp/x"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastUploadOpts.Progress == nil {
		t.Error("Progress callback must be wired when a progress token is present")
	}
}

func TestMediaUploadHandler_NoProgressWithoutToken(t *testing.T) {
	mock := &mockClient{uploaded: &telegram.UploadedFile{Name: "x", Size: 1}}
	handler := NewMediaUploadHandler(mock)

	_, _, err := handler(context.Background(), requestWithToken(nil),
		MediaUploadParams{Path: "/tmp/x"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if mock.lastUploadOpts.Progress != nil {
		t.Error("Progress callback must be nil without a progress token")
	}
}
