package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

const (
	testExampleURL = "https://example.com"
	testLangGo     = "go"
)

func TestConvertEntities_Nil(t *testing.T) {
	got := ConvertEntities(nil)
	if got != nil {
		t.Errorf("ConvertEntities(nil) = %v, want nil", got)
	}
}

func TestConvertEntities_Empty(t *testing.T) {
	got := ConvertEntities([]tg.MessageEntityClass{})
	if got != nil {
		t.Errorf("ConvertEntities([]) = %v, want nil", got)
	}
}

func TestConvertEntities_Bold(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 5},
	}

	got := ConvertEntities(raw)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	if got[0].Type != EntityTypeBold {
		t.Errorf("Type = %q, want %q", got[0].Type, EntityTypeBold)
	}

	if got[0].Offset != 0 || got[0].Length != 5 {
		t.Errorf("Offset=%d Length=%d, want 0/5", got[0].Offset, got[0].Length)
	}
}

func TestConvertEntities_TextURL(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityTextURL{Offset: 10, Length: 4, URL: testExampleURL},
	}

	got := ConvertEntities(raw)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	if got[0].Type != EntityTypeTextURL {
		t.Errorf("Type = %q, want %q", got[0].Type, EntityTypeTextURL)
	}

	if got[0].URL != testExampleURL {
		t.Errorf("URL = %q, want %q", got[0].URL, testExampleURL)
	}
}

func TestConvertEntities_Pre(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityPre{Offset: 0, Length: 20, Language: "go"},
	}

	got := ConvertEntities(raw)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	if got[0].Language != testLangGo {
		t.Errorf("Language = %q, want %q", got[0].Language, testLangGo)
	}
}

func TestConvertEntities_MentionName(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityMentionName{Offset: 0, Length: 5, UserID: 42},
	}

	got := ConvertEntities(raw)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	if got[0].UserID != 42 {
		t.Errorf("UserID = %d, want 42", got[0].UserID)
	}
}

func TestConvertEntities_CustomEmoji(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityCustomEmoji{Offset: 0, Length: 2, DocumentID: 12345},
	}

	got := ConvertEntities(raw)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}

	if got[0].Type != EntityTypeCustomEmoji {
		t.Errorf("Type = %q, want %q", got[0].Type, EntityTypeCustomEmoji)
	}

	if got[0].CustomEmojiID != 12345 {
		t.Errorf("CustomEmojiID = %d, want 12345", got[0].CustomEmojiID)
	}
}

func TestConvertEntities_Mixed(t *testing.T) {
	raw := []tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 5},
		&tg.MessageEntityItalic{Offset: 6, Length: 5},
		&tg.MessageEntityCode{Offset: 12, Length: 4},
		&tg.MessageEntityURL{Offset: 17, Length: 20},
	}

	got := ConvertEntities(raw)

	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}

	types := []string{EntityTypeBold, EntityTypeItalic, EntityTypeCode, EntityTypeURL}
	for idx, want := range types {
		if got[idx].Type != want {
			t.Errorf("entity[%d].Type = %q, want %q", idx, got[idx].Type, want)
		}
	}
}

func TestConvertMessage_WithEntities(t *testing.T) {
	raw := &tg.Message{
		ID:      42,
		Date:    1700000000,
		Message: "hello world",
		Entities: []tg.MessageEntityClass{
			&tg.MessageEntityBold{Offset: 6, Length: 5},
		},
	}

	got := ConvertMessage(raw)

	if len(got.Entities) != 1 {
		t.Fatalf("Entities len = %d, want 1", len(got.Entities))
	}

	if got.Entities[0].Type != EntityTypeBold {
		t.Errorf("Type = %q, want bold", got.Entities[0].Type)
	}
}

func TestConvertMessage_NoEntities(t *testing.T) {
	raw := &tg.Message{
		ID:      42,
		Date:    1700000000,
		Message: "plain text",
	}

	got := ConvertMessage(raw)

	if got.Entities != nil {
		t.Errorf("Entities = %v, want nil for plain message", got.Entities)
	}
}

// TestIsFormattingEntity_ExcludesServerAutoDetected pins the split that
// keeps entitiesParsed honest: Telegram adds url/mention/hashtag and
// friends to any message on its own, so they are not evidence that a
// parseMode did anything.
func TestIsFormattingEntity_ExcludesServerAutoDetected(t *testing.T) {
	formatting := []string{
		EntityTypeBold, EntityTypeItalic, EntityTypeUnderline, EntityTypeStrike,
		EntityTypeSpoiler, EntityTypeCode, EntityTypePre, EntityTypeBlockquote,
		EntityTypeTextURL, EntityTypeMentionName, EntityTypeCustomEmoji,
	}

	for _, entityType := range formatting {
		if !IsFormattingEntity(entityType) {
			t.Errorf("IsFormattingEntity(%q) = false, want true", entityType)
		}
	}

	autoDetected := []string{
		EntityTypeURL, EntityTypeMention, EntityTypeHashtag, EntityTypeCashtag,
		EntityTypeBotCommand, EntityTypeEmail, EntityTypePhone,
	}

	for _, entityType := range autoDetected {
		if IsFormattingEntity(entityType) {
			t.Errorf("IsFormattingEntity(%q) = true, want false — the server adds it unasked", entityType)
		}
	}

	// The allow-list must fail safe: an unmapped or future type (Telegram
	// auto-detects bank cards today, and this package may convert them
	// tomorrow) counts as formatting only if it is explicitly listed.
	for _, entityType := range []string{"bank_card", "", "some_future_type"} {
		if IsFormattingEntity(entityType) {
			t.Errorf("IsFormattingEntity(%q) = true — unknown types must not count as parsed markdown", entityType)
		}
	}
}
