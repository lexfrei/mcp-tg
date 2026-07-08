package tools

import "github.com/lexfrei/mcp-tg/internal/telegram"

// DialogItem is a structured dialog entry for JSON results.
//
// Peer is the bot-API numeric ID (signed: positive = user, negative =
// chat, -100xxx = channel), kept for backwards compatibility with the
// peer parameter every other tool accepts. Username is the @handle
// when the dialog is a public user/channel/supergroup — absent for
// private chats and basic groups. Type uses the same labels as
// ParticipantItem.Type / MessageItem.FromType so a consumer can pivot
// between the shapes uniformly.
type DialogItem struct {
	Peer        string `json:"peer"`
	Title       string `json:"title"`
	Username    string `json:"username,omitempty"`
	Type        string `json:"type"`
	UnreadCount int    `json:"unreadCount,omitempty"`
}

func dialogToItem(dlg *telegram.Dialog) DialogItem {
	return DialogItem{
		Peer:        formatPeer(dlg.Peer),
		Title:       dlg.Title,
		Username:    dlg.Username,
		Type:        dialogPeerType(dlg),
		UnreadCount: dlg.UnreadCount,
	}
}

// dialogPeerType maps a Dialog to the canonical kind label. It MUST
// agree with peerLabel for the same Peer — the README and CLAUDE.md
// promise that the JSON 'type' field and the text '[kind:N]' bracket
// label use the same string. Supergroups arrive as PeerChannel (gotd
// folds broadcast channels and supergroups into the same type), so
// they MUST label as 'channel', not 'group'. 'group' is only legacy
// basic groups (PeerChat).
//
// The Dialog.IsGroup flag is intentionally ignored here — it's a UX
// hint for grouping in lists, not a kind discriminator. Honouring it
// would create the very mismatch we promise not to: a supergroup
// rendered as 'Title [channel:N]' in text but 'type:"group"' in JSON.
func dialogPeerType(dlg *telegram.Dialog) string {
	switch dlg.Peer.Type {
	case telegram.PeerUser:
		return peerUser
	case telegram.PeerChat:
		return peerGroup
	case telegram.PeerChannel:
		return peerChannel
	default:
		return unknownPeerType
	}
}

// MessageItem is a structured message entry for JSON results.
//
// FromType disambiguates the FromID peer kind ("user" / "group" /
// "channel"). The mapping mirrors MTProto's PeerClass: PeerUser →
// "user", PeerChat → "group" (legacy basic groups only), PeerChannel
// → "channel" (which covers both broadcast channels AND supergroups
// — gotd represents supergroups as PeerChannel). Knowing the kind
// lets a caller pick the right deep-link form instead of guessing —
// channel-on-behalf-of posts and anonymous channel posts would
// otherwise look indistinguishable from regular user senders.
// PeerID identifies the host peer (chat / channel / user) that
// contains the message. Carried in every MessageItem so a consumer
// of tg_messages_search_global — where results span arbitrary
// peers — can attribute each result to its source without an extra
// resolution call. The embedded InputPeer's AccessHash is omitted
// when zero (a zero hash looks valid to MTProto but raises
// PEER_ID_INVALID on round-trip; see the InputPeer godoc).
//
// PeerID uses json:"peerId,omitzero" — when the upstream MTProto
// response carried no peer (e.g. tg.UpdateShortSentMessage from
// SendMessage, which only returns ID + Date), the field disappears
// from JSON rather than serializing a fake {type:0, id:0} that
// downstream code might misread as a real PeerUser ID 0. omitempty
// has no effect on nested structs in Go's encoding/json; omitzero
// (Go 1.24+) is what actually omits an all-zero InputPeer.
type MessageItem struct {
	ID             int                   `json:"id"`
	PeerID         telegram.InputPeer    `json:"peerId,omitzero"`
	Date           int                   `json:"date"`
	Text           string                `json:"text"`
	FromID         int64                 `json:"fromId"`
	FromType       string                `json:"fromType,omitempty"`
	FromName       string                `json:"fromName,omitempty"`
	FromUsername   string                `json:"fromUsername,omitempty"`
	TopicID        int                   `json:"topicId,omitempty"`
	Type           string                `json:"type"`
	Entities       []telegram.Entity     `json:"entities,omitempty"`
	ReplyTo        *telegram.ReplyToInfo `json:"replyTo,omitempty"`
	ReplyToMessage *ReplyToMessage       `json:"replyToMessage,omitempty"`
	Forward        *telegram.ForwardInfo `json:"forward,omitempty"`
}

