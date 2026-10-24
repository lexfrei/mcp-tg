package telegram

import (
	"slices"
	"testing"

	"github.com/gotd/td/tg"
)

func ruleFolder() *tg.DialogFilter {
	return &tg.DialogFilter{
		ID:              7,
		Title:           tg.TextWithEntities{Text: "Work"},
		Emoticon:        "💼",
		Groups:          true,
		Broadcasts:      true,
		ExcludeMuted:    true,
		ExcludeArchived: true,
		PinnedPeers:     []tg.InputPeerClass{&tg.InputPeerChannel{ChannelID: 10, AccessHash: 100}},
		IncludePeers: []tg.InputPeerClass{
			&tg.InputPeerUser{UserID: 20, AccessHash: 200},
			&tg.InputPeerSelf{},
		},
		ExcludePeers: []tg.InputPeerClass{&tg.InputPeerChat{ChatID: 30}},
	}
}

func TestExtractFolders_ReportsTheWholeFilter(t *testing.T) {
	folders := extractFolders(&tg.MessagesDialogFilters{Filters: []tg.DialogFilterClass{
		&tg.DialogFilterDefault{},
		ruleFolder(),
		&tg.DialogFilterChatlist{ID: 8, Title: tg.TextWithEntities{Text: "Shared"}},
	}})

	if len(folders) != 2 {
		t.Fatalf("got %d folders, want 2 (the default 'All chats' entry is not a folder)", len(folders))
	}

	work := folders[0]
	if len(work.Peers) != 1 || work.Peers[0] != (InputPeer{Type: PeerUser, ID: 20, AccessHash: 200}) {
		t.Errorf("Peers = %+v, want the included user (Saved Messages has no domain form)", work.Peers)
	}

	if len(work.PinnedPeers) != 1 || len(work.ExcludePeers) != 1 {
		t.Errorf("PinnedPeers = %+v, ExcludePeers = %+v, want one each", work.PinnedPeers, work.ExcludePeers)
	}

	if !slices.Equal(work.Includes, []string{"groups", "broadcasts"}) ||
		!slices.Equal(work.Excludes, []string{"muted", "archived"}) || work.Emoticon != "💼" {
		t.Errorf("rules = +%v -%v %q, want +[groups broadcasts] -[muted archived] 💼", work.Includes, work.Excludes, work.Emoticon)
	}

	if !folders[1].Shared {
		t.Error("a chatlist folder must be reported as shared")
	}
}

func TestEditedFilter_TitleOnlyKeepsEverything(t *testing.T) {
	current := ruleFolder()

	edited, ok := editedFilter(current, "Job", nil).(*tg.DialogFilter)
	if !ok {
		t.Fatal("edited filter changed constructor")
	}

	if edited.Title.Text != "Job" {
		t.Errorf("title = %q, want Job", edited.Title.Text)
	}

	if !edited.Groups || !edited.Broadcasts || !edited.ExcludeMuted || !edited.ExcludeArchived || edited.Emoticon != "💼" {
		t.Errorf("rules or emoticon lost: %+v", edited)
	}

	if len(edited.PinnedPeers) != 1 || len(edited.IncludePeers) != 2 || len(edited.ExcludePeers) != 1 {
		t.Errorf("peer lists changed on a title-only edit: pinned %d include %d exclude %d",
			len(edited.PinnedPeers), len(edited.IncludePeers), len(edited.ExcludePeers))
	}

	if current.Title.Text != "Work" {
		t.Error("editedFilter mutated the filter it was given")
	}
}

func TestEditedFilter_NewPeersReplaceTheListOnly(t *testing.T) {
	peers := []InputPeer{
		{Type: PeerChannel, ID: 10, AccessHash: 100}, // stays pinned
		{Type: PeerUser, ID: 40, AccessHash: 400},    // new
	}

	edited, ok := editedFilter(ruleFolder(), "Work", peers).(*tg.DialogFilter)
	if !ok {
		t.Fatal("edited filter changed constructor")
	}

	if len(edited.PinnedPeers) != 1 {
		t.Errorf("pinned = %+v, want the channel still pinned", edited.PinnedPeers)
	}

	// Saved Messages kept, user 20 dropped, user 40 added, the pinned
	// channel not duplicated into the include list.
	want := []tg.InputPeerClass{&tg.InputPeerSelf{}, &tg.InputPeerUser{UserID: 40, AccessHash: 400}}
	if len(edited.IncludePeers) != len(want) {
		t.Fatalf("include = %+v, want %+v", edited.IncludePeers, want)
	}

	if _, self := edited.IncludePeers[0].(*tg.InputPeerSelf); !self {
		t.Errorf("include[0] = %#v, want Saved Messages kept", edited.IncludePeers[0])
	}

	if user, ok := edited.IncludePeers[1].(*tg.InputPeerUser); !ok || user.UserID != 40 {
		t.Errorf("include[1] = %#v, want user 40", edited.IncludePeers[1])
	}

	if len(edited.ExcludePeers) != 1 || !edited.Groups {
		t.Error("excluded chats or rules lost when only the chat list changed")
	}
}
