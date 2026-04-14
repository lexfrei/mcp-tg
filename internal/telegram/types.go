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
	ID        int          `json:"id"`
	PeerID    InputPeer    `json:"peerId"`
	FromID    int64        `json:"fromId"`
	FromName  string       `json:"fromName,omitempty"`
	TopicID   int          `json:"topicId,omitempty"`
	Date      int          `json:"date"`
	Text      string       `json:"text"`
	MediaType string       `json:"mediaType,omitempty"`
	ReplyTo   *ReplyToInfo `json:"replyTo,omitempty"`
	Entities  []Entity     `json:"entities,omitempty"`
	Views     int          `json:"views,omitempty"`
	Forwards  int          `json:"forwards,omitempty"`
	EditDate  int          `json:"editDate,omitempty"`
}

// ReplyToInfo captures Telegram MessageReplyHeader fields relevant for
// reconstructing thread structure from a message history response.
type ReplyToInfo struct {
	MessageID  int        `json:"messageId"`
	TopID      int        `json:"topId,omitempty"`
	QuoteText  string     `json:"quoteText,omitempty"`
	FromPeerID *InputPeer `json:"fromPeerId,omitempty"`
}

// Entity describes a span of formatted text within a message. Offset
// and Length are counted in UTF-16 code units — the Telegram-native
// convention — so callers using a UTF-8 runtime must translate.
type Entity struct {
	Type     string `json:"type"`
	Offset   int    `json:"offset"`
	Length   int    `json:"length"`
	URL      string `json:"url,omitempty"`
	Language string `json:"language,omitempty"`
	UserID   int64  `json:"userId,omitempty"`
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

// ReactionUser represents a user who reacted to a message.
type ReactionUser struct {
	UserID   int64  `json:"userId"`
	UserName string `json:"userName,omitempty"`
	Emoji    string `json:"emoji"`
}

// ContactStatus represents the online status of a contact.
type ContactStatus struct {
	UserID   int64  `json:"userId"`
	Status   string `json:"status"`
	LastSeen int    `json:"lastSeen,omitempty"`
}

// AdminRights represents administrator rights.
type AdminRights struct {
	ChangeInfo   bool `json:"changeInfo,omitempty"`
	PostMessages bool `json:"postMessages,omitempty"`
	EditMessages bool `json:"editMessages,omitempty"`
	DeleteMsgs   bool `json:"deleteMessages,omitempty"`
	BanUsers     bool `json:"banUsers,omitempty"`
	InviteUsers  bool `json:"inviteUsers,omitempty"`
	PinMessages  bool `json:"pinMessages,omitempty"`
	ManageCall   bool `json:"manageCall,omitempty"`
	AddAdmins    bool `json:"addAdmins,omitempty"`
	ManageTopics bool `json:"manageTopics,omitempty"`
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

// ParseMode values understood by the Telegram wrapper.
//
// ParseModeMarkdown is the legacy alias kept for backward compatibility.
// New callers should prefer ParseModeCommonMark — both map onto the same
// parser, but the new name advertises its actual dialect (CommonMark
// subset: `**bold**`, `*italic*`, “ `code` “, `[text](url)`, etc.).
//
// ParseModeMarkdownV2 and ParseModeHTML are recognised for validation
// but not yet implemented; the wrapper returns a clear error instead
// of silently dropping formatting.
const (
	ParseModeMarkdown   = "markdown"
	ParseModeCommonMark = "commonmark"
	ParseModeMarkdownV2 = "markdownv2"
	ParseModeHTML       = "html"
)

// IsCommonMarkParseMode reports whether the parseMode string selects
// the CommonMark-flavoured parser used by the wrapper.
func IsCommonMarkParseMode(mode string) bool {
	return mode == ParseModeMarkdown || mode == ParseModeCommonMark
}

// SendOpts configures message sending.
type SendOpts struct {
	ReplyTo      int
	TopicID      int
	ParseMode    string
	Silent       bool
	NoWebpage    bool
	ScheduleDate int
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
