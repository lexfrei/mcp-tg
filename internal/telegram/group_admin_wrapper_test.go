package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// groupInvoker records the group-admin requests and answers each with a
// scripted reply.
type groupInvoker struct {
	banned      []*tg.ChannelsEditBannedRequest
	editPhoto   []*tg.ChannelsEditPhotoRequest
	chatPhoto   []*tg.MessagesEditChatPhotoRequest
	topicDelete int
	offsets     []int
	adminLog    *tg.ChannelsAdminLogResults
	migrate     tg.UpdatesClass
	join        tg.MessagesChatInviteJoinResultClass
}

func (g *groupInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.ChannelsEditBannedRequest:
		g.banned = append(g.banned, req)

		return encodeAndDecode(&tg.Updates{}, output)
	case *tg.ChannelsEditPhotoRequest:
		g.editPhoto = append(g.editPhoto, req)

		return encodeAndDecode(&tg.Updates{}, output)
	case *tg.MessagesEditChatPhotoRequest:
		g.chatPhoto = append(g.chatPhoto, req)

		return encodeAndDecode(&tg.Updates{}, output)
	case *tg.MessagesDeleteTopicHistoryRequest:
		g.topicDelete++
		offset := g.offsets[min(g.topicDelete, len(g.offsets))-1]

		return encodeAndDecode(&tg.MessagesAffectedHistory{Offset: offset}, output)
	case *tg.ChannelsGetAdminLogRequest:
		return encodeAndDecode(g.adminLog, output)
	case *tg.MessagesMigrateChatRequest:
		return encodeAndDecode(g.migrate, output)
	case *tg.MessagesImportChatInviteRequest:
		return encodeAndDecode(g.join, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

var (
	adminGroup  = InputPeer{Type: PeerChannel, ID: 10, AccessHash: 100}
	adminMember = InputPeer{Type: PeerUser, ID: 20, AccessHash: 200}
)

func TestBanMember_SendsTheRightsForBanAndUnban(t *testing.T) {
	invoker := &groupInvoker{}
	wrap := newWrapperWithInvoker(invoker)

	err := wrap.BanMember(t.Context(), adminGroup, adminMember, 1800000000, false)
	if err != nil {
		t.Fatalf("ban: %v", err)
	}

	err = wrap.BanMember(t.Context(), adminGroup, adminMember, 0, true)
	if err != nil {
		t.Fatalf("unban: %v", err)
	}

	ban, unban := invoker.banned[0].BannedRights, invoker.banned[1].BannedRights
	if !ban.ViewMessages || ban.UntilDate != 1800000000 {
		t.Errorf("ban rights = %+v, want view_messages until 1800000000", ban)
	}

	if unban.ViewMessages || unban.UntilDate != 0 {
		t.Errorf("unban rights = %+v, want no restriction", unban)
	}
}

func TestDeleteChatPhoto_PicksTheRequestByPeerKind(t *testing.T) {
	invoker := &groupInvoker{}
	wrap := newWrapperWithInvoker(invoker)

	err := wrap.DeleteChatPhoto(t.Context(), adminGroup)
	if err != nil {
		t.Fatalf("channel: %v", err)
	}

	err = wrap.DeleteChatPhoto(t.Context(), InputPeer{Type: PeerChat, ID: 7})
	if err != nil {
		t.Fatalf("basic group: %v", err)
	}

	if len(invoker.editPhoto) != 1 || len(invoker.chatPhoto) != 1 || invoker.chatPhoto[0].ChatID != 7 {
		t.Errorf("channel requests = %d, chat requests = %+v; want one of each, the chat one for 7",
			len(invoker.editPhoto), invoker.chatPhoto)
	}
}

// A user's ID sent as a basic-group ID could remove another group's photo.
func TestDeleteChatPhoto_RefusesAUser(t *testing.T) {
	invoker := &groupInvoker{}

	err := newWrapperWithInvoker(invoker).DeleteChatPhoto(t.Context(), adminMember)
	if !errors.Is(err, ErrNotAGroup) || len(invoker.chatPhoto)+len(invoker.editPhoto) != 0 {
		t.Errorf("err = %v, requests sent = %d; want ErrNotAGroup and none", err, len(invoker.chatPhoto)+len(invoker.editPhoto))
	}
}

func TestDeleteForumTopic_RepeatsUntilDoneAndStopsAtTheBound(t *testing.T) {
	invoker := &groupInvoker{offsets: []int{4, 2, 0}}

	err := newWrapperWithInvoker(invoker).DeleteForumTopic(t.Context(), adminGroup, 5)
	if err != nil {
		t.Fatalf("DeleteForumTopic: %v", err)
	}

	if invoker.topicDelete != 3 {
		t.Errorf("requests = %d, want 3", invoker.topicDelete)
	}

	endless := &groupInvoker{offsets: []int{1}}

	err = newWrapperWithInvoker(endless).DeleteForumTopic(t.Context(), adminGroup, 5)
	if !errors.Is(err, ErrTopicDeleteIncomplete) || endless.topicDelete != maxTopicDeleteRounds {
		t.Errorf("err = %v after %d requests, want ErrTopicDeleteIncomplete after %d",
			err, endless.topicDelete, maxTopicDeleteRounds)
	}
}

func TestGetAdminLog_MapsTheEvents(t *testing.T) {
	invoker := &groupInvoker{adminLog: &tg.ChannelsAdminLogResults{Events: []tg.ChannelAdminLogEvent{{
		ID: 9007199254740993, Date: 1700000000, UserID: 20,
		Action: &tg.ChannelAdminLogEventActionChangeTitle{PrevValue: "A", NewValue: "B"},
	}}}}

	events, err := newWrapperWithInvoker(invoker).GetAdminLog(t.Context(), adminGroup, AdminLogOpts{Limit: 10})
	if err != nil {
		t.Fatalf("GetAdminLog: %v", err)
	}

	want := AdminLogEvent{ID: 9007199254740993, Date: 1700000000, UserID: 20, Action: "change_title", Detail: "A → B"}
	if len(events) != 1 || events[0] != want {
		t.Errorf("events = %+v, want %+v", events, want)
	}
}

func migratedEnvelope(combined bool, withMigratedTo bool) tg.UpdatesClass {
	old := &tg.Chat{ID: 7, Title: "Old", Photo: &tg.ChatPhotoEmpty{}}
	if withMigratedTo {
		old.SetMigratedTo(&tg.InputChannel{ChannelID: 30, AccessHash: 3})
	}

	chats := []tg.ChatClass{
		old,
		&tg.Channel{ID: 30, AccessHash: 3, Title: "New", Megagroup: true, Photo: &tg.ChatPhotoEmpty{}},
	}

	if combined {
		return &tg.UpdatesCombined{Chats: chats}
	}

	return &tg.Updates{Chats: chats}
}

// The new supergroup is reported once, in either envelope form, and its
// access hash is cached so its numeric ID works afterwards.
func TestUpgradeToSupergroup_FindsTheChannelAndCachesIt(t *testing.T) {
	for name, envelope := range map[string]tg.UpdatesClass{
		"updates, migrated_to":         migratedEnvelope(false, true),
		"updatesCombined, migrated_to": migratedEnvelope(true, true),
		"updates, fallback":            migratedEnvelope(false, false),
	} {
		wrap := newWrapperWithInvoker(&groupInvoker{migrate: envelope})

		info, err := wrap.UpgradeToSupergroup(t.Context(), InputPeer{Type: PeerChat, ID: 7})
		if err != nil || info.Peer.ID != 30 {
			t.Errorf("%s: info = %+v, err = %v; want supergroup 30", name, info, err)

			continue
		}

		if cached, ok := wrap.cache.Lookup(PeerChannel, 30); !ok || cached.AccessHash != 3 {
			t.Errorf("%s: cache = %+v (%v), want the new supergroup's hash", name, cached, ok)
		}
	}
}

func TestUpgradeToSupergroup_NoChannelInTheReplyIsAnError(t *testing.T) {
	wrap := newWrapperWithInvoker(&groupInvoker{migrate: &tg.Updates{}})

	_, err := wrap.UpgradeToSupergroup(t.Context(), InputPeer{Type: PeerChat, ID: 7})
	if !errors.Is(err, ErrNoChatInReply) {
		t.Errorf("err = %v, want ErrNoChatInReply", err)
	}
}

func TestJoinByInvite_ReadsTheChatFromEitherEnvelope(t *testing.T) {
	chats := []tg.ChatClass{&tg.Channel{ID: 30, AccessHash: 3, Title: "Club", Megagroup: true, Photo: &tg.ChatPhotoEmpty{}}}

	for name, envelope := range map[string]tg.UpdatesClass{
		"updates":         &tg.Updates{Chats: chats},
		"updatesCombined": &tg.UpdatesCombined{Chats: chats},
	} {
		wrap := newWrapperWithInvoker(&groupInvoker{join: &tg.MessagesChatInviteJoinResultOk{Updates: envelope}})

		info, err := wrap.JoinByInvite(t.Context(), "+abc")
		if err != nil || info.Peer.ID != 30 {
			t.Errorf("%s: info = %+v, err = %v; want chat 30", name, info, err)
		}
	}

	empty := newWrapperWithInvoker(&groupInvoker{join: &tg.MessagesChatInviteJoinResultOk{Updates: &tg.Updates{}}})

	_, err := empty.JoinByInvite(t.Context(), "+abc")
	if !errors.Is(err, ErrNoChatInReply) {
		t.Errorf("no chat: err = %v, want ErrNoChatInReply", err)
	}
}

func TestJoinByInvite_PaidSubscriptionNeedsTheWebView(t *testing.T) {
	wrap := newWrapperWithInvoker(&groupInvoker{join: &tg.MessagesChatInviteJoinResultWebView{}})

	_, err := wrap.JoinByInvite(t.Context(), "+abc")
	if !errors.Is(err, ErrInviteNeedsWebView) {
		t.Errorf("err = %v, want ErrInviteNeedsWebView", err)
	}
}
