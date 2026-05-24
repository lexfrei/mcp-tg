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

// TestPeerRefItemFieldShape pins the JSON tags on the canonical
// peer-ref shape so a rename of ParticipantItem fields cannot
// silently change every consumer's expected schema.
func TestPeerRefItemFieldShape(t *testing.T) {
	typ := reflect.TypeOf(PeerRefItem{})

	wantTags := map[string]string{
		"ID":       "id",
		"Type":     "type",
		"Name":     "name",
		"Username": "username,omitempty",
	}

	for name, want := range wantTags {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("PeerRefItem missing field %q", name)

			continue
		}

		if got := field.Tag.Get("json"); got != want {
			t.Errorf("PeerRefItem.%s json tag = %q, want %q", name, got, want)
		}
	}
}
