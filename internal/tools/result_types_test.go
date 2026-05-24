package tools

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestParticipantsFromMessages_SenderWithUsername(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 10, FromName: "Alice", FromUsername: "alice"},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Fatalf("got %d participants, want 1", len(parts))
	}

	if parts[0].ID != 10 || parts[0].Name != "Alice" || parts[0].Username != "alice" {
		t.Errorf("participant = %+v, want {10 Alice alice}", parts[0])
	}
}

func TestParticipantsFromMessages_IncludesForwardAuthor(t *testing.T) {
	msgs := []telegram.Message{
		{
			ID: 1, FromID: 10, FromName: "Forwarder", FromUsername: "forw",
			Forward: &telegram.ForwardInfo{
				From: &telegram.PeerRef{
					Peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 99},
					Name:     "Original Author",
					Username: "orig",
				},
			},
		},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 2 {
		t.Fatalf("got %d participants, want 2 (sender + forward author)", len(parts))
	}

	found := make(map[int64]ParticipantItem, len(parts))
	for _, part := range parts {
		found[part.ID] = part
	}

	if got := found[99]; got.Name != "Original Author" || got.Username != "orig" {
		t.Errorf("forward author entry = %+v, want {99 Original Author orig}", got)
	}
}

func TestParticipantsFromMessages_DedupesSenderAndForwardAuthor(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 42, FromName: "Solo", FromUsername: "solo"},
		{
			ID: 2, FromID: 42, FromName: "Solo", FromUsername: "solo",
			Forward: &telegram.ForwardInfo{
				From: &telegram.PeerRef{
					Peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 42},
					Name: "Solo", Username: "solo",
				},
			},
		},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Errorf("got %d participants, want 1 — same ID must be deduplicated across sender and forward-author roles",
			len(parts))
	}
}

func TestParticipantsFromMessages_SkipsPrivacyHiddenForwardAuthor(t *testing.T) {
	msgs := []telegram.Message{
		{
			ID: 1, FromID: 10, FromName: "Forwarder",
			Forward: &telegram.ForwardInfo{
				FromName: "Kaidxen",
			},
		},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Fatalf("got %d participants, want 1 — privacy-hidden forward authors have no resolvable peer ID",
			len(parts))
	}

	if parts[0].ID != 10 {
		t.Errorf("only participant ID = %d, want 10 (the forwarder)", parts[0].ID)
	}
}

func TestParticipantsFromMessages_SkipsZeroFromID(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 0, FromName: "Anon Channel Post"},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 0 {
		t.Errorf("got %d participants, want 0 — FromID==0 means 'no identifiable sender'", len(parts))
	}
}
