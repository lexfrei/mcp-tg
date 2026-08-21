package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestDeletedScheduledCount_CountsTheDeletedIDs(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDeleteScheduledMessages{
				Peer:     &tg.PeerChannel{ChannelID: 55},
				Messages: []int{1, 2, 3},
			},
		},
	}

	if got := deletedScheduledCount(updates, peer); got != 3 {
		t.Errorf("deletedScheduledCount() = %d, want 3", got)
	}
}

// Deleting IDs Telegram never had in the queue leaves it unchanged: no
// update fires, so the count reflects that instead of the request size.
func TestDeletedScheduledCount_NoMatchingUpdateIsZero(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	updates := &tg.Updates{Updates: []tg.UpdateClass{}}

	if got := deletedScheduledCount(updates, peer); got != 0 {
		t.Errorf("deletedScheduledCount() = %d, want 0 for an empty envelope", got)
	}
}

// The envelope is not guaranteed to carry only the update this call asked
// for; an entry for a different peer must not inflate this call's count.
func TestDeletedScheduledCount_IgnoresAnotherPeersUpdate(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDeleteScheduledMessages{
				Peer:     &tg.PeerChannel{ChannelID: 999},
				Messages: []int{1, 2},
			},
		},
	}

	if got := deletedScheduledCount(updates, peer); got != 0 {
		t.Errorf("deletedScheduledCount() = %d, want 0 for a stranger peer's update", got)
	}
}

// An update whose SentMessages is set reports scheduled messages that
// fired on their own schedule, not ones this call deleted.
func TestDeletedScheduledCount_ExcludesSentMessages(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	sent := &tg.UpdateDeleteScheduledMessages{
		Peer:     &tg.PeerChannel{ChannelID: 55},
		Messages: []int{7},
	}
	sent.SetSentMessages([]int{700})

	updates := &tg.Updates{Updates: []tg.UpdateClass{sent}}

	if got := deletedScheduledCount(updates, peer); got != 0 {
		t.Errorf("deletedScheduledCount() = %d, want 0 for a sent-not-deleted update", got)
	}
}

func TestDeletedScheduledCount_SumsMultipleMatchingUpdates(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDeleteScheduledMessages{
				Peer:     &tg.PeerChannel{ChannelID: 55},
				Messages: []int{1},
			},
			&tg.UpdateDeleteScheduledMessages{
				Peer:     &tg.PeerChannel{ChannelID: 55},
				Messages: []int{2, 3},
			},
		},
	}

	if got := deletedScheduledCount(updates, peer); got != 3 {
		t.Errorf("deletedScheduledCount() = %d, want 3 across two matching updates", got)
	}
}

// A single deleteScheduledMessages update carries no Users/Chats to report,
// which is exactly the shape the server may compact into *tg.UpdateShort —
// unlike unwrapUpdates's callers, this helper needs no enrichment data, so
// it must not reject the compact form the way unwrapUpdates deliberately
// does.
func TestDeletedScheduledCount_HandlesUpdateShort(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}
	short := &tg.UpdateShort{
		Update: &tg.UpdateDeleteScheduledMessages{
			Peer:     &tg.PeerChannel{ChannelID: 55},
			Messages: []int{1, 2},
		},
	}

	if got := deletedScheduledCount(short, peer); got != 2 {
		t.Errorf("deletedScheduledCount() = %d, want 2 for a compact UpdateShort envelope", got)
	}
}

func TestDeletedScheduledCount_UnreadableEnvelopeIsZero(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 55}

	if got := deletedScheduledCount(&tg.UpdatesTooLong{}, peer); got != 0 {
		t.Errorf("deletedScheduledCount() = %d, want 0 for an unreadable envelope", got)
	}
}
