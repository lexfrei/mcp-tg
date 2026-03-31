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
	Type       PeerType
	ID         int64
	AccessHash int64
}

// Message represents a simplified Telegram message.
type Message struct {
	ID        int
	PeerID    InputPeer
	FromID    int64
	Date      int
	Text      string
	MediaType string
	ReplyTo   int
	Views     int
	Forwards  int
	EditDate  int
}

// User represents a simplified Telegram user.
type User struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	Phone     string
	Bot       bool
	Bio       string
	Online    bool
}

// Dialog represents a Telegram dialog (chat in the dialog list).
type Dialog struct {
	Peer        InputPeer
	Title       string
	Username    string
	UnreadCount int
	LastMessage *Message
	IsChannel   bool
	IsGroup     bool
}

// GroupInfo holds detailed information about a group or channel.
type GroupInfo struct {
	Peer         InputPeer
	Title        string
	Username     string
	About        string
	MembersCount int
	IsChannel    bool
	IsSupergroup bool
	IsForum      bool
}

// PeerInfo holds basic metadata about any peer.
type PeerInfo struct {
	Peer     InputPeer
	Title    string
	Username string
	About    string
	Type     string
}

// ForumTopic represents a topic in a forum supergroup.
type ForumTopic struct {
	ID    int
	Title string
	Icon  string
	Date  int
}

// StickerSet holds metadata about a sticker set.
type StickerSet struct {
	ID    int64
	Title string
	Name  string
	Count int
}

// StickerSetFull holds a sticker set with its stickers.
type StickerSetFull struct {
	StickerSet

	Stickers []Sticker
}

// Sticker represents a single sticker.
type Sticker struct {
	ID         int64
	Emoji      string
	FileID     int64
	AccessHash int64
}

// Photo represents a user or chat photo.
type Photo struct {
	ID   int64
	Date int
}

// Folder represents a Telegram chat folder (filter).
type Folder struct {
	ID    int
	Title string
	Peers []InputPeer
}

// ChatPermissions represents default chat permissions.
type ChatPermissions struct {
	SendMessages bool
	SendMedia    bool
	SendStickers bool
	SendGifs     bool
	SendPolls    bool
	AddMembers   bool
	PinMessages  bool
	ChangeInfo   bool
}

// UploadedFile holds information about an uploaded file.
type UploadedFile struct {
	ID   int64
	Name string
	Size int64
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

// SendOpts configures message sending.
type SendOpts struct {
	ReplyTo int
	TopicID int
}

// DialogOpts configures dialog listing.
type DialogOpts struct {
	Limit  int
	Offset int
}

// TopicOpts configures forum topic listing.
type TopicOpts struct {
	Limit    int
	OffsetID int
	Query    string
}
