package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

const (
	// ChannelIDOffset is the offset used to convert channel IDs from bot-API style negative IDs.
	ChannelIDOffset int64 = 1000000000000
)

// ErrPeerNotFound is returned when a peer cannot be resolved.
var ErrPeerNotFound = errors.New("peer not found")

// Resolve resolves a string identifier to an InputPeer.
// Accepts: numeric ID, @username, bare username, t.me/username URL.
func Resolve(ctx context.Context, api *tg.Client, identifier string) (InputPeer, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return InputPeer{}, errors.New("empty peer identifier")
	}

	identifier = strings.TrimSpace(normalizePeerIdentifier(identifier))

	if hash := extractInviteHash(identifier); hash != "" {
		return resolveByInvite(ctx, api, hash)
	}

	numID, err := strconv.ParseInt(identifier, 10, 64)
	if err == nil {
		return resolveByID(numID), nil
	}

	return resolveByUsername(ctx, api, identifier)
}

func normalizePeerIdentifier(ident string) string {
	for _, prefix := range []string{
		"https://t.me/",
		"http://t.me/",
		"t.me/",
	} {
		if after, found := strings.CutPrefix(ident, prefix); found {
			return after
		}
	}

	return strings.TrimPrefix(ident, "@")
}

// resolveByID builds an InputPeer from a numeric (bot-API style) ID.
//
// WARNING: The returned peer has AccessHash=0. This works for basic chat
// operations (PeerChat) but may fail for users and channels that require
// a valid access hash (e.g. getting full info, sending messages to users
// not in your contacts). Prefer @username resolution when possible.
func resolveByID(numID int64) InputPeer {
	switch {
	case numID > 0:
		return InputPeer{Type: PeerUser, ID: numID}
	case numID > -ChannelIDOffset:
		return InputPeer{Type: PeerChat, ID: -numID}
	default:
		return InputPeer{Type: PeerChannel, ID: -numID - ChannelIDOffset}
	}
}

func resolveByUsername(ctx context.Context, api *tg.Client, username string) (InputPeer, error) {
	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return InputPeer{}, errors.Wrap(err, "resolving username")
	}

	return peerFromResolved(resolved)
}

// extractInviteHash returns the invite hash from a normalized identifier,
// or empty string if it's not an invite link. Recognizes:
//   - "+hash" (from t.me/+hash)
//   - "joinchat/hash" (from t.me/joinchat/hash)
func extractInviteHash(identifier string) string {
	if after, found := strings.CutPrefix(identifier, "+"); found && after != "" {
		return after
	}

	if after, found := strings.CutPrefix(identifier, "joinchat/"); found && after != "" {
		return after
	}

	return ""
}

func resolveByInvite(ctx context.Context, api *tg.Client, hash string) (InputPeer, error) {
	result, err := api.MessagesCheckChatInvite(ctx, hash)
	if err != nil {
		return InputPeer{}, errors.Wrap(err, "checking invite link")
	}

	return peerFromInvite(result)
}

func peerFromInvite(invite tg.ChatInviteClass) (InputPeer, error) {
	already, ok := invite.(*tg.ChatInviteAlready)
	if !ok {
		return InputPeer{}, errors.New("invite link: you must join this chat first")
	}

	return peerFromChat(already.Chat)
}

func peerFromChat(chat tg.ChatClass) (InputPeer, error) {
	switch typed := chat.(type) {
	case *tg.Chat:
		return InputPeer{Type: PeerChat, ID: typed.ID}, nil
	case *tg.Channel:
		return InputPeer{
			Type:       PeerChannel,
			ID:         typed.ID,
			AccessHash: typed.AccessHash,
		}, nil
	default:
		return InputPeer{}, ErrPeerNotFound
	}
}

func peerFromResolved(resolved *tg.ContactsResolvedPeer) (InputPeer, error) {
	switch peer := resolved.Peer.(type) {
	case *tg.PeerUser:
		for _, usr := range resolved.Users {
			if u, ok := usr.(*tg.User); ok && u.ID == peer.UserID {
				return InputPeer{
					Type:       PeerUser,
					ID:         u.ID,
					AccessHash: u.AccessHash,
				}, nil
			}
		}
	case *tg.PeerChat:
		return InputPeer{Type: PeerChat, ID: peer.ChatID}, nil
	case *tg.PeerChannel:
		for _, ch := range resolved.Chats {
			if c, ok := ch.(*tg.Channel); ok && c.ID == peer.ChannelID {
				return InputPeer{
					Type:       PeerChannel,
					ID:         c.ID,
					AccessHash: c.AccessHash,
				}, nil
			}
		}
	}

	return InputPeer{}, ErrPeerNotFound
}
