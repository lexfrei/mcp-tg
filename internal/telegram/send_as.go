package telegram

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// ErrSendAsUnsupportedPeer is returned when a send-as operation targets
// a peer that cannot carry send-as identities at all. MTProto exposes
// them through channels.getSendAs, so only channels and supergroups
// qualify; direct messages and legacy basic groups return an opaque
// CHANNEL_INVALID / CHAT_ID_INVALID after a wasted round trip.
var ErrSendAsUnsupportedPeer = errors.New(
	"send-as identities exist only in supergroups and channels",
)

// SendAsOption is one identity the account may post under in a chat, as
// reported by channels.getSendAs. Peer carries a usable access hash even
// though the server sends the identity list as bare peers — the hash is
// recovered from the Chats/Users arrays of the same response.
type SendAsOption struct {
	Peer            InputPeer `json:"peer"`
	Name            string    `json:"name,omitempty"`
	Username        string    `json:"username,omitempty"`
	PremiumRequired bool      `json:"premiumRequired,omitempty"`
}

// GetSendAs lists the identities this account may post under in peer.
//
// The returned peers are also stored in the peer cache, which is what
// makes a later numeric-ID sendAs work for a private channel: the bare
// peers Telegram puts in the identity list carry no access hash, and a
// numeric resolve has nowhere else to find one.
func (w *Wrapper) GetSendAs(ctx context.Context, peer InputPeer) ([]SendAsOption, error) {
	if peer.Type != PeerChannel {
		return nil, ErrSendAsUnsupportedPeer
	}

	result, err := w.api.ChannelsGetSendAs(ctx, &tg.ChannelsGetSendAsRequest{
		Peer: InputPeerToTG(peer),
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting send-as identities")
	}

	options := sendAsOptionsFrom(result)

	peers := make([]InputPeer, 0, len(options))
	for _, opt := range options {
		peers = append(peers, opt.Peer)
	}

	w.cache.StoreAll(peers)

	return options, nil
}

// SetDefaultSendAs changes the identity the account posts under in peer
// by default. The default also governs reactions and poll votes, which
// have no per-request send_as field of their own.
//
// A nil identity resets the chat to the account itself. The domain
// InputPeer has no way to name "self" — it would need an access hash the
// account never sees — so that case maps onto inputPeerSelf directly.
func (w *Wrapper) SetDefaultSendAs(ctx context.Context, peer InputPeer, sendAs *InputPeer) error {
	if peer.Type != PeerChannel {
		return ErrSendAsUnsupportedPeer
	}

	var identity tg.InputPeerClass = &tg.InputPeerSelf{}
	if sendAs != nil {
		identity = InputPeerToTG(*sendAs)
	}

	_, err := w.api.MessagesSaveDefaultSendAs(ctx, &tg.MessagesSaveDefaultSendAsRequest{
		Peer:   InputPeerToTG(peer),
		SendAs: identity,
	})

	return errors.Wrap(err, "saving the default send-as identity")
}

// sendAsOptionsFrom joins the bare identity list against the Chats and
// Users arrays that accompany it, recovering both the access hash and
// the display name for each entry.
func sendAsOptionsFrom(result *tg.ChannelsSendAsPeers) []SendAsOption {
	if result == nil {
		return nil
	}

	users := buildUserRefs(result.Users)
	chats := buildChatRefs(result.Chats)
	hydrated := sendAsInputPeers(result)

	options := make([]SendAsOption, 0, len(result.Peers))

	for _, entry := range result.Peers {
		peer := extractPeerID(entry.Peer)
		if peer.ID == 0 {
			continue
		}

		if full, ok := hydrated[peerKey{typ: peer.Type, id: peer.ID}]; ok {
			peer = full
		}

		ref, _ := lookupRefByPeer(peer, users, chats)

		options = append(options, SendAsOption{
			Peer:            peer,
			Name:            ref.Name,
			Username:        ref.Username,
			PremiumRequired: entry.PremiumRequired,
		})
	}

	return options
}

// sendAsInputPeers indexes the access-hash-bearing peers that accompany
// the identity list, keyed the same way the peer cache keys its entries.
func sendAsInputPeers(result *tg.ChannelsSendAsPeers) map[peerKey]InputPeer {
	return inputPeersByKey(result.Chats, result.Users)
}

// defaultSendAsFrom reads the identity a channel posts under by default.
// ChannelFull carries it as a bare peer, so the display name and access
// hash come from the Chats and Users of the same getFullChannel reply.
func defaultSendAsFrom(
	full *tg.ChannelFull, chats []tg.ChatClass, users []tg.UserClass,
) *SendAsOption {
	raw, ok := full.GetDefaultSendAs()
	if !ok {
		return nil
	}

	peer := extractPeerID(raw)
	if peer.ID == 0 {
		return nil
	}

	if hydrated, found := inputPeersByKey(chats, users)[peerKey{typ: peer.Type, id: peer.ID}]; found {
		peer = hydrated
	}

	ref, _ := lookupRefByPeer(peer, buildUserRefs(users), buildChatRefs(chats))

	return &SendAsOption{Peer: peer, Name: ref.Name, Username: ref.Username}
}

// cachePeersOf remembers every access-hash-bearing peer an MTProto
// response mentions.
func (w *Wrapper) cachePeersOf(chats []tg.ChatClass, users []tg.UserClass) {
	indexed := inputPeersByKey(chats, users)

	peers := make([]InputPeer, 0, len(indexed))
	for _, peer := range indexed {
		peers = append(peers, peer)
	}

	w.cache.StoreAll(peers)
}

// inputPeersByKey indexes the peers of an MTProto response by the key
// the peer cache uses, keeping their access hashes.
func inputPeersByKey(chats []tg.ChatClass, users []tg.UserClass) map[peerKey]InputPeer {
	peers := make(map[peerKey]InputPeer, len(users)+len(chats))

	for _, usr := range users {
		if typed, ok := usr.(*tg.User); ok {
			peers[peerKey{typ: PeerUser, id: typed.ID}] = InputPeer{
				Type: PeerUser, ID: typed.ID, AccessHash: typed.AccessHash,
			}
		}
	}

	for _, chat := range chats {
		if typed, ok := chat.(*tg.Channel); ok {
			peers[peerKey{typ: PeerChannel, id: typed.ID}] = InputPeer{
				Type: PeerChannel, ID: typed.ID, AccessHash: typed.AccessHash,
			}
		}
	}

	return peers
}
