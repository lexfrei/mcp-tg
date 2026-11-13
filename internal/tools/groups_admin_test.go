package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestGroupsBan(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1)}
	handler := NewGroupsBanHandler(mock)

	_, result, err := handler(context.Background(), nil, GroupsBanParams{Group: "@team", User: "@spammer", UntilDate: 1700000000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastUnban || mock.lastBanUntil != 1700000000 || !strings.HasPrefix(result.Output, "Banned @spammer until") {
		t.Errorf("unban = %v, until = %d, output = %q", mock.lastUnban, mock.lastBanUntil, result.Output)
	}

	_, result, err = handler(context.Background(), nil, GroupsBanParams{Group: "@team", User: "@spammer", Unban: new(true)})
	if err != nil || !mock.lastUnban || result.Output != "Unbanned @spammer" {
		t.Errorf("unban: err = %v, unban = %v, output = %q", err, mock.lastUnban, result.Output)
	}

	_, _, err = handler(context.Background(), nil, GroupsBanParams{Group: "@team"})
	if !errors.Is(err, ErrUserRequired) {
		t.Errorf("missing user: err = %v, want ErrUserRequired", err)
	}
}

func TestGroupsInviteCheck(t *testing.T) {
	mock := &mockClient{inviteInfo: &telegram.InviteInfo{Title: "Club", Type: "supergroup", ParticipantsCount: 12, RequestNeeded: true}}

	_, result, err := NewGroupsInviteCheckHandler(mock)(context.Background(), nil,
		GroupsInviteCheckParams{Link: "https://t.me/+abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastInviteLink != "https://t.me/+abc" || !strings.Contains(result.Output, "needs admin approval") {
		t.Errorf("link = %q, output = %q", mock.lastInviteLink, result.Output)
	}

	_, _, err = NewGroupsInviteCheckHandler(mock)(context.Background(), nil, GroupsInviteCheckParams{})
	if !errors.Is(err, ErrInviteLinkRequired) {
		t.Errorf("empty link: err = %v", err)
	}
}

func TestGroupsJoinInviteAndUpgrade(t *testing.T) {
	joined := &telegram.PeerInfo{Peer: channelPeerFor(30), Title: "Club", Type: "supergroup"}
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChat, ID: 7}, info: joined}

	_, join, err := NewGroupsJoinInviteHandler(mock)(context.Background(), nil, GroupsJoinInviteParams{Link: "+abc"})
	if err != nil || join.Chat.Peer.ID != 30 || !strings.Contains(join.Output, "Club") {
		t.Errorf("join: err = %v, result = %+v", err, join)
	}

	_, upgrade, err := NewGroupsUpgradeHandler(mock)(context.Background(), nil, GroupsUpgradeParams{Peer: "-7"})
	if err != nil || upgrade.Supergroup.Peer.ID != 30 || mock.lastPeer.ID != 7 {
		t.Errorf("upgrade: err = %v, result = %+v, peer = %+v", err, upgrade, mock.lastPeer)
	}
}

func TestGroupsSetForum(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1)}

	_, result, err := NewGroupsSetForumHandler(mock)(context.Background(), nil, GroupsSetForumParams{Peer: "@team", Enabled: true})
	if err != nil || mock.lastForum == nil || !*mock.lastForum || result.Output != "Forum topics turned on" {
		t.Errorf("err = %v, forum = %v, output = %q", err, mock.lastForum, result.Output)
	}
}

func TestGroupsAdminLog_ClampsLimitAndFormats(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1), adminLog: []telegram.AdminLogEvent{
		{ID: 3, Date: 1700000000, UserID: 5, Action: "change_title", Detail: "A → B"},
	}}

	_, result, err := NewGroupsAdminLogHandler(mock)(context.Background(), nil,
		GroupsAdminLogParams{Peer: "@team", Limit: 500, Query: "title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastAdminLog.Limit != maxAdminLogLimit/2 || mock.lastAdminLog.Query != "title" {
		t.Errorf("opts = %+v, want the default limit for an out-of-range value", mock.lastAdminLog)
	}

	if !strings.Contains(result.Output, "change_title — A → B") {
		t.Errorf("output = %q", result.Output)
	}
}

func TestChatsDeletePhotoAndTopicsDelete(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1)}

	_, _, err := NewChatsDeletePhotoHandler(mock)(context.Background(), nil, ChatsDeletePhotoParams{Peer: "@team"})
	if err != nil || !mock.photoDeleted {
		t.Errorf("delete photo: err = %v, deleted = %v", err, mock.photoDeleted)
	}

	_, _, err = NewTopicsDeleteHandler(mock)(context.Background(), nil, TopicsDeleteParams{Peer: "@team", TopicID: 4})
	if err != nil || mock.deletedTopic != 4 {
		t.Errorf("delete topic: err = %v, topic = %d", err, mock.deletedTopic)
	}

	_, _, err = NewTopicsDeleteHandler(mock)(context.Background(), nil, TopicsDeleteParams{Peer: "@team"})
	if !errors.Is(err, ErrTopicIDRequired) {
		t.Errorf("missing topic: err = %v", err)
	}
}

func channelPeerFor(id int64) telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerChannel, ID: id, AccessHash: 1}
}
