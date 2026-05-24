package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// TestDialogsGetInfoHandler_OutputUsesFormatPeerRef pins the
// dialogs_get_info text output contract: the Output field is
// formatPeerRef(title, username, peer) plus ": " + about when about
// is non-empty. Drops the old "Title (@username) [type]: about" form.
func TestDialogsGetInfoHandler_OutputUsesFormatPeerRef(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		info: &telegram.PeerInfo{
			Peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
			Title:    "Cool Channel",
			Username: "cool",
			About:    "Hello",
			Type:     "channel",
		},
	}

	handler := NewDialogsGetInfoHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		DialogsGetInfoParams{Peer: "@cool"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Cool Channel [@cool]: Hello"
	if res.Output != want {
		t.Errorf("Output = %q, want %q — must use formatPeerRef without [type] suffix", res.Output, want)
	}
}

func TestDialogsGetInfoHandler_OmitsColonWhenAboutEmpty(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 42},
		info: &telegram.PeerInfo{
			Peer:  telegram.InputPeer{Type: telegram.PeerUser, ID: 42},
			Title: "Pavel",
			Type:  "user",
		},
	}

	handler := NewDialogsGetInfoHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		DialogsGetInfoParams{Peer: "42"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "Pavel [user:42]"
	if res.Output != want {
		t.Errorf("Output = %q, want %q — empty about must not produce trailing ': '",
			res.Output, want)
	}
}

// TestContactsGetStatusesHandler_OutputUsesFormatPeerRef pins the
// contacts_get_statuses text output contract: each line is
// formatPeerRef(...) + " " + status. Drops the old "[ID] status".
func TestContactsGetStatusesHandler_OutputUsesFormatPeerRef(t *testing.T) {
	mock := &mockClient{
		statuses: []telegram.ContactStatus{
			{UserID: 42, Status: "online"},
			{UserID: 99, Name: "Bob", Username: "bob", Status: "offline"},
		},
	}

	handler := NewContactsGetStatusesHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsGetStatusesParams{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Bare ID line — Name/Username currently empty for upstream rows
	// without a Users[] resolution.
	if !strings.Contains(res.Output, "[user:42] online") {
		t.Errorf("Output missing '[user:42] online' (bare ref form), got:\n%s", res.Output)
	}

	// Populated entry uses the full identifier shape.
	if !strings.Contains(res.Output, "Bob [@bob] offline") {
		t.Errorf("Output missing 'Bob [@bob] offline' (populated ref form), got:\n%s", res.Output)
	}
}

func TestContactsGetStatusesHandler_JSONExposesNameUsername(t *testing.T) {
	mock := &mockClient{
		statuses: []telegram.ContactStatus{
			{UserID: 99, Name: "Bob", Username: "bob", Status: "offline"},
		},
	}

	handler := NewContactsGetStatusesHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsGetStatusesParams{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Statuses) != 1 {
		t.Fatalf("got %d items, want 1", len(res.Statuses))
	}

	if res.Statuses[0].Name != "Bob" || res.Statuses[0].Username != "bob" {
		t.Errorf("Status JSON = %+v, want Name=Bob Username=bob", res.Statuses[0])
	}
}

// TestReactionUserItem_JSONShape pins the renamed JSON field tags.
// Old shape had a single "userName" field; new shape uses "name" and
// "username" matching every other peer-bearing JSON entry.
func TestReactionUserItem_JSONShape(t *testing.T) {
	item := ReactionUserItem{
		UserID:   1,
		Name:     "Alice",
		Username: "alice",
		Emoji:    "👍",
	}

	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(raw)

	for _, want := range []string{`"userId":1`, `"name":"Alice"`, `"username":"alice"`, `"emoji":"👍"`} {
		if !strings.Contains(got, want) {
			t.Errorf("ReactionUserItem JSON missing %q in:\n%s", want, got)
		}
	}

	if strings.Contains(got, "userName") {
		t.Errorf("ReactionUserItem JSON still contains legacy 'userName' tag in:\n%s", got)
	}
}

// assertJSONTags is the shared helper used by every JSON-shape pin
// test. It walks the field-name → expected-tag map and complains on
// any field that is missing or carries a different json tag. Centralised
// so a future field-tag policy change (e.g. always-omitempty rule)
// updates the asserter, not every test.
func assertJSONTags(t *testing.T, typ reflect.Type, wantTags map[string]string) {
	t.Helper()

	for name, want := range wantTags {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("%s missing field %q", typ.Name(), name)

			continue
		}

		if got := field.Tag.Get("json"); got != want {
			t.Errorf("%s.%s json tag = %q, want %q", typ.Name(), name, got, want)
		}
	}
}