// ReplyToMessage carries minimal parent-message context used to help
// callers reconstruct thread structure when the parent is outside the
// returned batch.
type ReplyToMessage struct {
	FromName     string `json:"fromName,omitempty"`
	FromUsername string `json:"fromUsername,omitempty"`
	Text         string `json:"text,omitempty"`
}

// ParticipantItem identifies every peer that appears as a sender or as
// the original author of a forwarded message in a returned batch.
//
// Type follows the same mapping as MessageItem.FromType ("user" /
// "group" / "channel"; "channel" covers both broadcast channels and
// supergroups, "group" is only legacy basic groups). The Type field
// also disambiguates the ID space — without it a user with ID N and
// a channel with ID N would collide in the seen-set and silently
// merge into one entry.
type ParticipantItem struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// PeerRefItem is the canonical {id, type, name, username} shape used
// for every peer JSON-rendered across the tool surface — sender,
// forward-author, participant, dialog, channel/group reference,
// reactor, contact. Tools that historically used ad-hoc shapes
// (DialogItem, UserItem, ReactionUserItem, etc.) are converging on
// this struct so a consumer can treat any peer uniformly.
type PeerRefItem = ParticipantItem

type participantKey struct {
	Type string
	ID   int64
}

func participantTypeLabel(peerType telegram.PeerType) string {
	switch peerType {
	case telegram.PeerUser:
		return peerUser
	case telegram.PeerChat:
		return peerGroup
	case telegram.PeerChannel:
		return peerChannel
	default:
		// Mirror peerLabel's defensive default — surface an unknown
		// peer kind explicitly so a future PeerType extension doesn't
		// silently misclassify itself as a regular user in JSON while
		// the text output (via peerLabel) shows 'unknown:N'.
		return unknownPeerType
	}
}

func participantsFromMessages(msgs []telegram.Message) []ParticipantItem {
	// Insertion order is preserved while later non-empty fields
	// upgrade earlier entries (last-non-empty-wins) — same semantics
	// as buildSenderLookup, so a renamed peer reflects its most
	// recent display name regardless of which appearance the caller
	// happens to scan first.
	index := make(map[participantKey]int)

	var parts []ParticipantItem

	add := func(peerID int64, peerType telegram.PeerType, name, username string) {
		if peerID == 0 {
			return
		}

		label := participantTypeLabel(peerType)
		key := participantKey{Type: label, ID: peerID}

		if pos, ok := index[key]; ok {
			if name != "" {
				parts[pos].Name = name
			}

			if username != "" {
				parts[pos].Username = username
			}

			return
		}

		index[key] = len(parts)

		parts = append(parts, ParticipantItem{
			ID: peerID, Type: label, Name: name, Username: username,
		})
	}

	for idx := range msgs {
		msg := &msgs[idx]
		add(msg.FromID, msg.FromType, msg.FromName, msg.FromUsername)

		if msg.Forward != nil && msg.Forward.From != nil {
			add(
				msg.Forward.From.Peer.ID, msg.Forward.From.Peer.Type,
				msg.Forward.From.Name, msg.Forward.From.Username,
			)
		}
	}

	return parts
}

// UserItem is a structured user entry for JSON results.
type UserItem struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName,omitempty"`
	Username  string `json:"username,omitempty"`
}

