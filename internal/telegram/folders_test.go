package telegram

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
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

// tg_folders_list reports pinned chats inside peers too, so a caller that
// reads peers, edits and sends it back keeps them pinned. A pinned chat left
// out of peers is removed from the folder: from both lists, not just unpinned.
func TestEditedFilter_PinnedChatLeftOutLeavesTheFolder(t *testing.T) {
	peers := []InputPeer{{Type: PeerUser, ID: 20, AccessHash: 200}}

	edited, ok := editedFilter(ruleFolder(), "Work", peers).(*tg.DialogFilter)
	if !ok {
		t.Fatal("edited filter changed constructor")
	}

	if len(edited.PinnedPeers) != 0 {
		t.Errorf("pinned = %+v, want the channel no longer pinned", edited.PinnedPeers)
	}

	for _, raw := range edited.IncludePeers {
		if channel, isChannel := raw.(*tg.InputPeerChannel); isChannel && channel.ChannelID == 10 {
			t.Errorf("include = %+v, the left-out channel moved into the include list", edited.IncludePeers)
		}
	}
}

// peers: [] empties the folder: both lists end up empty except for Saved
// Messages, which peers cannot name.
func TestEditedFilter_EmptyListEmptiesTheFolder(t *testing.T) {
	edited, ok := editedFilter(ruleFolder(), "Work", []InputPeer{}).(*tg.DialogFilter)
	if !ok {
		t.Fatal("edited filter changed constructor")
	}

	if len(edited.PinnedPeers) != 0 {
		t.Errorf("pinned = %+v, want none", edited.PinnedPeers)
	}

	if len(edited.IncludePeers) != 1 {
		t.Fatalf("include = %+v, want only Saved Messages", edited.IncludePeers)
	}

	if _, self := edited.IncludePeers[0].(*tg.InputPeerSelf); !self {
		t.Errorf("include[0] = %#v, want Saved Messages kept", edited.IncludePeers[0])
	}
}

// A chat already in the folder is sent back as the filter holds it, so a
// resolution without an access hash cannot break the edit.
func TestEditedFilter_KeptChatReusesTheFiltersPeer(t *testing.T) {
	peers := []InputPeer{{Type: PeerUser, ID: 20}}

	edited, ok := editedFilter(ruleFolder(), "Work", peers).(*tg.DialogFilter)
	if !ok {
		t.Fatal("edited filter changed constructor")
	}

	for _, raw := range edited.IncludePeers {
		if user, isUser := raw.(*tg.InputPeerUser); isUser && user.UserID == 20 {
			if user.AccessHash != 200 {
				t.Errorf("user 20 hash = %d, want the filter's 200", user.AccessHash)
			}

			return
		}
	}

	t.Errorf("include = %+v, want user 20 kept", edited.IncludePeers)
}

// folderInvoker serves a fixed folder list and records every write.
type folderInvoker struct {
	filters []tg.DialogFilterClass
	updates []*tg.MessagesUpdateDialogFilterRequest
}