// assertLegacyFieldAbsent pins that an old-style field name does NOT
// reappear in a struct after a rename — symmetric protection against
// accidental reintroduction during refactoring.
func assertLegacyFieldAbsent(t *testing.T, typ reflect.Type, legacyName string) {
	t.Helper()

	if _, exists := typ.FieldByName(legacyName); exists {
		t.Errorf("%s reintroduced legacy field %q — was renamed in the unification sweep",
			typ.Name(), legacyName)
	}
}

// TestPeerRefItemFieldShape pins the JSON tags on the canonical
// peer-ref shape so a rename of ParticipantItem fields cannot
// silently change every consumer's expected schema.
func TestPeerRefItemFieldShape(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(PeerRefItem{}), map[string]string{
		"ID":       "id",
		"Type":     "type",
		"Name":     "name",
		"Username": "username,omitempty",
	})
}

// TestMessageItemFieldShape pins the JSON tags on MessageItem so the
// new FromType / FromUsername / Forward fields cannot be renamed
// silently.
func TestMessageItemFieldShape(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(MessageItem{}), map[string]string{
		"ID":             "id",
		"PeerID":         "peerId,omitzero",
		"Date":           "date",
		"Text":           "text",
		"FromID":         "fromId",
		"FromType":       "fromType,omitempty",
		"FromName":       "fromName,omitempty",
		"FromUsername":   "fromUsername,omitempty",
		"TopicID":        "topicId,omitempty",
		"MediaType":      "mediaType,omitempty",
		"Entities":       "entities,omitempty",
		"ReplyTo":        "replyTo,omitempty",
		"ReplyToMessage": "replyToMessage,omitempty",
		"Forward":        "forward,omitempty",
	})
}

// TestReplyToMessageFieldShape pins the nested ReplyToMessage shape
// so a rename of FromName/FromUsername/Text silently changing the
// resolveReplies pipeline's JSON contract gets caught.
func TestReplyToMessageFieldShape(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(ReplyToMessage{}), map[string]string{
		"FromName":     "fromName,omitempty",
		"FromUsername": "fromUsername,omitempty",
		"Text":         "text,omitempty",
	})
}

// TestUserItemFieldShape pins UserItem (used by users_get,
// contacts_search, contacts_list_blocked, groups_members_list,
// chats_admins, users_get_common_chats via different result types).
func TestUserItemFieldShape(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(UserItem{}), map[string]string{
		"ID":        "id",
		"FirstName": "firstName",
		"LastName":  "lastName,omitempty",
		"Username":  "username,omitempty",
	})
}

// TestMessageToItem_PeerIDPopulated pins that messageToItem copies
// msg.PeerID into item.PeerID — critical for tg_messages_search_global
// where results span arbitrary host peers and the caller has no other
// way to learn which chat a message belongs to.
func TestMessageToItem_PeerIDPopulated(t *testing.T) {
	host := telegram.InputPeer{Type: telegram.PeerChannel, ID: 1234567890}
	msg := &telegram.Message{ID: 7, PeerID: host, Date: 1700000000, Text: "hi"}

	item := messageToItem(msg)

	if item.PeerID != host {
		t.Errorf("item.PeerID = %+v, want %+v — host peer must surface in MessageItem JSON",
			item.PeerID, host)
	}
}

// TestMessageItem_JSONOmitsZeroPeerID is the behavioural counterpart
// to the tag-string pin in TestMessageItemFieldShape: it marshals a
// MessageItem with a zero PeerID and verifies the encoder actually
// omits the field. A misparsed or silently-ignored 'omitzero' tag
// would leak 'peerId:{type:0,id:0}' into JSON; the tag test alone
// would not catch that.
func TestMessageItem_JSONOmitsZeroPeerID(t *testing.T) {
	item := MessageItem{ID: 1, Date: 1700000000, Text: "hi"} // PeerID = zero

	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(raw)

	if strings.Contains(got, `"peerId"`) {
		t.Errorf("zero PeerID must be omitted from JSON (omitzero), but found 'peerId' in:\n%s", got)
	}
}

