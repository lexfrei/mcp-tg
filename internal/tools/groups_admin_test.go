package tools

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
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
	if err != nil || mock.lastForum == nil || !*mock.lastForum || result.Output != "Forum topics are on in @team" {
		t.Errorf("err = %v, forum = %v, output = %q", err, mock.lastForum, result.Output)
	}
}

func TestGroupsAdminLog_DefaultsLimitAndFormats(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1), adminLog: []telegram.AdminLogEvent{
		{ID: "3", Date: 1700000000, UserID: 5, Action: "change_title", Detail: "A → B"},
	}}

	_, result, err := NewGroupsAdminLogHandler(mock)(context.Background(), nil,
		GroupsAdminLogParams{Peer: "@team", Query: "title", MaxID: "9007199254740993"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2^53+1 survives as a string; as a JSON number it would round.
	if mock.lastAdminLog.Limit != maxAdminLogLimit/2 || mock.lastAdminLog.Query != "title" ||
		mock.lastAdminLog.MaxID != 9007199254740993 {
		t.Errorf("opts = %+v, want the default limit and the exact maxId", mock.lastAdminLog)
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

func TestGroupsAdminLog_RejectsBadPaging(t *testing.T) {
	for name, tc := range map[string]struct {
		params GroupsAdminLogParams
		want   error
	}{
		"limit above 100":    {GroupsAdminLogParams{Peer: "@team", Limit: 101}, ErrAdminLogLimit},
		"negative limit":     {GroupsAdminLogParams{Peer: "@team", Limit: -1}, ErrNegativeLimit},
		"maxId not a number": {GroupsAdminLogParams{Peer: "@team", MaxID: "12x"}, ErrAdminLogMaxID},
		"negative maxId":     {GroupsAdminLogParams{Peer: "@team", MaxID: "-1"}, ErrAdminLogMaxID},
	} {
		mock := &mockClient{peer: channelPeerFor(1)}

		_, _, err := NewGroupsAdminLogHandler(mock)(context.Background(), nil, tc.params)
		if !errors.Is(err, tc.want) || !errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v, want %v as a validation error", name, err, tc.want)
		}
	}
}

// A join request is filed, not failed: a retry would file a second one.
func TestGroupsJoinInvite_RequestSentIsAResult(t *testing.T) {
	_, result, err := NewGroupsJoinInviteHandler(clientFailingWith("INVITE_REQUEST_SENT"))(context.Background(), nil,
		GroupsJoinInviteParams{Link: "+abc"})
	if err != nil || !result.RequestSent || !strings.Contains(result.Output, "pending") {
		t.Errorf("result = %+v, err = %v; want a pending join request", result, err)
	}
}

// An existing membership is reported as the state, naming the chat when the
// invite can be read back.
func TestGroupsJoinInvite_AlreadyMemberNamesTheChat(t *testing.T) {
	club := telegram.InputPeer{Type: telegram.PeerChannel, ID: 30, AccessHash: 3}
	mock := &alreadyMemberClient{mockClient: &mockClient{inviteInfo: &telegram.InviteInfo{
		Title: "Club", Type: telegram.PeerInfoTypeSupergroup, Peer: club, AlreadyJoined: true,
	}}}

	_, result, err := NewGroupsJoinInviteHandler(mock)(context.Background(), nil, GroupsJoinInviteParams{Link: "+abc"})
	if err != nil || result.Output != "Already a member of Club [channel:30]" || result.Chat.Peer != club {
		t.Errorf("result = %+v, err = %v", result, err)
	}
}

// alreadyMemberClient fails the join with USER_ALREADY_PARTICIPANT and
// answers the invite read-back.
type alreadyMemberClient struct {
	*mockClient
}

func (*alreadyMemberClient) JoinByInvite(context.Context, string) (*telegram.PeerInfo, error) {
	return nil, tgerr.New(400, "USER_ALREADY_PARTICIPANT")
}

// The wrapper's input refusals are validation errors, not Telegram failures.
// Each sentinel groupErr admits has a row, so dropping one fails a test.
func TestGroupTools_InputRefusalsAreValidationErrors(t *testing.T) {
	setForum := func(m *mockClient) error {
		_, _, err := NewGroupsSetForumHandler(m)(context.Background(), nil, GroupsSetForumParams{Peer: "@x"})

		return err
	}
	upgrade := func(m *mockClient) error {
		_, _, err := NewGroupsUpgradeHandler(m)(context.Background(), nil, GroupsUpgradeParams{Peer: "@x"})

		return err
	}
	adminLog := func(m *mockClient) error {
		_, _, err := NewGroupsAdminLogHandler(m)(context.Background(), nil, GroupsAdminLogParams{Peer: "@x"})

		return err
	}
	deletePhoto := func(m *mockClient) error {
		_, _, err := NewChatsDeletePhotoHandler(m)(context.Background(), nil, ChatsDeletePhotoParams{Peer: "@x"})

		return err
	}

	for name, tc := range map[string]struct {
		call     func(*mockClient) error
		sentinel error
	}{
		"set_forum, channel only":    {setForum, telegram.ErrChannelOnly},
		"admin_log, channel only":    {adminLog, telegram.ErrChannelOnly},
		"upgrade, basic group only":  {upgrade, telegram.ErrBasicGroupOnly},
		"delete_photo, not a group":  {deletePhoto, telegram.ErrNotAGroup},
		"upgrade, channel only":      {upgrade, telegram.ErrChannelOnly},
		"delete_photo, channel only": {deletePhoto, telegram.ErrChannelOnly},
	} {
		mock := &mockClient{
			resolvePeerFn: func(string) (telegram.InputPeer, error) { return channelPeerFor(1), nil },
			err:           tc.sentinel,
		}

		err := tc.call(mock)
		if !errors.Is(err, ErrValidation) || !errors.Is(err, tc.sentinel) {
			t.Errorf("%s: err = %v, want %v as a validation error", name, err, tc.sentinel)
		}
	}
}

// An upgrade or join whose reply left the chat out did go through: the text
// must say so, not start with "failed to", since an upgrade cannot be redone.
func TestGroupTools_AppliedButUnnamedIsNotReportedAsAFailure(t *testing.T) {
	for name, call := range map[string]func(*mockClient) error{
		"upgrade": func(m *mockClient) error {
			_, _, err := NewGroupsUpgradeHandler(m)(context.Background(), nil, GroupsUpgradeParams{Peer: "@x"})

			return err
		},
		"join_invite": func(m *mockClient) error {
			_, _, err := NewGroupsJoinInviteHandler(m)(context.Background(), nil, GroupsJoinInviteParams{Link: "+abc"})

			return err
		},
	} {
		mock := &mockClient{
			resolvePeerFn: func(string) (telegram.InputPeer, error) {
				return telegram.InputPeer{Type: telegram.PeerChat, ID: 7}, nil
			},
			err: errors.Wrap(telegram.ErrNoChatInReply, "the group was upgraded"),
		}

		err := call(mock)
		if !errors.Is(err, telegram.ErrNoChatInReply) || !errors.Is(err, ErrTelegram) ||
			strings.Contains(err.Error(), "failed to") {
			t.Errorf("%s: err = %q, want ErrNoChatInReply without a \"failed to\" prefix", name, err)
		}
	}
}

// untilDate and topicId are 32-bit on the wire; a larger value would wrap
// into another date or another topic, so it is refused before any request.
func TestGroupTools_RejectValuesOutsideInt32(t *testing.T) {
	mock := &mockClient{peer: channelPeerFor(1)}

	for name, untilDate := range map[string]int{"above int32": math.MaxInt32 + 1, "negative": -1} {
		_, _, err := NewGroupsBanHandler(mock)(context.Background(), nil,
			GroupsBanParams{Group: "@team", User: "@spammer", UntilDate: untilDate})
		if !errors.Is(err, ErrUntilDateOutOfRange) || !errors.Is(err, ErrValidation) {
			t.Errorf("ban untilDate %s: err = %v, want ErrUntilDateOutOfRange", name, err)
		}
	}

	_, _, err := NewTopicsDeleteHandler(mock)(context.Background(), nil,
		TopicsDeleteParams{Peer: "@team", TopicID: 4294967297})
	if !errors.Is(err, ErrTopicIDOutOfRange) || !errors.Is(err, ErrValidation) {
		t.Errorf("topicId 4294967297: err = %v, want ErrTopicIDOutOfRange", err)
	}

	if mock.lastBan[0] != (telegram.InputPeer{}) || mock.deletedTopic != 0 {
		t.Errorf("an out-of-range value reached the client: ban %+v, topic %d", mock.lastBan, mock.deletedTopic)
	}
}

func TestGroupTools_RejectEmptyPeerAndPassTelegramErrors(t *testing.T) {
	errBoom := errors.New("boom")

	calls := map[string]func(*mockClient, string) error{
		"set_forum": func(m *mockClient, peer string) error {
			_, _, err := NewGroupsSetForumHandler(m)(context.Background(), nil, GroupsSetForumParams{Peer: peer})

			return err
		},
		"admin_log": func(m *mockClient, peer string) error {
			_, _, err := NewGroupsAdminLogHandler(m)(context.Background(), nil, GroupsAdminLogParams{Peer: peer})

			return err
		},
		"upgrade": func(m *mockClient, peer string) error {
			_, _, err := NewGroupsUpgradeHandler(m)(context.Background(), nil, GroupsUpgradeParams{Peer: peer})

			return err
		},
		"delete_photo": func(m *mockClient, peer string) error {
			_, _, err := NewChatsDeletePhotoHandler(m)(context.Background(), nil, ChatsDeletePhotoParams{Peer: peer})

			return err
		},
		"join_invite": func(m *mockClient, link string) error {
			_, _, err := NewGroupsJoinInviteHandler(m)(context.Background(), nil, GroupsJoinInviteParams{Link: link})

			return err
		},
	}

	for name, call := range calls {
		err := call(&mockClient{}, "")
		if !errors.Is(err, ErrValidation) {
			t.Errorf("%s with no peer: err = %v, want a validation error", name, err)
		}

		failing := &mockClient{
			resolvePeerFn: func(string) (telegram.InputPeer, error) { return channelPeerFor(1), nil },
			err:           errBoom,
		}

		err = call(failing, "@x")
		if !errors.Is(err, errBoom) || errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v, want the Telegram error", name, err)
		}
	}
}