func (f *folderInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.MessagesGetDialogFiltersRequest:
		return encodeAndDecode(&tg.MessagesDialogFilters{Filters: f.filters}, output)
	case *tg.MessagesUpdateDialogFilterRequest:
		f.updates = append(f.updates, req)

		return encodeAndDecode(&tg.BoolTrue{}, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

func TestEditFolder_UnknownIDWritesNothing(t *testing.T) {
	invoker := &folderInvoker{filters: []tg.DialogFilterClass{ruleFolder()}}

	err := newWrapperWithInvoker(invoker).EditFolder(t.Context(), 99, "Job", nil)
	if !errors.Is(err, ErrFolderNotFound) {
		t.Errorf("err = %v, want ErrFolderNotFound", err)
	}

	if len(invoker.updates) != 0 {
		t.Errorf("sent %d updateDialogFilter request(s) for a missing folder, want none", len(invoker.updates))
	}
}

// The filter written is the one read back, so a title-only edit carries the
// rule flags and peer lists on the wire.
func TestEditFolder_WritesTheReadBackFilter(t *testing.T) {
	invoker := &folderInvoker{filters: []tg.DialogFilterClass{&tg.DialogFilterDefault{}, ruleFolder()}}

	err := newWrapperWithInvoker(invoker).EditFolder(t.Context(), 7, "Job", nil)
	if err != nil {
		t.Fatalf("EditFolder: %v", err)
	}

	if len(invoker.updates) != 1 {
		t.Fatalf("sent %d update(s), want 1", len(invoker.updates))
	}

	req := invoker.updates[0]

	filter, ok := req.Filter.(*tg.DialogFilter)
	if !ok || req.ID != 7 {
		t.Fatalf("request = id %d filter %T, want id 7 and a dialogFilter", req.ID, req.Filter)
	}

	if filter.Title.Text != "Job" || !filter.Groups || !filter.Broadcasts || !filter.ExcludeMuted ||
		!filter.ExcludeArchived || filter.Emoticon != "💼" {
		t.Errorf("filter on the wire = %+v, want the new title and every rule kept", filter)
	}

	if len(filter.PinnedPeers) != 1 || len(filter.IncludePeers) != 2 || len(filter.ExcludePeers) != 1 {
		t.Errorf("peer lists on the wire: pinned %d include %d exclude %d, want 1 2 1",
			len(filter.PinnedPeers), len(filter.IncludePeers), len(filter.ExcludePeers))
	}
}

func TestFreeFolderID_TakesTheSmallestUnusedFromTwo(t *testing.T) {
	filters := []tg.DialogFilterClass{
		&tg.DialogFilterDefault{},
		&tg.DialogFilter{ID: 2},
		&tg.DialogFilterChatlist{ID: 3},
		&tg.DialogFilter{ID: 72},
	}

	got, err := freeFolderID(filters)
	if err != nil || got != 4 {
		t.Errorf("freeFolderID = %d, %v; want 4 (2 and 3 are taken, chatlists included)", got, err)
	}
}

func TestFreeFolderID_NeverHandsOutAReservedID(t *testing.T) {
	got, err := freeFolderID(nil)
	if err != nil || got != firstFolderID {
		t.Errorf("freeFolderID on an empty account = %d, %v; want %d (0 and 1 are reserved)", got, err, firstFolderID)
	}
}

func TestFreeFolderID_FullAccount(t *testing.T) {
	filters := make([]tg.DialogFilterClass, 0, lastFolderID)
	for folderID := firstFolderID; folderID <= lastFolderID; folderID++ {
		filters = append(filters, &tg.DialogFilter{ID: folderID})
	}

	_, err := freeFolderID(filters)
	if !errors.Is(err, ErrNoFreeFolderID) {
		t.Errorf("err = %v, want ErrNoFreeFolderID", err)
	}
}

// statefulFolderInvoker keeps a folder list that updateDialogFilter writes
// to, the way the server does: an update on a taken id replaces that folder,
// and an update with no filter deletes it.
type statefulFolderInvoker struct {
	mu        sync.Mutex
	filters   map[int]tg.DialogFilterClass
	listErr   error
	updateErr error
	updates   []*tg.MessagesUpdateDialogFilterRequest

	// window, when set, holds every list and every delete that long, so
	// an unguarded overlap with another folder write has time to happen.
	window time.Duration
	// open counts the read-then-write sequences under way: a list opens
	// one, the update with a filter that follows closes it. maxOpen is the
	// most seen at once.
	open    int
	maxOpen int
	// resendDelete applies each delete a second time after the window, the
	// way a delete whose answer was lost is resent within the same call.
	// deleting is set between the two copies, and listedInDelete records a
	// list that ran while it was.
	resendDelete   bool
	deleting       bool
	listedInDelete bool
	deleteStarted  chan struct{}
}

func (s *statefulFolderInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.MessagesGetDialogFiltersRequest:
		return s.list(output)
	case *tg.MessagesUpdateDialogFilterRequest:
		if req.Filter == nil {
			return s.remove(req, output)
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		s.updates = append(s.updates, req)
		s.open--

		if s.updateErr != nil {
			return s.updateErr
		}

		s.filters[req.ID] = req.Filter

		return encodeAndDecode(&tg.BoolTrue{}, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

func (s *statefulFolderInvoker) remove(req *tg.MessagesUpdateDialogFilterRequest, output bin.Decoder) error {
	s.mu.Lock()
	s.updates = append(s.updates, req)
	delete(s.filters, req.ID)
	s.deleting = s.resendDelete
	s.mu.Unlock()

	if s.resendDelete {
		if s.deleteStarted != nil {
			close(s.deleteStarted)
		}

		time.Sleep(s.window)

		s.mu.Lock()
		delete(s.filters, req.ID)
		s.deleting = false
		s.mu.Unlock()
	}

	return encodeAndDecode(&tg.BoolTrue{}, output)
}

func (s *statefulFolderInvoker) list(output bin.Decoder) error {
	s.mu.Lock()
	if s.listErr != nil {
		s.mu.Unlock()

		return s.listErr
	}

	s.open++
	s.maxOpen = max(s.maxOpen, s.open)
	s.listedInDelete = s.listedInDelete || s.deleting

	reply := &tg.MessagesDialogFilters{Filters: []tg.DialogFilterClass{&tg.DialogFilterDefault{}}}
	for _, filter := range s.filters {
		reply.Filters = append(reply.Filters, filter)
	}
	s.mu.Unlock()

	time.Sleep(s.window)

	return encodeAndDecode(reply, output)
}

func newFolderStore(ids ...int) *statefulFolderInvoker {
	store := &statefulFolderInvoker{filters: map[int]tg.DialogFilterClass{}}
	for _, id := range ids {
		store.filters[id] = &tg.DialogFilter{ID: id, Title: tg.TextWithEntities{Text: "taken"}}
	}

	return store
}

// The fix is the wiring: the free id must go out in the request and in the
// filter alike, and come back in the result.
func TestCreateFolder_SendsAndReturnsTheFreeID(t *testing.T) {
	store := newFolderStore(2, 3)

	folder, err := newWrapperWithInvoker(store).CreateFolder(t.Context(), "Work",
		[]InputPeer{{Type: PeerUser, ID: 20, AccessHash: 200}})
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}

	if len(store.updates) != 1 {
		t.Fatalf("sent %d update(s), want 1", len(store.updates))
	}

	req := store.updates[0]

	filter, ok := req.Filter.(*tg.DialogFilter)
	if !ok || req.ID != 4 || filter.ID != 4 || filter.Title.Text != "Work" || len(filter.IncludePeers) != 1 {
		t.Errorf("request = id %d filter %+v, want id 4 in both, the title and the one chat", req.ID, req.Filter)
	}

	if folder.ID != 4 {
		t.Errorf("result id = %d, want 4", folder.ID)
	}
}

func TestCreateFolder_StopsBeforeTheUpdate(t *testing.T) {
	listFailed := newFolderStore()
	listFailed.listErr = errors.New("list failed")

	full := newFolderStore()
	for id := firstFolderID; id <= lastFolderID; id++ {
		full.filters[id] = &tg.DialogFilter{ID: id}
	}

	for name, tc := range map[string]struct {
		store *statefulFolderInvoker
		want  error
	}{
		"list error": {listFailed, listFailed.listErr},
		"no free id": {full, ErrNoFreeFolderID},
	} {
		_, err := newWrapperWithInvoker(tc.store).CreateFolder(t.Context(), "Work", nil)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}

		if len(tc.store.updates) != 0 {
			t.Errorf("%s: sent %d update(s), want none", name, len(tc.store.updates))
		}
	}
}

func TestCreateFolder_PassesTheUpdateErrorThrough(t *testing.T) {
	store := newFolderStore()
	store.updateErr = errors.New("DIALOG_FILTERS_TOO_MUCH")

	_, err := newWrapperWithInvoker(store).CreateFolder(t.Context(), "Work", nil)
	if !errors.Is(err, store.updateErr) {
		t.Errorf("err = %v, want the update error", err)
	}
}

// Two creates in one process must not pick the same id: updateDialogFilter
// on a taken id is an edit, so the second would silently replace the first.
// Each list is held open for a window long enough for an unguarded second
// create to read the same list, so the overlap is counted, not raced for.
func TestCreateFolder_ConcurrentCreatesGetDistinctIDs(t *testing.T) {
	store := newFolderStore()
	store.window = 100 * time.Millisecond

	wrap := newWrapperWithInvoker(store)

	var group sync.WaitGroup

	results := make([]*Folder, 2)
	errs := make([]error, 2)

	for idx := range 2 {
		group.Go(func() {
			results[idx], errs[idx] = wrap.CreateFolder(t.Context(), "Folder", nil)
		})
	}

	group.Wait()

	for idx, err := range errs {
		if err != nil {
			t.Fatalf("create %d: %v", idx, err)
		}
	}

	if store.maxOpen != 1 {
		t.Errorf("%d creates read the list at once, want 1", store.maxOpen)
	}

	if results[0].ID == results[1].ID || len(store.filters) != 2 {
		t.Errorf("ids %d and %d, store holds %d folder(s); want two distinct folders",
			results[0].ID, results[1].ID, len(store.filters))
	}
}

// A delete whose answer is lost is resent within the same call. A create
// that read the list between the two copies would take the freed id, report
// it, and lose its folder to the resent delete.
func TestDeleteFolder_CreateWaitsForTheResentDelete(t *testing.T) {
	store := newFolderStore(firstFolderID)
	store.window = 100 * time.Millisecond
	store.resendDelete = true
	store.deleteStarted = make(chan struct{})

	wrap := newWrapperWithInvoker(store)

	var (
		group     sync.WaitGroup
		deleteErr error
		created   *Folder
		createErr error
	)

	group.Go(func() { deleteErr = wrap.DeleteFolder(t.Context(), firstFolderID) })

	<-store.deleteStarted

	group.Go(func() { created, createErr = wrap.CreateFolder(t.Context(), "New", nil) })
	group.Wait()

	if deleteErr != nil || createErr != nil {
		t.Fatalf("delete: %v, create: %v", deleteErr, createErr)
	}

	if store.listedInDelete {
		t.Error("the create read the list while the delete was still being resent")
	}

	if _, ok := store.filters[created.ID]; !ok {
		t.Errorf("create reported folder %d, but the store has %d folder(s) and not that one", created.ID, len(store.filters))
	}
}
