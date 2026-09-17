package telegram

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

const (
	inviteChannelID   = 7001
	inviteChannelHash = 7002
	inviteChatID      = 7003
	invitePrimaryLink = "https://t.me/+primary"
)

// fullChatInviteInvoker answers the two full-chat reads and nothing else.
// messages.exportChatInvite therefore lands in the default branch, which is
// what pins the read: a lookup that creates a link cannot pass.
type fullChatInviteInvoker struct {
	calls atomic.Int32
	full  tg.ChatFullClass
}

func (f *fullChatInviteInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	f.calls.Add(1)

	switch input.(type) {
	case *tg.ChannelsGetFullChannelRequest, *tg.MessagesGetFullChatRequest:
		return encodeResp(&tg.MessagesChatFull{FullChat: f.full}, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

// channelFullWithInvite builds the shape channels.getFullChannel answers with.
// ChatPhoto is not a conditional field, so a nil there fails encoding rather
// than decoding as absent.
func channelFullWithInvite(invite tg.ExportedChatInviteClass) *tg.ChannelFull {
	full := &tg.ChannelFull{ID: inviteChannelID, ChatPhoto: &tg.PhotoEmpty{}}
	if invite != nil {
		full.SetExportedInvite(invite)
	}

	return full
}

// chatFullWithInvite is the basic-group counterpart; Participants is the
// required interface field there.
func chatFullWithInvite(invite tg.ExportedChatInviteClass) *tg.ChatFull {
	full := &tg.ChatFull{
		ID:           inviteChatID,
		Participants: &tg.ChatParticipantsForbidden{ChatID: inviteChatID},
	}
	if invite != nil {
		full.SetExportedInvite(invite)
	}

	return full
}

func TestGetInviteLink_ReadsTheFullChatAndNeverExports(t *testing.T) {
	invoker := &fullChatInviteInvoker{
		full: channelFullWithInvite(&tg.ChatInviteExported{Link: invitePrimaryLink, Permanent: true}),
	}
	wrap := NewWrapper(tg.NewClient(invoker))

	peer := InputPeer{Type: PeerChannel, ID: inviteChannelID, AccessHash: inviteChannelHash}

	got, err := wrap.GetInviteLink(t.Context(), peer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != invitePrimaryLink {
		t.Errorf("link = %q, want %q", got, invitePrimaryLink)
	}
}

func TestGetInviteLink_BasicGroupReadsChatFull(t *testing.T) {
	invoker := &fullChatInviteInvoker{
		full: chatFullWithInvite(&tg.ChatInviteExported{Link: invitePrimaryLink, Permanent: true}),
	}
	wrap := NewWrapper(tg.NewClient(invoker))

	got, err := wrap.GetInviteLink(t.Context(), InputPeer{Type: PeerChat, ID: inviteChatID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != invitePrimaryLink {
		t.Errorf("link = %q, want %q", got, invitePrimaryLink)
	}
}

func TestGetInviteLink_AbsentInviteNamesTheRemedy(t *testing.T) {
	invoker := &fullChatInviteInvoker{full: channelFullWithInvite(nil)}
	wrap := NewWrapper(tg.NewClient(invoker))

	peer := InputPeer{Type: PeerChannel, ID: inviteChannelID, AccessHash: inviteChannelHash}

	_, err := wrap.GetInviteLink(t.Context(), peer)
	if !errors.Is(err, ErrNoPrimaryInviteLink) {
		t.Fatalf("error = %v, want ErrNoPrimaryInviteLink", err)
	}
}

// The linkless constructor must not share the rights message: an admin who
// already holds the invite-users right would be sent to check their rights.
func TestGetInviteLink_PublicJoinRequestsIsItsOwnAnswer(t *testing.T) {
	invoker := &fullChatInviteInvoker{
		full: channelFullWithInvite(&tg.ChatInvitePublicJoinRequests{}),
	}
	wrap := NewWrapper(tg.NewClient(invoker))

	peer := InputPeer{Type: PeerChannel, ID: inviteChannelID, AccessHash: inviteChannelHash}

	_, err := wrap.GetInviteLink(t.Context(), peer)
	if !errors.Is(err, ErrInviteLinkIsJoinRequestOnly) {
		t.Fatalf("error = %v, want ErrInviteLinkIsJoinRequestOnly", err)
	}
}

func TestGetInviteLink_RefusesAUserPeerBeforeAnyRequest(t *testing.T) {
	invoker := &fullChatInviteInvoker{full: channelFullWithInvite(nil)}
	wrap := NewWrapper(tg.NewClient(invoker))

	_, err := wrap.GetInviteLink(t.Context(), InputPeer{Type: PeerUser, ID: 42, AccessHash: 43})
	if !errors.Is(err, ErrNotAGroupPeer) {
		t.Fatalf("error = %v, want ErrNotAGroupPeer", err)
	}

	if got := invoker.calls.Load(); got != 0 {
		t.Errorf("requests sent = %d, want 0", got)
	}
}

// The guard lives in fullChat, so it covers the group-info read too — which is
// where the same user-peer hazard was worked around at a call site instead.
func TestGetGroupInfo_RefusesAUserPeerBeforeAnyRequest(t *testing.T) {
	invoker := &fullChatInviteInvoker{full: channelFullWithInvite(nil)}
	wrap := NewWrapper(tg.NewClient(invoker))

	_, err := wrap.GetGroupInfo(t.Context(), InputPeer{Type: PeerUser, ID: 42, AccessHash: 43})
	if !errors.Is(err, ErrNotAGroupPeer) {
		t.Fatalf("error = %v, want ErrNotAGroupPeer", err)
	}

	if got := invoker.calls.Load(); got != 0 {
		t.Errorf("requests sent = %d, want 0", got)
	}
}
