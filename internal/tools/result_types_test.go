package tools

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestParticipantTypeLabel_UnknownSurfacesAsUnknown(t *testing.T) {
	got := participantTypeLabel(telegram.PeerType(99))

	if got != unknownPeerType {
		t.Errorf("participantTypeLabel(unknown) = %q, want %q — must mirror peerLabel's default to keep text and JSON consistent",
			got, unknownPeerType)
	}
}

func TestParticipantsFromMessages_SupergroupSenderLabelsAsChannel(t *testing.T) {
	// gotd represents supergroups as PeerChannel — verify the
	// participants entry uses "channel" (not "group") for a supergroup
	// sender, so the docstring claim stays honest.
	msgs := []telegram.Message{
		{
			ID: 1, FromID: 1234567890, FromType: telegram.PeerChannel,
			FromName: "Cozystack Discussion",
		},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Fatalf("got %d participants, want 1", len(parts))
	}

	if parts[0].Type != peerChannel {
		t.Errorf("supergroup sender type = %q, want %q — gotd folds supergroups into PeerChannel",
			parts[0].Type, peerChannel)
	}
}

func TestMessageToItem_FromTypeChannel(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000000,
		FromID:   500,
		FromType: telegram.PeerChannel,
		FromName: "Example Channel",
		Text:     "anonymous post",
	}

	item := messageToItem(msg)

	if item.FromType != peerChannel {
		t.Errorf("item.FromType = %q, want %q — channel-shaped sender must surface in JSON",
			item.FromType, peerChannel)
	}
}

func TestMessageToItem_FromTypeOmittedForUnknownSender(t *testing.T) {
	msg := &telegram.Message{ID: 7, Date: 1700000000} // FromID == 0
	item := messageToItem(msg)

	if item.FromType != "" {
		t.Errorf("item.FromType = %q, want empty when FromID==0", item.FromType)
	}
}

func TestParticipantsFromMessages_SenderWithUsername(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 10, FromType: telegram.PeerUser, FromName: "Alice", FromUsername: "alice"},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Fatalf("got %d participants, want 1", len(parts))
	}

	got := parts[0]
	if got.ID != 10 || got.Type != peerUser || got.Name != "Alice" || got.Username != "alice" {
		t.Errorf("participant = %+v, want {10 user Alice alice}", got)
	}
}

func TestParticipantsFromMessages_UserAndChannelSharedIDAreDistinct(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 500, FromType: telegram.PeerUser, FromName: "Alice"},
		{
			ID: 2, FromID: 600, FromType: telegram.PeerUser, FromName: "Bob",
			Forward: &telegram.ForwardInfo{
				From: &telegram.PeerRef{
					Peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
					Name: "Coincident Channel",
				},
			},
		},
	}

	parts := participantsFromMessages(msgs)

	var (
		sawUser    bool
		sawChannel bool
	)

	for _, part := range parts {
		if part.ID == 500 && part.Type == peerUser {
			sawUser = true
		}

		if part.ID == 500 && part.Type == peerChannel {
			sawChannel = true
		}
	}

	if !sawUser || !sawChannel {
		t.Errorf("user 500 and channel 500 must survive dedup as distinct entries; got %+v", parts)
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
				FromName: "Privacy Hidden Author",
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

func TestParticipantsFromMessages_LatestNameWinsForRename(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 42, FromType: telegram.PeerUser, FromName: "Old Name", FromUsername: "olduser"},
		{
			ID: 2, FromID: 42, FromType: telegram.PeerUser,
			FromName: "New Name", FromUsername: "newuser",
		},
	}

	parts := participantsFromMessages(msgs)

	if len(parts) != 1 {
		t.Fatalf("got %d participants, want 1 (dedup)", len(parts))
	}

	if parts[0].Name != "New Name" || parts[0].Username != "newuser" {
		t.Errorf("participant = %+v, want {New Name, newuser} — must mirror buildSenderLookup's last-non-empty-wins",
			parts[0])
	}
}

func TestParticipantsFromMessages_EmptyLaterValueDoesNotOverwrite(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, FromID: 42, FromType: telegram.PeerUser, FromName: "Real Name", FromUsername: "real"},
		{ID: 2, FromID: 42, FromType: telegram.PeerUser}, // empty
	}

	parts := participantsFromMessages(msgs)

	if parts[0].Name != "Real Name" || parts[0].Username != "real" {
		t.Errorf("empty later entry erased preset Name/Username → %+v", parts[0])
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
