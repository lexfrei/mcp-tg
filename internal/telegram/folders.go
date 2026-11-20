package telegram

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// ErrFolderNotFound marks a folder ID the account has no folder for.
var ErrFolderNotFound = errors.New("folder not found")

// kindBots names bot chats, both as a folder rule and as a participant filter.
const kindBots = "bots"

// rawFolder returns the server's own filter for folderID.
func (w *Wrapper) rawFolder(ctx context.Context, folderID int) (tg.DialogFilterClass, error) {
	result, err := w.api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting folders")
	}

	for _, raw := range result.Filters {
		switch filter := raw.(type) {
		case *tg.DialogFilter:
			if filter.ID == folderID {
				return filter, nil
			}
		case *tg.DialogFilterChatlist:
			if filter.ID == folderID {
				return filter, nil
			}
		}
	}

	return nil, errors.Wrapf(ErrFolderNotFound, "folder %d", folderID)
}

// editedFilter returns a copy of current with the new title and, when
// peers is non-nil, the new chat list. A pinned chat stays pinned while
// it remains in the list; the account's own chat (Saved Messages), which
// peers cannot name, is kept wherever it was.
func editedFilter(current tg.DialogFilterClass, title string, peers []InputPeer) tg.DialogFilterClass {
	switch filter := current.(type) {
	case *tg.DialogFilter:
		edited := *filter
		edited.Title = retitled(filter.Title, title)

		if peers != nil {
			edited.PinnedPeers, edited.IncludePeers = splitFolderPeers(filter.PinnedPeers, filter.IncludePeers, peers)
		}

		return &edited
	case *tg.DialogFilterChatlist:
		edited := *filter
		edited.Title = retitled(filter.Title, title)

		if peers != nil {
			edited.PinnedPeers, edited.IncludePeers = splitFolderPeers(filter.PinnedPeers, filter.IncludePeers, peers)
		}

		return &edited
	default:
		return current
	}
}

// retitled keeps the title's formatting (custom emoji) when the text is
// unchanged and drops it otherwise, since its offsets would no longer fit.
func retitled(current tg.TextWithEntities, title string) tg.TextWithEntities {
	if title == current.Text {
		return current
	}

	return tg.TextWithEntities{Text: title}
}

//nolint:gocritic // unnamedResult: the pair is (pinned, include), named in the doc above.
func splitFolderPeers(
	pinned, included []tg.InputPeerClass, peers []InputPeer,
) ([]tg.InputPeerClass, []tg.InputPeerClass) {
	wanted := make(map[InputPeer]bool, len(peers))
	for _, peer := range peers {
		wanted[InputPeer{Type: peer.Type, ID: peer.ID}] = true
	}

	keptPinned := make([]tg.InputPeerClass, 0, len(pinned))
	isPinned := make(map[InputPeer]bool, len(pinned))

	for _, raw := range pinned {
		key, self := folderPeerKey(raw)
		if self || wanted[key] {
			keptPinned = append(keptPinned, raw)
			isPinned[key] = !self
		}
	}

	include := make([]tg.InputPeerClass, 0, len(peers)+1)

	for _, raw := range included {
		if _, self := folderPeerKey(raw); self {
			include = append(include, raw)
		}
	}

	for _, peer := range peers {
		if !isPinned[InputPeer{Type: peer.Type, ID: peer.ID}] {
			include = append(include, InputPeerToTG(peer))
		}
	}

	return keptPinned, include
}

func folderIncludes(filter *tg.DialogFilter) []string {
	return namedFlags([]namedFlag{
		{"contacts", filter.Contacts},
		{"non_contacts", filter.NonContacts},
		{"groups", filter.Groups},
		{"broadcasts", filter.Broadcasts},
		{kindBots, filter.Bots},
	})
}

func folderExcludes(filter *tg.DialogFilter) []string {
	return namedFlags([]namedFlag{
		{"muted", filter.ExcludeMuted},
		{"read", filter.ExcludeRead},
		{"archived", filter.ExcludeArchived},
	})
}

type namedFlag struct {
	name string
	set  bool
}

func namedFlags(flags []namedFlag) []string {
	var names []string

	for _, flag := range flags {
		if flag.set {
			names = append(names, flag.name)
		}
	}

	return names
}

// inputPeersFromTG converts a folder's peer list to domain peers. The
// account itself (inputPeerSelf, Saved Messages) has no domain form and
// is left out, as is any constructor a folder cannot hold.
func inputPeersFromTG(peers []tg.InputPeerClass) []InputPeer {
	result := make([]InputPeer, 0, len(peers))

	for _, raw := range peers {
		switch peer := raw.(type) {
		case *tg.InputPeerUser:
			result = append(result, InputPeer{Type: PeerUser, ID: peer.UserID, AccessHash: peer.AccessHash})
		case *tg.InputPeerChat:
			result = append(result, InputPeer{Type: PeerChat, ID: peer.ChatID})
		case *tg.InputPeerChannel:
			result = append(result, InputPeer{Type: PeerChannel, ID: peer.ChannelID, AccessHash: peer.AccessHash})
		}
	}

	return result
}

// folderPeerKey returns the type+ID key of a folder peer, or reports the
// account's own chat, which has no key.
func folderPeerKey(raw tg.InputPeerClass) (InputPeer, bool) {
	converted := inputPeersFromTG([]tg.InputPeerClass{raw})
	if len(converted) == 0 {
		_, self := raw.(*tg.InputPeerSelf)

		return InputPeer{}, self
	}

	return InputPeer{Type: converted[0].Type, ID: converted[0].ID}, false
}
