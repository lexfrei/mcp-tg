package tools

import "github.com/lexfrei/mcp-tg/internal/telegram"

// DialogItem is a structured dialog entry for JSON results.
type DialogItem struct {
	Peer        string `json:"peer"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	UnreadCount int    `json:"unreadCount,omitempty"`
}

func dialogToItem(dlg *telegram.Dialog) DialogItem {
	return DialogItem{
		Peer:        formatPeer(dlg.Peer),
		Title:       dlg.Title,
		Type:        dialogPeerType(dlg),
		UnreadCount: dlg.UnreadCount,
	}
}

func dialogPeerType(dlg *telegram.Dialog) string {
	if dlg.IsGroup {
		return peerGroup
	}

	switch dlg.Peer.Type {
	case telegram.PeerUser:
		return peerUser
	case telegram.PeerChat:
		return peerGroup
	case telegram.PeerChannel:
		return peerChannel
	default:
		return unknownValue
	}
}

// MessageItem is a structured message entry for JSON results.
//
// FromType disambiguates the FromID peer kind ("user" / "group" /
// "channel"; "group" is the label for both legacy basic chats and
// supergroups when they surface as a sender peer) so a caller can
// pick the right deep-link form without guessing — channel-on-behalf-
// of posts and anonymous channel posts would otherwise look
// indistinguishable from regular user senders in the JSON.
type MessageItem struct {
	ID             int                   `json:"id"`
	Date           int                   `json:"date"`
	Text           string                `json:"text"`
	FromID         int64                 `json:"fromId"`
	FromType       string                `json:"fromType,omitempty"`
	FromName       string                `json:"fromName,omitempty"`
	FromUsername   string                `json:"fromUsername,omitempty"`
	TopicID        int                   `json:"topicId,omitempty"`
	MediaType      string                `json:"mediaType,omitempty"`
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
// Type ("user" / "group" / "channel") disambiguates the ID space:
// without it a user with ID N and a channel with ID N would collide
// in the seen-set and silently merge into one entry. The same Type
// label appears on MessageItem.FromType so the caller can correlate
// a sender with its participant entry.
type ParticipantItem struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

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
		return unknownValue
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
		Date:         msg.Date,
		Text:         msg.Text,
		FromID:       msg.FromID,
		FromName:     msg.FromName,
		FromUsername: msg.FromUsername,
		TopicID:      msg.TopicID,
		MediaType:    msg.MediaType,
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