// FolderItem is a structured folder entry for JSON results.
type FolderItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Peers int    `json:"peerCount"`
}

// TopicItem is a structured forum topic entry for JSON results.
type TopicItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Date  int    `json:"date"`
}

// StickerSetItem is a structured sticker set entry for JSON results.
type StickerSetItem struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Count int    `json:"count"`
}

// PhotoItem is a structured photo entry for JSON results.
type PhotoItem struct {
	ID   int64 `json:"id"`
	Date int   `json:"date"`
}

func messageToItem(msg *telegram.Message) MessageItem {
	item := MessageItem{
		ID:           msg.ID,
		PeerID:       msg.PeerID,
		Date:         msg.Date,
		Text:         msg.Text,
		FromID:       msg.FromID,
		FromName:     msg.FromName,
		FromUsername: msg.FromUsername,
		TopicID:      msg.TopicID,
		Type:         messageTypeOrText(msg.Type),
		Entities:     msg.Entities,
		ReplyTo:      msg.ReplyTo,
		Forward:      msg.Forward,
	}

	// Only emit fromType when we actually have a sender to label —
	// FromID==0 means "no identifiable sender" and the type would
	// be a spurious "user" from the PeerType zero value.
	if msg.FromID != 0 {
		item.FromType = participantTypeLabel(msg.FromType)
	}

	return item
}

func messagesToItems(msgs []telegram.Message) []MessageItem {
	items := make([]MessageItem, len(msgs))
	for idx := range msgs {
		items[idx] = messageToItem(&msgs[idx])
	}

	return items
}

func userToItem(usr *telegram.User) UserItem {
	return UserItem{
		ID:        usr.ID,
		FirstName: usr.FirstName,
		LastName:  usr.LastName,
		Username:  usr.Username,
	}
}

func usersToItems(users []telegram.User) []UserItem {
	items := make([]UserItem, len(users))
	for idx := range users {
		items[idx] = userToItem(&users[idx])
	}

	return items
}

func photoToItem(pht *telegram.Photo) PhotoItem {
	return PhotoItem{
		ID:   pht.ID,
		Date: pht.Date,
	}
}

func photosToItems(photos []telegram.Photo) []PhotoItem {
	items := make([]PhotoItem, len(photos))
	for idx := range photos {
		items[idx] = photoToItem(&photos[idx])
	}

	return items
}

func folderToItem(fld *telegram.Folder) FolderItem {
	return FolderItem{
		ID:    fld.ID,
		Title: fld.Title,
		Peers: len(fld.Peers),
	}
}

func foldersToItems(folders []telegram.Folder) []FolderItem {
	items := make([]FolderItem, len(folders))
	for idx := range folders {
		items[idx] = folderToItem(&folders[idx])
	}

	return items
}

func topicToItem(tpc *telegram.ForumTopic) TopicItem {
	return TopicItem{
		ID:    tpc.ID,
		Title: tpc.Title,
		Date:  tpc.Date,
	}
}

func topicsToItems(topics []telegram.ForumTopic) []TopicItem {
	items := make([]TopicItem, len(topics))
	for idx := range topics {
		items[idx] = topicToItem(&topics[idx])
	}

	return items
}

func stickerSetToItem(set *telegram.StickerSet) StickerSetItem {
	return StickerSetItem{
		Name:  set.Name,
		Title: set.Title,
		Count: set.Count,
	}
}

func stickerSetsToItems(sets []telegram.StickerSet) []StickerSetItem {
	items := make([]StickerSetItem, len(sets))
	for idx := range sets {
		items[idx] = stickerSetToItem(&sets[idx])
	}

	return items
}

func dialogsToItems(dialogs []telegram.Dialog) []DialogItem {
	items := make([]DialogItem, len(dialogs))
	for idx := range dialogs {
		items[idx] = dialogToItem(&dialogs[idx])
	}

	return items
}
