package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// Omitted peers keep the folder's chats: the client gets nil.
func TestFoldersEditHandler_OmittedPeersPassNil(t *testing.T) {
	mock := &mockClient{}

	_, _, err := NewFoldersEditHandler(mock)(context.Background(), nil, FoldersEditParams{FolderID: 7, Title: "Work"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.editFolderCalls != 1 || mock.editFolderPeers != nil {
		t.Errorf("calls = %d, peers = %#v; want one call with nil", mock.editFolderCalls, mock.editFolderPeers)
	}
}

// peers: [] empties the folder: the client gets an empty, non-nil list.
func TestFoldersEditHandler_EmptyPeersPassAnEmptyList(t *testing.T) {
	mock := &mockClient{}

	_, _, err := NewFoldersEditHandler(mock)(context.Background(), nil,
		FoldersEditParams{FolderID: 7, Title: "Work", Peers: []string{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.editFolderCalls != 1 || mock.editFolderPeers == nil || len(mock.editFolderPeers) != 0 {
		t.Errorf("calls = %d, peers = %#v; want one call with an empty non-nil list",
			mock.editFolderCalls, mock.editFolderPeers)
	}
}

// A peer that does not resolve fails the edit before anything is written.
func TestFoldersEditHandler_UnresolvablePeerWritesNothing(t *testing.T) {
	errResolve := errors.New("no such peer")
	mock := &mockClient{resolvePeerFn: func(string) (telegram.InputPeer, error) {
		return telegram.InputPeer{}, errResolve
	}}

	result, _, err := NewFoldersEditHandler(mock)(context.Background(), nil,
		FoldersEditParams{FolderID: 7, Title: "Work", Peers: []string{"@nobody"}})
	if !errors.Is(err, errResolve) || result == nil || !result.IsError {
		t.Errorf("result = %+v, err = %v; want the resolve error", result, err)
	}

	if mock.editFolderCalls != 0 {
		t.Errorf("EditFolder ran %d time(s) after a failed resolve", mock.editFolderCalls)
	}
}
