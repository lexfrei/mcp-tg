package tools

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestPrivacyGet(t *testing.T) {
	mock := &mockClient{privacy: &telegram.PrivacySetting{
		Key: "last_seen", Value: telegram.PrivacyContacts, DisallowUsers: []int64{4},
	}}

	_, result, err := NewPrivacyGetHandler(mock)(context.Background(), nil, PrivacyGetParams{Key: "last_seen"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "last_seen: contacts, never allowed: [4]" {
		t.Errorf("output = %q", result.Output)
	}
}

func TestPrivacySet_ResolvesExceptions(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 9, AccessHash: 1}}

	_, _, err := NewPrivacySetHandler(mock)(context.Background(), nil, PrivacySetParams{
		Key: "phone_number", Value: telegram.PrivacyNobody, AllowUsers: []string{"@mom"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	update := mock.lastPrivacyUpdate
	if update == nil || update.Key != "phone_number" || update.Value != telegram.PrivacyNobody ||
		len(update.AllowUsers) != 1 || update.AllowUsers[0].ID != 9 {
		t.Errorf("update = %+v", update)
	}

	_, _, err = NewPrivacySetHandler(mock)(context.Background(), nil, PrivacySetParams{Value: telegram.PrivacyNobody})
	if !errors.Is(err, ErrPrivacyKeyRequired) {
		t.Errorf("missing key: err = %v", err)
	}
}

// The schema itself must name the allowed keys and values, so a client sees
// them without reading the docs.
func TestPrivacyTools_SchemaEnums(t *testing.T) {
	schema, ok := PrivacySetTool().InputSchema.(*jsonschema.Schema)
	if !ok {
		t.Fatalf("input schema is %T", PrivacySetTool().InputSchema)
	}

	if !slices.Contains(schema.Properties["key"].Enum, any("last_seen")) {
		t.Errorf("key enum = %v", schema.Properties["key"].Enum)
	}

	if len(schema.Properties["value"].Enum) != 4 {
		t.Errorf("value enum = %v", schema.Properties["value"].Enum)
	}
}

func TestProfileDeletePhoto(t *testing.T) {
	mock := &mockClient{}

	_, _, err := NewProfileDeletePhotoHandler(mock)(context.Background(), nil, ProfileDeletePhotoParams{})
	if err != nil || !mock.profilePhotoGone {
		t.Errorf("err = %v, deleted = %v", err, mock.profilePhotoGone)
	}
}

func TestChatsSearchPublic(t *testing.T) {
	mock := &mockClient{infos: []telegram.PeerInfo{
		{Peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 8}, Title: "Go News", Username: "gonews", Type: "channel"},
	}}

	_, result, err := NewChatsSearchPublicHandler(mock)(context.Background(), nil, ChatsSearchPublicParams{Query: "golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastSearchLimit != defaultPublicSearchLimit || !strings.Contains(result.Output, "Go News [@gonews] (channel)") {
		t.Errorf("limit = %d, output = %q", mock.lastSearchLimit, result.Output)
	}

	_, _, err = NewChatsSearchPublicHandler(mock)(context.Background(), nil, ChatsSearchPublicParams{Query: "  "})
	if !errors.Is(err, ErrQueryRequired) {
		t.Errorf("blank query: err = %v", err)
	}
}

func TestContactsListAndImport(t *testing.T) {
	mock := &mockClient{
		users:        []telegram.User{{ID: 5, FirstName: "Ann", Phone: "15550100"}},
		importResult: &telegram.ImportResult{Users: []telegram.User{{ID: 6, FirstName: "Bob"}}, Skipped: 1},
	}

	_, list, err := NewContactsListHandler(mock)(context.Background(), nil, ContactsListParams{})
	if err != nil || list.Count != 1 || !strings.Contains(list.Output, "+15550100") {
		t.Errorf("list: err = %v, result = %+v", err, list)
	}

	_, imported, err := NewContactsImportHandler(mock)(context.Background(), nil, ContactsImportParams{Contacts: []ImportContact{
		{Phone: "+15550101", FirstName: "Bob"}, {Phone: "+15550102", FirstName: "Nobody"},
	}})
	if err != nil || imported.Imported != 1 || imported.Skipped != 1 || len(mock.lastImported) != 2 {
		t.Errorf("import: err = %v, result = %+v", err, imported)
	}

	_, _, err = NewContactsImportHandler(mock)(context.Background(), nil, ContactsImportParams{})
	if !errors.Is(err, ErrContactsRequired) {
		t.Errorf("empty import: err = %v", err)
	}
}

func TestFoldersReorder(t *testing.T) {
	mock := &mockClient{}

	_, _, err := NewFoldersReorderHandler(mock)(context.Background(), nil, FoldersReorderParams{FolderIDs: []int{3, 1, 2}})
	if err != nil || !slices.Equal(mock.lastFolderOrder, []int{3, 1, 2}) {
		t.Errorf("err = %v, order = %v", err, mock.lastFolderOrder)
	}
}
