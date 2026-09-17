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
