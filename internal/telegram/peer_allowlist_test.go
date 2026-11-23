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
	// paged, when set, makes GetDialogs serve it the way the wrapper does:
	// newest first, at most 100 per call, older than OffsetDate.
	paged        []Dialog
	dialogCalls  []DialogOpts
	contactUsers []User
	statuses     []ContactStatus
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

func (fake *allowlistFake) GetDialogs(_ context.Context, opts DialogOpts) ([]Dialog, error) {
	fake.dialogCalls = append(fake.dialogCalls, opts)

	if fake.paged == nil {
		return fake.dialogs, nil
	}

	var page []Dialog

	for _, dialog := range fake.paged {
		if len(page) == min(opts.Limit, allowlistDialogsPage) {
			break
		}

		if opts.OffsetDate == 0 || dialog.LastMessage.Date < opts.OffsetDate {
			page = append(page, dialog)
		}
	}

	return page, nil
}

func (fake *allowlistFake) SearchContacts(context.Context, string, int) ([]User, error) {
	return fake.contactUsers, nil
}

func (fake *allowlistFake) GetBlockedContacts(context.Context, int) ([]User, error) {
	return fake.contactUsers, nil
}

func (fake *allowlistFake) GetContactStatuses(context.Context) ([]ContactStatus, error) {
	return fake.statuses, nil
}

// pagedDialogs builds count user dialogs, newest first, one second apart.
func pagedDialogs(count int) []Dialog {
	dialogs := make([]Dialog, count)
	for idx := range dialogs {
		dialogs[idx] = Dialog{
			Peer:        InputPeer{Type: PeerUser, ID: int64(1000 + idx)},
			LastMessage: &Message{Date: 100000 - idx},
		}
	}

	return dialogs
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

// The contact tools return users the caller never named, so they are
// filtered like the dialog listings.
func TestPeerAllowlist_FiltersContactListings(t *testing.T) {
	fake := newAllowlistFake()
	fake.contactUsers = []User{{ID: 100, FirstName: "friend"}, {ID: 999, FirstName: "other"}}
	fake.statuses = []ContactStatus{{UserID: 100}, {UserID: 999}}
	list := NewPeerAllowlist(fake, []string{"@friend"})

	for name, call := range map[string]func() ([]User, error){
		"SearchContacts":     func() ([]User, error) { return list.SearchContacts(context.Background(), "x", 10) },
		"GetBlockedContacts": func() ([]User, error) { return list.GetBlockedContacts(context.Background(), 10) },
	} {
		users, err := call()
		if err != nil || len(users) != 1 || users[0].ID != 100 {
			t.Errorf("%s = %+v, %v; want only user 100", name, users, err)
		}
	}

	statuses, err := list.GetContactStatuses(context.Background())
	if err != nil || len(statuses) != 1 || statuses[0].UserID != 100 {
		t.Errorf("GetContactStatuses = %+v, %v; want only user 100", statuses, err)
	}
}

// An allowed chat past the first source page must still be listed: the
// decorator reads on until it has a full page of allowed dialogs or the
// source runs out.
func TestPeerAllowlist_DialogsReadPastTheFirstSourcePage(t *testing.T) {
	fake := newAllowlistFake()
	fake.paged = pagedDialogs(250)
	// Position 150, on the second source page.
	list := NewPeerAllowlist(fake, []string{"1149"})

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 1 || dialogs[0].Peer.ID != 1149 {
		t.Fatalf("dialogs = %+v, want the allowed user 1149", dialogs)
	}

	// 100 + 100 + 50: the walk ends where the source does.
	if len(fake.dialogCalls) != 3 {
		t.Errorf("source calls = %d, want 3", len(fake.dialogCalls))
	}

	if next := fake.dialogCalls[1].OffsetDate; next != 100000-99 {
		t.Errorf("second page offsetDate = %d, want the first page's last date %d", next, 100000-99)
	}
}

// Once the page holds limit allowed dialogs, nothing more is read.
func TestPeerAllowlist_DialogsStopAtTheLimit(t *testing.T) {
	fake := newAllowlistFake()
	fake.paged = pagedDialogs(250)
	list := NewPeerAllowlist(fake, []string{"1000", "1001", "1200"})

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 2})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 2 || len(fake.dialogCalls) != 1 {
		t.Errorf("dialogs = %d over %d call(s), want 2 from one call", len(dialogs), len(fake.dialogCalls))
	}
}
