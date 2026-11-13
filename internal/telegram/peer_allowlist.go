package telegram

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
)

// ErrPeerNotAllowed is returned when a peer is outside TELEGRAM_ALLOWED_PEERS.
var ErrPeerNotAllowed = errors.New("peer is not in TELEGRAM_ALLOWED_PEERS")

// ErrAccountWideDisabled is returned by operations that span every chat of
// the account, which an allowlist cannot scope.
var ErrAccountWideDisabled = errors.New("this operation spans every chat and is disabled by TELEGRAM_ALLOWED_PEERS")

// PeerAllowlist is a Client that only addresses the peers it was given.
//
// Every tool, resource, prompt and subscription turns its peer argument into
// an InputPeer through ResolvePeer, so refusing there covers all of them at
// once. The listings that return peers the caller never named (dialogs,
// common chats) are filtered instead, and the few operations that can only
// act on the whole account (global search, clearing every draft, folders)
// are refused outright.
//
// Entries are resolved lazily, on the first check: the server logs in on
// the first tool call, so at construction time there may be no session to
// resolve a username with. An entry that fails to resolve fails the check
// and is retried on the next one — a typo must not quietly shrink the list
// to the entries that happened to work.
type PeerAllowlist struct {
	Client

	entries []string

	mut      sync.Mutex
	allowed  map[peerKey]struct{}
	resolved []bool
}

// NewPeerAllowlist wraps client so that it only addresses the peers named
// by entries, each in any form ResolvePeer accepts.
func NewPeerAllowlist(client Client, entries []string) *PeerAllowlist {
	return &PeerAllowlist{
		Client:   client,
		entries:  entries,
		allowed:  make(map[peerKey]struct{}, len(entries)),
		resolved: make([]bool, len(entries)),
	}
}

// ResolvePeer resolves identifier and refuses it unless it is on the list.
// A numeric identifier is checked before any network call.
func (list *PeerAllowlist) ResolvePeer(ctx context.Context, identifier string) (InputPeer, error) {
	numID, parseErr := strconv.ParseInt(strings.TrimSpace(identifier), 10, 64)
	if parseErr == nil {
		allowed, err := list.Allows(ctx, resolveByID(numID))
		if err != nil {
			return InputPeer{}, err
		}

		if !allowed {
			return InputPeer{}, errors.Wrapf(ErrPeerNotAllowed, "peer %q", identifier)
		}
	}

	peer, err := list.Client.ResolvePeer(ctx, identifier)
	if err != nil {
		return InputPeer{}, errors.Wrap(err, "resolving peer")
	}

	allowed, err := list.Allows(ctx, peer)
	if err != nil {
		return InputPeer{}, err
	}

	if !allowed {
		return InputPeer{}, errors.Wrapf(ErrPeerNotAllowed, "peer %q", identifier)
	}

	return peer, nil
}

// Allows reports whether peer is on the list, resolving any entries not yet
// resolved.
func (list *PeerAllowlist) Allows(ctx context.Context, peer InputPeer) (bool, error) {
	list.mut.Lock()
	defer list.mut.Unlock()

	err := list.resolveEntriesLocked(ctx)
	if err != nil {
		return false, err
	}

	_, ok := list.allowed[peerKey{typ: peer.Type, id: peer.ID}]

	return ok, nil
}

// GetDialogs returns only the dialogs on the list.
func (list *PeerAllowlist) GetDialogs(ctx context.Context, opts DialogOpts) ([]Dialog, error) {
	dialogs, err := list.Client.GetDialogs(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "listing dialogs")
	}

	return filterAllowed(ctx, list, dialogs, func(dialog *Dialog) InputPeer { return dialog.Peer })
}

// SearchDialogs returns only the matching dialogs on the list.
func (list *PeerAllowlist) SearchDialogs(ctx context.Context, query string) ([]Dialog, error) {
	dialogs, err := list.Client.SearchDialogs(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "searching dialogs")
	}

	return filterAllowed(ctx, list, dialogs, func(dialog *Dialog) InputPeer { return dialog.Peer })
}

// GetCommonChats returns only the common chats on the list.
func (list *PeerAllowlist) GetCommonChats(ctx context.Context, peer InputPeer) ([]PeerInfo, error) {
	chats, err := list.Client.GetCommonChats(ctx, peer)
	if err != nil {
		return nil, errors.Wrap(err, "listing common chats")
	}

	return filterAllowed(ctx, list, chats, func(info *PeerInfo) InputPeer { return info.Peer })
}

// SearchGlobal is refused: its results and cursor span every chat.
func (*PeerAllowlist) SearchGlobal(context.Context, string, *SearchGlobalOpts) (SearchGlobalPage, error) {
	return SearchGlobalPage{}, ErrAccountWideDisabled
}

// ClearAllDrafts is refused: it touches every chat.
func (*PeerAllowlist) ClearAllDrafts(context.Context) error {
	return ErrAccountWideDisabled
}

// GetFolders is refused: folders list chats the caller never named.
func (*PeerAllowlist) GetFolders(context.Context) ([]Folder, error) {
	return nil, ErrAccountWideDisabled
}

// CreateFolder is refused along with the rest of the folder surface.
func (*PeerAllowlist) CreateFolder(context.Context, string, []InputPeer) (*Folder, error) {
	return nil, ErrAccountWideDisabled
}

// EditFolder is refused: it replaces a folder's whole peer list, including
// chats the caller cannot see.
func (*PeerAllowlist) EditFolder(context.Context, int, string, []InputPeer) error {
	return ErrAccountWideDisabled
}

// DeleteFolder is refused along with the rest of the folder surface.
func (*PeerAllowlist) DeleteFolder(context.Context, int) error {
	return ErrAccountWideDisabled
}

func (list *PeerAllowlist) resolveEntriesLocked(ctx context.Context) error {
	for idx, entry := range list.entries {
		if list.resolved[idx] {
			continue
		}

		peer, err := list.resolveEntry(ctx, entry)
		if err != nil {
			return errors.Wrapf(err, "resolving TELEGRAM_ALLOWED_PEERS entry %q", entry)
		}

		list.allowed[peerKey{typ: peer.Type, id: peer.ID}] = struct{}{}
		list.resolved[idx] = true
	}

	return nil
}

// resolveEntry builds a numeric entry locally — its type and ID are all the
// list compares — and asks the wrapped client for anything else.
func (list *PeerAllowlist) resolveEntry(ctx context.Context, entry string) (InputPeer, error) {
	numID, parseErr := strconv.ParseInt(entry, 10, 64)
	if parseErr == nil {
		return resolveByID(numID), nil
	}

	peer, err := list.Client.ResolvePeer(ctx, entry)
	if err != nil {
		return InputPeer{}, errors.Wrap(err, "resolving peer")
	}

	return peer, nil
}

func filterAllowed[T any](ctx context.Context, list *PeerAllowlist, items []T, peerOf func(*T) InputPeer) ([]T, error) {
	kept := items[:0:0]

	for idx := range items {
		allowed, err := list.Allows(ctx, peerOf(&items[idx]))
		if err != nil {
			return nil, err
		}

		if allowed {
			kept = append(kept, items[idx])
		}
	}

	return kept, nil
}
