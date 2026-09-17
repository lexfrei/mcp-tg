package telegram

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// ErrNoPrimaryInviteLink is returned when a chat's full record carries no
// exported invite at all. Telegram populates that field only for
// administrators holding the invite-users right — TDLib exposes the same field
// as supergroupFullInfo.invite_link with exactly that caveat — so its absence
// answers a question about rights, not about the chat.
var ErrNoPrimaryInviteLink = errors.New(
	"no primary invite link is visible for this chat; Telegram shows it only to " +
		"administrators with the invite-users right — call tg_groups_invite_link_create " +
		"for a link of your own instead",
)

// ErrInviteLinkIsJoinRequestOnly is returned for chatInvitePublicJoinRequests,
// the other constructor of ExportedChatInvite, which carries no link field at
// all. It is a separate answer from ErrNoPrimaryInviteLink on purpose: telling
// an administrator who already holds the right to go and check their rights
// sends them down a dead end.
var ErrInviteLinkIsJoinRequestOnly = errors.New(
	"this chat's primary invite is a public join-request queue, which carries no link — " +
		"call tg_groups_invite_link_create for a link of your own",
)

// exportedInviteHolder is the half of tg.ChatFullClass that carries the primary
// invite link. gotd puts GetExportedInvite on *tg.ChatFull and *tg.ChannelFull
// but not on the interface they both satisfy.
type exportedInviteHolder interface {
	GetExportedInvite() (tg.ExportedChatInviteClass, bool)
}

// GetInviteLink returns the chat's primary invite link.
//
// It READS the link out of the chat's full record. messages.exportChatInvite
// is Bot API createChatInviteLink under another name and mints an ADDITIONAL
// link on every call, so it cannot back a lookup. Neither can
// messages.getExportedChatInvites: admin_id is mandatory there, so that method
// answers only for one administrator and would miss a primary link created by
// somebody else.
func (w *Wrapper) GetInviteLink(ctx context.Context, peer InputPeer) (string, error) {
	full, err := w.fullChat(ctx, peer)
	if err != nil {
		return "", err
	}

	holder, ok := full.FullChat.(exportedInviteHolder)
	if !ok {
		return "", ErrNoPrimaryInviteLink
	}

	invite, ok := holder.GetExportedInvite()
	if !ok {
		return "", ErrNoPrimaryInviteLink
	}

	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return "", ErrInviteLinkIsJoinRequestOnly
	}

	return exported.Link, nil
}

// RevokeInviteLink revokes an invite link.
func (w *Wrapper) RevokeInviteLink(ctx context.Context, peer InputPeer, link string) error {
	_, err := w.api.MessagesEditExportedChatInvite(ctx, &tg.MessagesEditExportedChatInviteRequest{
		Peer:    InputPeerToTG(peer),
		Link:    link,
		Revoked: true,
	})

	return errors.Wrap(err, "revoking invite link")
}

// InviteLink is one exported chat invite. Date and ExpireDate are unix
// seconds, like every other timestamp in this package.
type InviteLink struct {
	Link          string `json:"link"`
	Title         string `json:"title,omitempty"`
	AdminID       int64  `json:"adminId,omitempty"`
	Date          int    `json:"date,omitempty"`
	ExpireDate    int    `json:"expireDate,omitempty"`
	UsageLimit    int    `json:"usageLimit,omitempty"`
	Usage         int    `json:"usage,omitempty"`
	Requested     int    `json:"requested,omitempty"`
	Permanent     bool   `json:"permanent,omitempty"`
	RequestNeeded bool   `json:"requestNeeded,omitempty"`
	Revoked       bool   `json:"revoked,omitempty"`
}

// InviteLinkOpts are the optional properties of a new invite link. A zero
// value leaves the matching conditional field off the wire entirely, so a
// default request is byte-identical to one carrying no options at all.
type InviteLinkOpts struct {
	Title         string
	ExpireDate    int
	UsageLimit    int
	RequestNeeded bool
}

// inviteLinkFrom converts the one ExportedChatInvite constructor that carries a
// link. chatInvitePublicJoinRequests is the other and is empty, so it converts
// to nothing rather than to a zero-valued entry.
func inviteLinkFrom(invite tg.ExportedChatInviteClass) (InviteLink, bool) {
	exported, ok := invite.(*tg.ChatInviteExported)
	if !ok {
		return InviteLink{}, false
	}

	link := InviteLink{
		Link:          exported.Link,
		AdminID:       exported.AdminID,
		Date:          exported.Date,
		Permanent:     exported.Permanent,
		RequestNeeded: exported.RequestNeeded,
		Revoked:       exported.Revoked,
	}

	link.Title, _ = exported.GetTitle()
	link.ExpireDate, _ = exported.GetExpireDate()
	link.UsageLimit, _ = exported.GetUsageLimit()
	link.Usage, _ = exported.GetUsage()
	link.Requested, _ = exported.GetRequested()

	return link, true
}

// CreateInviteLink mints an ADDITIONAL invite link for the chat.
//
// It never sets legacy_revoke_permanent. That flag is what Bot API's
// exportChatInviteLink uses to REPLACE a chat's primary link, revoking every
// earlier one along with it — including the link GetInviteLink reports.
// Adding a link is the whole contract here.
func (w *Wrapper) CreateInviteLink(
	ctx context.Context, peer InputPeer, opts InviteLinkOpts,
) (*InviteLink, error) {
	if peer.Type == PeerUser {
		return nil, ErrNotAGroupPeer
	}

	req := &tg.MessagesExportChatInviteRequest{Peer: InputPeerToTG(peer)}

	if opts.Title != "" {
		req.SetTitle(opts.Title)
	}

	if opts.ExpireDate > 0 {
		req.SetExpireDate(opts.ExpireDate)
	}

	if opts.UsageLimit > 0 {
		req.SetUsageLimit(opts.UsageLimit)
	}

	if opts.RequestNeeded {
		req.SetRequestNeeded(true)
	}

	result, err := w.api.MessagesExportChatInvite(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "creating invite link")
	}

	link, ok := inviteLinkFrom(result)
	if !ok {
		return nil, ErrInviteLinkIsJoinRequestOnly
	}

	return &link, nil
}
