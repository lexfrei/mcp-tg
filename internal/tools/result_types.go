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
type MessageItem struct {
	ID             int                   `json:"id"`
	Date           int                   `json:"date"`
	Text           string                `json:"text"`
	FromID         int64                 `json:"fromId"`
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
// Type ("user" / "chat" / "channel") disambiguates the ID space:
// without it a user with ID N and a channel with ID N would collide
// in the seen-set and silently merge into one entry. Type also lets
// the caller pick the right Telegram link form ("@username" /
// "user:N" / "channel:N") when re-rendering.
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
		return peerUser
	}
}

func participantsFromMessages(msgs []telegram.Message) []ParticipantItem {
	seen := make(map[participantKey]bool)

	var parts []ParticipantItem

	add := func(peerID int64, peerType telegram.PeerType, name, username string) {
		if peerID == 0 {
			return
		}

		label := participantTypeLabel(peerType)
		key := participantKey{Type: label, ID: peerID}

		if seen[key] {
			return
		}

		seen[key] = true

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
	return MessageItem{
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
