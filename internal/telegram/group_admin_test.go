package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func TestInviteHash_AcceptsEverySpelling(t *testing.T) {
	for _, link := range []string{
		"https://t.me/+AbCd123",
		"t.me/+AbCd123",
		"https://t.me/joinchat/AbCd123",
		"+AbCd123",
		"AbCd123",
		"  https://t.me/+AbCd123 ",
	} {
		if got := inviteHash(link); got != "AbCd123" {
			t.Errorf("inviteHash(%q) = %q, want AbCd123", link, got)
		}
	}
}

func TestInviteInfo(t *testing.T) {
	preview := inviteInfo(&tg.ChatInvite{
		Title: "Club", About: "Members only", Megagroup: true, ParticipantsCount: 12, RequestNeeded: true,
	})
	if preview.Title != "Club" || preview.Type != PeerInfoTypeSupergroup || preview.ParticipantsCount != 12 ||
		!preview.RequestNeeded || preview.AlreadyJoined {
		t.Errorf("preview = %+v", preview)
	}

	joined := inviteInfo(&tg.ChatInviteAlready{Chat: &tg.Channel{ID: 5, AccessHash: 9, Title: "News", Broadcast: true}})
	if !joined.AlreadyJoined || joined.Type != PeerInfoTypeChannel || joined.Peer.ID != 5 || joined.Peer.AccessHash != 9 {
		t.Errorf("joined = %+v", joined)
	}
}

// The migration envelope carries the old chat pointing at its successor; the
// successor is the answer even when other channels ride along.
func TestMigratedChannel_FollowsMigratedTo(t *testing.T) {
	old := &tg.Chat{ID: 7, Title: "Old"}
	old.SetMigratedTo(&tg.InputChannel{ChannelID: 30, AccessHash: 3})

	updates := &tg.Updates{Chats: []tg.ChatClass{
		&tg.Channel{ID: 20, AccessHash: 2, Title: "Unrelated", Megagroup: true},
		old,
		&tg.Channel{ID: 30, AccessHash: 3, Title: "Old", Megagroup: true},
	}}

	got, found := migratedChannel(updates, 7)
	if !found || got.Peer.ID != 30 || got.Peer.Type != PeerChannel || got.Type != PeerInfoTypeSupergroup {
		t.Fatalf("migratedChannel = %+v, %v; want channel 30", got, found)
	}
}

func TestAdminLogActionName(t *testing.T) {
	cases := map[string]tg.ChannelAdminLogEventActionClass{
		"change_title":           &tg.ChannelAdminLogEventActionChangeTitle{},
		"participant_toggle_ban": &tg.ChannelAdminLogEventActionParticipantToggleBan{},
		"delete_message":         &tg.ChannelAdminLogEventActionDeleteMessage{},
		"unrecognized":           nil,
	}

	for want, action := range cases {
		if got := adminLogActionName(action); got != want {
			t.Errorf("adminLogActionName(%T) = %q, want %q", action, got, want)
		}
	}

	detail := adminLogDetail(&tg.ChannelAdminLogEventActionChangeTitle{PrevValue: "A", NewValue: "B"})
	if detail != "A → B" {
		t.Errorf("detail = %q, want A → B", detail)
	}
}

// Bans, forum mode and the admin log exist only on channels; a basic group
// must fail before a round trip with the reason, and upgrading anything but
// a basic group likewise.
func TestGroupAdmin_RejectsWrongPeerKinds(t *testing.T) {
	wrapper := &Wrapper{}
	ctx := context.Background()
	chat := InputPeer{Type: PeerChat, ID: 7}
	channel := InputPeer{Type: PeerChannel, ID: 8}

	_, logErr := wrapper.GetAdminLog(ctx, chat, AdminLogOpts{})
	_, upgradeErr := wrapper.UpgradeToSupergroup(ctx, channel)

	for name, pair := range map[string][2]error{
		"ban":       {wrapper.BanMember(ctx, chat, InputPeer{ID: 1}, 0, false), ErrChannelOnly},
		"forum":     {wrapper.SetForum(ctx, chat, true), ErrChannelOnly},
		"admin log": {logErr, ErrChannelOnly},
		"upgrade":   {upgradeErr, ErrBasicGroupOnly},
	} {
		if !errors.Is(pair[0], pair[1]) {
			t.Errorf("%s: err = %v, want %v", name, pair[0], pair[1])
		}
	}
}