func TestMessageItem_JSONIncludesNonZeroPeerID(t *testing.T) {
	item := MessageItem{
		ID:     1,
		PeerID: telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		Date:   1700000000,
		Text:   "hi",
	}

	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	got := string(raw)

	if !strings.Contains(got, `"peerId"`) {
		t.Errorf("non-zero PeerID must be included in JSON, but missing 'peerId' in:\n%s", got)
	}
}

// TestDialogPeerType_SupergroupMatchesPeerLabel pins that the JSON
// 'type' field on DialogItem for a supergroup matches the text
// '[kind:N]' bracket label that peerLabel emits. Both must say
// 'channel' (gotd folds supergroups and broadcast channels into
// PeerChannel) — the previous code returned 'group' for IsGroup
// dialogs, which contradicted the formatPeerRef text rendering and
// broke the README's 'label = type' guarantee.
func TestDialogPeerType_SupergroupMatchesPeerLabel(t *testing.T) {
	dlg := &telegram.Dialog{
		Peer:    telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		Title:   "Supergroup",
		IsGroup: true, // gotd flags supergroups via IsGroup hint
	}

	item := dialogToItem(dlg)
	text := formatDialog(dlg)

	if item.Type != peerChannel {
		t.Errorf("supergroup dialog JSON type = %q, want %q — must match formatPeerRef's text label",
			item.Type, peerChannel)
	}

	wantText := "Supergroup [channel:500]"
	if text != wantText {
		t.Errorf("supergroup formatDialog = %q, want %q", text, wantText)
	}
}

// TestDialogItemFieldShape pins the JSON tags on DialogItem so the
// added Username field stays under its canonical tag.
func TestDialogItemFieldShape(t *testing.T) {
	assertJSONTags(t, reflect.TypeOf(DialogItem{}), map[string]string{
		"Peer":        "peer",
		"Title":       "title",
		"Username":    "username,omitempty",
		"Type":        "type",
		"UnreadCount": "unreadCount,omitempty",
	})
}

// TestContactStatusItemFieldShape pins ContactStatusItem's renamed
// Name/Username fields and verifies the legacy UserName field did
// not get reintroduced alongside them.
func TestContactStatusItemFieldShape(t *testing.T) {
	typ := reflect.TypeOf(ContactStatusItem{})

	assertJSONTags(t, typ, map[string]string{
		"UserID":   "userId",
		"Name":     "name,omitempty",
		"Username": "username,omitempty",
		"Status":   "status",
		"LastSeen": "lastSeen,omitempty",
	})
	assertLegacyFieldAbsent(t, typ, "UserName")
}

// TestReactionUserItemFieldShape pins the rename ReactionUserItem
// underwent — separate Name + Username, legacy UserName field gone.
func TestReactionUserItemFieldShape(t *testing.T) {
	typ := reflect.TypeOf(ReactionUserItem{})

	assertJSONTags(t, typ, map[string]string{
		"UserID":   "userId",
		"Name":     "name,omitempty",
		"Username": "username,omitempty",
		"Emoji":    "emoji",
	})
	assertLegacyFieldAbsent(t, typ, "UserName")
}

// TestPeerKindConstantValues pins the wire-string values of the
// kind labels. Tools across the surface (peerLabel, dialogPeerType,
// participantTypeLabel, chats_create, etc.) all source from these
// constants — a value drift like peerChannel = "broadcast" would
// silently change every JSON consumer's expected "type" enum.
func TestPeerKindConstantValues(t *testing.T) {
	cases := map[string]struct {
		got, want string
	}{
		"peerUser":        {peerUser, "user"},
		"peerGroup":       {peerGroup, "group"},
		"peerChannel":     {peerChannel, "channel"},
		"unknownPeerType": {unknownPeerType, "unknown"},
	}

	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q — constant value drift would change every JSON consumer",
				name, c.got, c.want)
		}
	}
}
