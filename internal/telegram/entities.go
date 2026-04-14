package telegram

import "github.com/gotd/td/tg"

// Entity type labels. These match Bot API naming so LLMs trained on
// Telegram documentation can reason about them directly.
const (
	EntityTypeBold        = "bold"
	EntityTypeItalic      = "italic"
	EntityTypeUnderline   = "underline"
	EntityTypeStrike      = "strike"
	EntityTypeSpoiler     = "spoiler"
	EntityTypeCode        = "code"
	EntityTypePre         = "pre"
	EntityTypeBlockquote  = "blockquote"
	EntityTypeTextURL     = "text_url"
	EntityTypeURL         = "url"
	EntityTypeMention     = "mention"
	EntityTypeMentionName = "mention_name"
	EntityTypeHashtag     = "hashtag"
	EntityTypeCashtag     = "cashtag"
	EntityTypeBotCommand  = "bot_command"
	EntityTypeEmail       = "email"
	EntityTypePhone       = "phone"
	EntityTypeCustomEmoji = "custom_emoji"
)

// ConvertEntities maps MTProto entities into the domain representation
// used by tool results. Unknown entity types are dropped silently so
// callers never see partial data with an empty Type.
func ConvertEntities(raw []tg.MessageEntityClass) []Entity {
	if len(raw) == 0 {
		return nil
	}

	result := make([]Entity, 0, len(raw))

	for _, ent := range raw {
		if converted, ok := entityFromTG(ent); ok {
			result = append(result, converted)
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func entityFromTG(ent tg.MessageEntityClass) (Entity, bool) {
	if entity, ok := simpleEntityFromTG(ent); ok {
		return entity, true
	}

	return extraEntityFromTG(ent)
}

// simpleEntityRange exposes the common Offset+Length fields that every
// MTProto message entity carries. gotd generates these per-type so we
// can't use a shared interface provided by the library; declare one.
type simpleEntityRange interface {
	tg.MessageEntityClass
	GetOffset() int
	GetLength() int
}

// simpleEntityType maps an MTProto entity constructor ID to the domain
// type label. Only types that carry nothing but Offset+Length belong
// here — anything with URL/language/userID metadata goes through
// extraEntityFromTG.
func simpleEntityType(ent tg.MessageEntityClass) (string, bool) {
	switch ent.(type) {
	case *tg.MessageEntityBold:
		return EntityTypeBold, true
	case *tg.MessageEntityItalic:
		return EntityTypeItalic, true
	case *tg.MessageEntityUnderline:
		return EntityTypeUnderline, true
	case *tg.MessageEntityStrike:
		return EntityTypeStrike, true
	case *tg.MessageEntitySpoiler:
		return EntityTypeSpoiler, true
	case *tg.MessageEntityCode:
		return EntityTypeCode, true
	case *tg.MessageEntityBlockquote:
		return EntityTypeBlockquote, true
	}

	return simpleEntityTypeExtras(ent)
}

func simpleEntityTypeExtras(ent tg.MessageEntityClass) (string, bool) {
	switch ent.(type) {
	case *tg.MessageEntityURL:
		return EntityTypeURL, true
	case *tg.MessageEntityMention:
		return EntityTypeMention, true
	case *tg.MessageEntityHashtag:
		return EntityTypeHashtag, true
	case *tg.MessageEntityCashtag:
		return EntityTypeCashtag, true
	case *tg.MessageEntityBotCommand:
		return EntityTypeBotCommand, true
	case *tg.MessageEntityEmail:
		return EntityTypeEmail, true
	case *tg.MessageEntityPhone:
		return EntityTypePhone, true
	}

	return "", false
}

// simpleEntityFromTG covers entity types that carry only offset+length.
func simpleEntityFromTG(ent tg.MessageEntityClass) (Entity, bool) {
	kind, ok := simpleEntityType(ent)
	if !ok {
		return Entity{}, false
	}

	ranged, ok := ent.(simpleEntityRange)
	if !ok {
		return Entity{}, false
	}

	return Entity{Type: kind, Offset: ranged.GetOffset(), Length: ranged.GetLength()}, true
}

// extraEntityFromTG covers entity types that carry URL / language /
// userId metadata beyond offset+length.
func extraEntityFromTG(ent tg.MessageEntityClass) (Entity, bool) {
	switch typed := ent.(type) {
	case *tg.MessageEntityPre:
		return Entity{
			Type: EntityTypePre, Offset: typed.Offset, Length: typed.Length,
			Language: typed.Language,
		}, true
	case *tg.MessageEntityTextURL:
		return Entity{
			Type: EntityTypeTextURL, Offset: typed.Offset, Length: typed.Length,
			URL: typed.URL,
		}, true
	case *tg.MessageEntityMentionName:
		return Entity{
			Type: EntityTypeMentionName, Offset: typed.Offset, Length: typed.Length,
			UserID: typed.UserID,
		}, true
	case *tg.MessageEntityCustomEmoji:
		return Entity{
			Type: EntityTypeCustomEmoji, Offset: typed.Offset, Length: typed.Length,
			CustomEmojiID: typed.DocumentID,
		}, true
	default:
		return Entity{}, false
	}
}
