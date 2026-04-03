// Package telegram provides a Telegram Client API abstraction.
package telegram

// PeerType identifies the kind of Telegram peer.
type PeerType int

const (
	// PeerUser represents a user peer.
	PeerUser PeerType = iota
	// PeerChat represents a basic group chat peer.
	PeerChat
	// PeerChannel represents a channel or supergroup peer.
	PeerChannel
)

// InputPeer identifies a Telegram chat participant or channel.
type InputPeer struct {
	Type       PeerType `json:"type"`
	ID         int64    `json:"id"`
	AccessHash int64    `json:"accessHash"`
}

// Message represents a simplified Telegram message.
type Message struct {
	ID        int       `json:"id"`
	PeerID    InputPeer `json:"peerId"`
	FromID    int64     `json:"fromId"`
	FromName  string    `json:"fromName,omitempty"`
	Date      int       `json:"date"`
	Text      string    `json:"text"`
	MediaType string    `json:"mediaType,omitempty"`
	ReplyTo   int       `json:"replyTo,omitempty"`
	Views     int       `json:"views,omitempty"`
	Forwards  int       `json:"forwards,omitempty"`
	EditDate  int       `json:"editDate,omitempty"`
}

// User represents a simplified Telegram user.
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName,omitempty"`
	Username  string `json:"username,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Bot       bool   `json:"bot,omitempty"`
	Bio       string `json:"bio,omitempty"`
	Online    bool   `json:"online,omitempty"`
}

// Dialog represents a Telegram dialog (chat in the dialog list).
type Dialog struct {
	Peer        InputPeer `json:"peer"`
	Title       string    `json:"title"`
	Username    string    `json:"username,omitempty"`
	UnreadCount int       `json:"unreadCount,omitempty"`
	LastMessage *Message  `json:"lastMessage,omitempty"`
	IsChannel   bool      `json:"isChannel,omitempty"`
	IsGroup     bool      `json:"isGroup,omitempty"`
}

// GroupInfo holds detailed information about a group or channel.
type GroupInfo struct {
	Peer         InputPeer `json:"peer"`
	Title        string    `json:"title"`
	Username     string    `json:"username,omitempty"`
	About        string    `json:"about,omitempty"`
	MembersCount int       `json:"membersCount,omitempty"`
	IsChannel    bool      `json:"isChannel,omitempty"`
	IsSupergroup bool      `json:"isSupergroup,omitempty"`
	IsForum      bool      `json:"isForum,omitempty"`
}

// PeerInfo holds basic metadata about any peer.
type PeerInfo struct {
	Peer     InputPeer `json:"peer"`
	Title    string    `json:"title"`
	Username string    `json:"username,omitempty"`
	About    string    `json:"about,omitempty"`
	Type     string    `json:"type"`
}

// ForumTopic represents a topic in a forum supergroup.
type ForumTopic struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	Date  int    `json:"date"`
}

// StickerSet holds metadata about a sticker set.
type StickerSet struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// StickerSetFull holds a sticker set with its stickers.
type StickerSetFull struct {
	StickerSet

	Stickers []Sticker `json:"stickers"`
}

// Sticker represents a single sticker.
type Sticker struct {
	ID         int64  `json:"id"`
	Emoji      string `json:"emoji,omitempty"`
	FileID     int64  `json:"fileId"`
	AccessHash int64  `json:"accessHash"`
}

// Photo represents a user or chat photo.
type Photo struct {
	ID   int64 `json:"id"`
	Date int   `json:"date"`
}

// Folder represents a Telegram chat folder (filter).
type Folder struct {
	ID    int         `json:"id"`
	Title string      `json:"title"`
	Peers []InputPeer `json:"peers,omitempty"`
}

// ChatPermissions represents default chat permissions.
type ChatPermissions struct {
	SendMessages bool `json:"sendMessages"`
	SendMedia    bool `json:"sendMedia"`
	SendStickers bool `json:"sendStickers"`
	SendGifs     bool `json:"sendGifs"`
	SendPolls    bool `json:"sendPolls"`
	AddMembers   bool `json:"addMembers"`
	PinMessages  bool `json:"pinMessages"`
	ChangeInfo   bool `json:"changeInfo"`
}

// UploadedFile holds information about an uploaded file.
type UploadedFile struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Participant represents a user seen in a message result set.
type Participant struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username,omitempty"`
}

// HistoryOpts configures message history retrieval.
type HistoryOpts struct {
	Limit    int
	OffsetID int
}

// SearchOpts configures message search.
type SearchOpts struct {
	Limit    int
	OffsetID int
}

// ParseModeMarkdown is the parse mode value for Markdown formatting.
const ParseModeMarkdown = "markdown"

// SendOpts configures message sending.
type SendOpts struct {
	ReplyTo   int
	TopicID   int
	ParseMode string
}

// DialogOpts configures dialog listing.
type DialogOpts struct {
	Limit      int
	OffsetDate int
}

// TopicOpts configures forum topic listing.
type TopicOpts struct {
	Limit    int
	OffsetID int
	Query    string
}
