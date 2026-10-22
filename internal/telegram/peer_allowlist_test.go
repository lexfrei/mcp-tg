package telegram

import (
	"context"
	"strconv"
	"testing"

	"github.com/cockroachdb/errors"
)

// allowlistFake is the Client a PeerAllowlist wraps. Only the methods the
// allowlist forwards are implemented; anything else panics on the nil
// embedded interface, which is the point — the test would notice a method
// the allowlist was not expected to reach.
type allowlistFake struct {
	Client

	usernames map[string]InputPeer
	resolved  []string
	dialogs   []Dialog
	failOnce  map[string]bool
}

var errFakeUnknownUsername = errors.New("USERNAME_NOT_OCCUPIED")

func (fake *allowlistFake) ResolvePeer(_ context.Context, identifier string) (InputPeer, error) {
	fake.resolved = append(fake.resolved, identifier)

	if fake.failOnce[identifier] {
		delete(fake.failOnce, identifier)

		return InputPeer{}, errFakeUnknownUsername
	}

	if peer, ok := fake.usernames[identifier]; ok {
		return peer, nil
	}

	numID, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		return InputPeer{}, errFakeUnknownUsername
	}

	return resolveByID(numID), nil
}

func (fake *allowlistFake) GetDialogs(context.Context, DialogOpts) ([]Dialog, error) {
	return fake.dialogs, nil
}

func (fake *allowlistFake) GetCommonChats(context.Context, InputPeer) ([]PeerInfo, error) {
	return []PeerInfo{
		{Peer: InputPeer{Type: PeerChannel, ID: 42}},
		{Peer: InputPeer{Type: PeerChat, ID: 7}},
	}, nil
}

func newAllowlistFake() *allowlistFake {
	return &allowlistFake{
		usernames: map[string]InputPeer{
			"@friend": {Type: PeerUser, ID: 100, AccessHash: 1},
			"@team":   {Type: PeerChannel, ID: 42, AccessHash: 2},
			"@other":  {Type: PeerUser, ID: 999, AccessHash: 3},
		},
		failOnce: map[string]bool{},
	}
}

func TestPeerAllowlist_AdmitsListedPeers(t *testing.T) {
	list := NewPeerAllowlist(newAllowlistFake(), []string{"@friend", "-1000000000042"})

	for _, identifier := range []string{"@friend", "@team", "100", "-1000000000042"} {
		peer, err := list.ResolvePeer(context.Background(), identifier)
		if err != nil {
			t.Errorf("ResolvePeer(%q) = %v, want admitted", identifier, err)

			continue
		}

		if peer.ID == 0 {
			t.Errorf("ResolvePeer(%q) returned an empty peer", identifier)
		}
	}
}

func TestPeerAllowlist_RefusesOtherPeers(t *testing.T) {
	list := NewPeerAllowlist(newAllowlistFake(), []string{"@friend"})

	for _, identifier := range []string{"@other", "999", "-1000000000042", "-7"} {
		_, err := list.ResolvePeer(context.Background(), identifier)
		if !errors.Is(err, ErrPeerNotAllowed) {
			t.Errorf("ResolvePeer(%q) = %v, want ErrPeerNotAllowed", identifier, err)
		}
	}
}

// A refused numeric ID must not reach the wrapped client: resolving a user
// ID there can cost a full dialog scan, which a refusal has no use for.
func TestPeerAllowlist_RefusesNumericIDWithoutResolving(t *testing.T) {
	fake := newAllowlistFake()
	list := NewPeerAllowlist(fake, []string{"100"})

	_, err := list.ResolvePeer(context.Background(), "999")
	if !errors.Is(err, ErrPeerNotAllowed) {
		t.Fatalf("err = %v, want ErrPeerNotAllowed", err)
	}

	if len(fake.resolved) != 0 {
		t.Errorf("wrapped client resolved %q, want no calls", fake.resolved)
	}
}

// An entry that cannot be resolved fails the check instead of being
// dropped, and is retried on the next call.
func TestPeerAllowlist_UnresolvableEntryFailsClosedThenRetries(t *testing.T) {
	fake := newAllowlistFake()
	fake.failOnce["@friend"] = true
	list := NewPeerAllowlist(fake, []string{"@friend"})

	_, err := list.ResolvePeer(context.Background(), "100")
	if err == nil || errors.Is(err, ErrPeerNotAllowed) {
		t.Fatalf("first call err = %v, want the entry's resolution error", err)
	}

	_, err = list.ResolvePeer(context.Background(), "100")
	if err != nil {
		t.Fatalf("second call err = %v, want admitted after the entry resolved", err)
	}
}

func TestPeerAllowlist_FiltersListings(t *testing.T) {
	fake := newAllowlistFake()
	fake.dialogs = []Dialog{
		{Peer: InputPeer{Type: PeerUser, ID: 100}, Title: "friend"},
		{Peer: InputPeer{Type: PeerUser, ID: 999}, Title: "other"},
		{Peer: InputPeer{Type: PeerChannel, ID: 42}, Title: "team"},
	}
	list := NewPeerAllowlist(fake, []string{"@friend", "@team"})

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 2 || dialogs[0].Title != "friend" || dialogs[1].Title != "team" {
		t.Errorf("GetDialogs = %+v, want friend and team", dialogs)
	}

	chats, err := list.GetCommonChats(context.Background(), InputPeer{Type: PeerUser, ID: 100})
	if err != nil {
		t.Fatalf("GetCommonChats: %v", err)
	}

	if len(chats) != 1 || chats[0].Peer.ID != 42 {
		t.Errorf("GetCommonChats = %+v, want only the team channel", chats)
	}
}

func TestPeerAllowlist_RefusesAccountWideOperations(t *testing.T) {
	list := NewPeerAllowlist(newAllowlistFake(), []string{"@friend"})
	ctx := context.Background()

	_, searchErr := list.SearchGlobal(ctx, "x", &SearchGlobalOpts{})
	_, foldersErr := list.GetFolders(ctx)
	_, createErr := list.CreateFolder(ctx, "x", nil)

	for name, err := range map[string]error{
		"SearchGlobal":   searchErr,
		"ClearAllDrafts": list.ClearAllDrafts(ctx),
		"GetFolders":     foldersErr,
		"CreateFolder":   createErr,
		"EditFolder":     list.EditFolder(ctx, 1, "x", nil),
		"DeleteFolder":   list.DeleteFolder(ctx, 1),
	} {
		if !errors.Is(err, ErrAccountWideDisabled) {
			t.Errorf("%s = %v, want ErrAccountWideDisabled", name, err)
		}
	}
}
