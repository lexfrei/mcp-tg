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

// createInviteInvoker captures the export request and answers with a canned
// invite, so the assertions are about what went on the wire.
type createInviteInvoker struct {
	req    *tg.MessagesExportChatInviteRequest
	invite tg.ExportedChatInviteClass
}

func (c *createInviteInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesExportChatInviteRequest)
	if !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	c.req = req

	return encodeResp(c.invite, output)
}

func newCreateInviteWrapper(invite tg.ExportedChatInviteClass) (*Wrapper, *createInviteInvoker) {
	invoker := &createInviteInvoker{invite: invite}

	return NewWrapper(tg.NewClient(invoker)), invoker
}

func inviteChannelPeer() InputPeer {
	return InputPeer{Type: PeerChannel, ID: inviteChannelID, AccessHash: inviteChannelHash}
}

func TestCreateInviteLink_DefaultsLeaveConditionalFieldsUnset(t *testing.T) {
	wrap, invoker := newCreateInviteWrapper(&tg.ChatInviteExported{Link: invitePrimaryLink})

	_, err := wrap.CreateInviteLink(t.Context(), inviteChannelPeer(), InviteLinkOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := invoker.req.GetTitle(); ok {
		t.Error("title must stay off the wire when no title was asked for")
	}

	if _, ok := invoker.req.GetExpireDate(); ok {
		t.Error("expire date must stay off the wire when none was asked for")
	}

	if _, ok := invoker.req.GetUsageLimit(); ok {
		t.Error("usage limit must stay off the wire when none was asked for")
	}
}

func TestCreateInviteLink_SetsEveryOptionalField(t *testing.T) {
	wrap, invoker := newCreateInviteWrapper(&tg.ChatInviteExported{Link: invitePrimaryLink})

	opts := InviteLinkOpts{Title: "Conference", ExpireDate: 1800000000, UsageLimit: 50, RequestNeeded: true}

	_, err := wrap.CreateInviteLink(t.Context(), inviteChannelPeer(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, _ := invoker.req.GetTitle(); got != opts.Title {
		t.Errorf("title = %q, want %q", got, opts.Title)
	}

	if got, _ := invoker.req.GetExpireDate(); got != opts.ExpireDate {
		t.Errorf("expire date = %d, want %d", got, opts.ExpireDate)
	}

	if got, _ := invoker.req.GetUsageLimit(); got != opts.UsageLimit {
		t.Errorf("usage limit = %d, want %d", got, opts.UsageLimit)
	}

	if !invoker.req.GetRequestNeeded() {
		t.Error("request_needed must reach the wire")
	}
}

// legacy_revoke_permanent replaces the chat's primary link and revokes every
// earlier one, so setting it here would quietly undo the read this tool family
// is built on.
func TestCreateInviteLink_NeverSetsLegacyRevokePermanent(t *testing.T) {
	wrap, invoker := newCreateInviteWrapper(&tg.ChatInviteExported{Link: invitePrimaryLink})

	opts := InviteLinkOpts{Title: "Conference", ExpireDate: 1800000000, UsageLimit: 50, RequestNeeded: true}

	_, err := wrap.CreateInviteLink(t.Context(), inviteChannelPeer(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if invoker.req.LegacyRevokePermanent {
		t.Error("legacy_revoke_permanent must never be set: it revokes every existing link")
	}
}

func TestCreateInviteLink_ReturnsTheServerEcho(t *testing.T) {
	echo := &tg.ChatInviteExported{Link: invitePrimaryLink, AdminID: 99, Date: 1700000000}
	echo.SetTitle("Conference")
	echo.SetUsageLimit(50)
	echo.SetUsage(3)

	wrap, _ := newCreateInviteWrapper(echo)

	got, err := wrap.CreateInviteLink(t.Context(), inviteChannelPeer(), InviteLinkOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := InviteLink{
		Link: invitePrimaryLink, Title: "Conference", AdminID: 99,
		Date: 1700000000, UsageLimit: 50, Usage: 3,
	}
	if *got != want {
		t.Errorf("link = %+v, want %+v", *got, want)
	}
}

func TestCreateInviteLink_RefusesAUserPeer(t *testing.T) {
	wrap, invoker := newCreateInviteWrapper(&tg.ChatInviteExported{Link: invitePrimaryLink})

	_, err := wrap.CreateInviteLink(t.Context(), InputPeer{Type: PeerUser, ID: 42}, InviteLinkOpts{})
	if !errors.Is(err, ErrNotAGroupPeer) {
		t.Fatalf("error = %v, want ErrNotAGroupPeer", err)
	}

	if invoker.req != nil {
		t.Error("a user peer must be refused before the round trip")
	}
}
