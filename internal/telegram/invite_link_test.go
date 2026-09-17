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

// listInviteInvoker captures the list request and answers with a canned page.
type listInviteInvoker struct {
	req      *tg.MessagesGetExportedChatInvitesRequest
	response *tg.MessagesExportedChatInvites
}

func (l *listInviteInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetExportedChatInvitesRequest)
	if !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	l.req = req

	return encodeResp(l.response, output)
}

func newListInviteWrapper(response *tg.MessagesExportedChatInvites) (*Wrapper, *listInviteInvoker) {
	invoker := &listInviteInvoker{response: response}

	return NewWrapper(tg.NewClient(invoker)), invoker
}

// admin_id is mandatory, so the only question is WHOSE links are asked for.
// Anything but self answers about an administrator the caller cannot speak for.

func TestListInviteLinks_AsksForItsOwnLinks(t *testing.T) {
	wrap, invoker := newListInviteWrapper(&tg.MessagesExportedChatInvites{Count: 0})

	_, _, err := wrap.ListInviteLinks(t.Context(), inviteChannelPeer(), false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := invoker.req.AdminID.(*tg.InputUserSelf); !ok {
		t.Errorf("admin_id = %T, want *tg.InputUserSelf", invoker.req.AdminID)
	}

	if invoker.req.Limit != DefaultLimit {
		t.Errorf("limit = %d, want %d", invoker.req.Limit, DefaultLimit)
	}

	if invoker.req.GetRevoked() {
		t.Error("the revoked flag must stay clear unless it was asked for")
	}
}

func TestListInviteLinks_RevokedRidesTheFlag(t *testing.T) {
	wrap, invoker := newListInviteWrapper(&tg.MessagesExportedChatInvites{Count: 0})

	_, _, err := wrap.ListInviteLinks(t.Context(), inviteChannelPeer(), true, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !invoker.req.GetRevoked() {
		t.Error("revoked must reach the wire")
	}

	if invoker.req.Limit != 5 {
		t.Errorf("limit = %d, want 5", invoker.req.Limit)
	}
}

// The linkless constructor must not become a zero-valued row, and the total
// must come from the server rather than from the rows that survived.
func TestListInviteLinks_SkipsLinklessInvitesAndKeepsTheServerTotal(t *testing.T) {
	wrap, _ := newListInviteWrapper(&tg.MessagesExportedChatInvites{
		Count: 7,
		Invites: []tg.ExportedChatInviteClass{
			&tg.ChatInviteExported{Link: invitePrimaryLink, Permanent: true},
			&tg.ChatInvitePublicJoinRequests{},
		},
	})

	links, total, err := wrap.ListInviteLinks(t.Context(), inviteChannelPeer(), false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(links) != 1 {
		t.Fatalf("links = %d, want 1", len(links))
	}

	if links[0].Link != invitePrimaryLink {
		t.Errorf("link = %q, want %q", links[0].Link, invitePrimaryLink)
	}

	if total != 7 {
		t.Errorf("total = %d, want the server's 7", total)
	}
}

func TestListInviteLinks_RefusesAUserPeer(t *testing.T) {
	wrap, invoker := newListInviteWrapper(&tg.MessagesExportedChatInvites{})

	_, _, err := wrap.ListInviteLinks(t.Context(), InputPeer{Type: PeerUser, ID: 42}, false, 0)
	if !errors.Is(err, ErrNotAGroupPeer) {
		t.Fatalf("error = %v, want ErrNotAGroupPeer", err)
	}

	if invoker.req != nil {
		t.Error("a user peer must be refused before the round trip")
	}
}

// revokeInviteInvoker captures the edit request and answers with a canned
// reply, so a test can drive both shapes of messages.ExportedChatInvite.
type revokeInviteInvoker struct {
	req      *tg.MessagesEditExportedChatInviteRequest
	response tg.MessagesExportedChatInviteClass
}

func (r *revokeInviteInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesEditExportedChatInviteRequest)
	if !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	r.req = req

	return encodeResp(r.response, output)
}

func newRevokeInviteWrapper(response tg.MessagesExportedChatInviteClass) (*Wrapper, *revokeInviteInvoker) {
	invoker := &revokeInviteInvoker{response: response}

	return NewWrapper(tg.NewClient(invoker)), invoker
}

// Revoking the chat's primary link makes Telegram mint a new one on the spot.
// Discarding that reply leaves the caller believing the chat has no link.
func TestRevokeInviteLink_ReportsTheReplacement(t *testing.T) {
	const replacement = "https://t.me/+replacement"

	wrap, invoker := newRevokeInviteWrapper(&tg.MessagesExportedChatInviteReplaced{
		Invite:    &tg.ChatInviteExported{Link: invitePrimaryLink, Revoked: true},
		NewInvite: &tg.ChatInviteExported{Link: replacement, Permanent: true},
	})

	got, err := wrap.RevokeInviteLink(t.Context(), inviteChannelPeer(), invitePrimaryLink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != replacement {
		t.Errorf("replacement = %q, want %q", got, replacement)
	}

	if !invoker.req.Revoked {
		t.Error("the request must ask for a revoke")
	}

	if invoker.req.Link != invitePrimaryLink {
		t.Errorf("link = %q, want %q", invoker.req.Link, invitePrimaryLink)
	}
}

// A non-primary link is revoked without a replacement, and reporting the
// revoked link itself would read as a live one.
func TestRevokeInviteLink_PlainRevokeReportsNoReplacement(t *testing.T) {
	wrap, _ := newRevokeInviteWrapper(&tg.MessagesExportedChatInvite{
		Invite: &tg.ChatInviteExported{Link: invitePrimaryLink, Revoked: true},
	})

	got, err := wrap.RevokeInviteLink(t.Context(), inviteChannelPeer(), invitePrimaryLink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != "" {
		t.Errorf("replacement = %q, want none", got)
	}
}

func TestRevokeInviteLink_RefusesAUserPeer(t *testing.T) {
	wrap, invoker := newRevokeInviteWrapper(&tg.MessagesExportedChatInvite{
		Invite: &tg.ChatInviteExported{Link: invitePrimaryLink},
	})

	_, err := wrap.RevokeInviteLink(t.Context(), InputPeer{Type: PeerUser, ID: 42}, invitePrimaryLink)
	if !errors.Is(err, ErrNotAGroupPeer) {
		t.Fatalf("error = %v, want ErrNotAGroupPeer", err)
	}

	if invoker.req != nil {
		t.Error("a user peer must be refused before the round trip")
	}
}
