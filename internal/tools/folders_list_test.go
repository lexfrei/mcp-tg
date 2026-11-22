package tools

import (
	"slices"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// peers is the chat list tg_folders_edit takes back, so it must hold the
// pinned chats too; pinnedPeers only says which of them are pinned.
func TestFolderToItem_PeersIncludeThePinned(t *testing.T) {
	item := folderToItem(&telegram.Folder{
		ID:          7,
		Title:       "Work",
		PinnedPeers: []telegram.InputPeer{{Type: telegram.PeerChannel, ID: 10, AccessHash: 100}},
		Peers:       []telegram.InputPeer{{Type: telegram.PeerUser, ID: 20, AccessHash: 200}},
	})

	want := []string{"-1000000000010", "20"}
	if !slices.Equal(item.PeerList, want) {
		t.Errorf("peers = %v, want %v (pinned first, then included)", item.PeerList, want)
	}

	if !slices.Equal(item.PinnedPeers, []string{"-1000000000010"}) || item.Peers != 2 {
		t.Errorf("pinnedPeers = %v, peerCount = %d; want [-10010] and 2", item.PinnedPeers, item.Peers)
	}
}
