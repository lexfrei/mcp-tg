// Package telegram provides a Telegram Client API abstraction.
package telegram

import "context"

// PeerType identifies the kind of Telegram peer.
//
// The zero value is PeerUser by historical accident — when an InputPeer
// is constructed from an unknown/nil tg.PeerClass the returned value is
// `{Type: PeerUser, ID: 0}`. Callers must NOT treat `{PeerUser, 0}` as
// a real user; ID == 0 is the absent-sentinel for the whole shape,
// regardless of Type. Code that needs to distinguish "unknown peer
// kind" from "real user with ID 0" should gate on ID first.
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
//
// AccessHash is serialized only when non-zero so callers don't mistake
// an unknown hash (the common case when a peer is constructed from a
// forwarded message header or a numeric ID) for a valid one — passing
// AccessHash=0 back to MTProto raises PEER_ID_INVALID or similar.
type InputPeer struct {
	Type       PeerType `json:"type"`
	ID         int64    `json:"id"`
	AccessHash int64    `json:"accessHash,omitempty"`
}

// Message represents a simplified Telegram message.
//
// FromType disambiguates which peer space FromID belongs to —
// PeerUser for regular senders, PeerChannel when a channel admin
// posts under the channel's own identity (the "post as channel"
// flow in supergroups). Without it the formatter would label every
// non-user sender as user:N.
type Message struct {
	ID           int          `json:"id"`
	PeerID       InputPeer    `json:"peerId"`
	FromID       int64        `json:"fromId"`
	FromType     PeerType     `json:"fromType,omitempty"`
	FromName     string       `json:"fromName,omitempty"`
	FromUsername string       `json:"fromUsername,omitempty"`
	TopicID      int          `json:"topicId,omitempty"`
	Date         int          `json:"date"`
	Text         string       `json:"text"`
	Type         string       `json:"type"`
	ReplyTo      *ReplyToInfo `json:"replyTo,omitempty"`
	Forward      *ForwardInfo `json:"forward,omitempty"`
	Entities     []Entity     `json:"entities,omitempty"`
	Views        int          `json:"views,omitempty"`
	Forwards     int          `json:"forwards,omitempty"`
	EditDate     int          `json:"editDate,omitempty"`
}

// Transcription status values returned by Telegram audio transcription tools.
const (
	TranscriptionStatusCompleted        = "completed"
	TranscriptionStatusPending          = "pending"
	TranscriptionStatusPremiumRequired  = "premium_required"
	TranscriptionStatusNotTranscribable = "not_transcribable"
	TranscriptionStatusFailed           = "failed"
)

// Transcription represents Telegram's audio transcription state.
type Transcription struct {
	Status                string `json:"status"`
	MessageID             int    `json:"messageId"`
	Type                  string `json:"type,omitempty"`
	Pending               bool   `json:"pending,omitempty"`
	TranscriptionID       int64  `json:"transcriptionId,omitempty"`
	Text                  string `json:"text,omitempty"`
	TrialRemainsNum       int    `json:"trialRemainsNum,omitempty"`
	TrialRemainsUntilDate int    `json:"trialRemainsUntilDate,omitempty"`
}

// ReplyToInfo captures Telegram MessageReplyHeader fields relevant for
// reconstructing thread structure from a message history response.
//
// FromName and FromUsername are populated when FromPeerID identifies a
// peer present in the response's Users/Chats list. They are advisory —
// the source of truth is the peer ID.
type ReplyToInfo struct {
	MessageID    int        `json:"messageId"`
	TopID        int        `json:"topId,omitempty"`
	QuoteText    string     `json:"quoteText,omitempty"`
	FromPeerID   *InputPeer `json:"fromPeerId,omitempty"`
	FromName     string     `json:"fromName,omitempty"`
	FromUsername string     `json:"fromUsername,omitempty"`
}

// PeerRef pairs an InputPeer with its human-readable display name and
// optional @username. It is used wherever a message references another
// peer (sender, forwarded-from origin, cross-chat reply target) so the
// caller gets a single consistent identifier shape instead of bare IDs.
type PeerRef struct {
	Peer     InputPeer `json:"peer"`
	Name     string    `json:"name,omitempty"`
	Username string    `json:"username,omitempty"`
}

// ForwardInfo captures Telegram MessageFwdHeader fields. From identifies
// the original sender (user or channel) when the original author has not
// hidden it via forward-privacy settings. When From is nil but FromName
// is set, the author is privacy-hidden and only the display name leaked
// through.
type ForwardInfo struct {
	From        *PeerRef `json:"from,omitempty"`
	FromName    string   `json:"fromName,omitempty"`
	Date        int      `json:"date,omitempty"`
	ChannelPost int      `json:"channelPost,omitempty"`
	PostAuthor  string   `json:"postAuthor,omitempty"`
}

// Entity describes a span of formatted text within a message. Offset
// and Length are counted in UTF-16 code units — the Telegram-native
// convention — so callers using a UTF-8 runtime must translate.
type Entity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	Language      string `json:"language,omitempty"`
	UserID        int64  `json:"userId,omitempty"`
	CustomEmojiID int64  `json:"customEmojiId,omitempty"`
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

// GroupInfo holds detailed information about a group, supergroup or channel.
//
// DefaultSendAs is the identity this account posts under by default, and
// is nil unless one was chosen. Basic groups never have one — MTProto
// only tracks the setting on channels.
type GroupInfo struct {
	Peer          InputPeer     `json:"peer"`
	Title         string        `json:"title"`
	Username      string        `json:"username,omitempty"`
	About         string        `json:"about,omitempty"`
	MembersCount  int           `json:"membersCount,omitempty"`
	IsChannel     bool          `json:"isChannel,omitempty"`
	IsSupergroup  bool          `json:"isSupergroup,omitempty"`
	IsForum       bool          `json:"isForum,omitempty"`
	DefaultSendAs *SendAsOption `json:"defaultSendAs,omitempty"`
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

// ReactionUser is one peer's reaction to a message.
//
// The Name and Username fields mirror the shape every other peer-bearing
// JSON surface uses (sender, forward-author, participant) so a downstream
// consumer can treat a reactor as a "Display Name [@username]" identifier
// just like any other peer.
//
// The reactor is not always a user: a channel reacts whenever it is the
// chat's default send-as identity. UserID keeps its name for backwards
// compatibility, but it holds a channel ID when PeerType says so, and
// the two id spaces do not overlap.
type ReactionUser struct {
	UserID   int64    `json:"userId"`
	PeerType PeerType `json:"-"`
	Name     string   `json:"name,omitempty"`
	Username string   `json:"username,omitempty"`
	Emoji    string   `json:"emoji"`
}

// ContactStatus represents the online status of a contact.
type ContactStatus struct {
	UserID   int64  `json:"userId"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
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
//
// Limit may exceed the 100-message server page size: GetHistory and
// GetTopicMessages fetch it in chunks and merge the pages.
//
// OffsetDate, when non-zero, anchors the newest returned message at or
// before that unix timestamp (Telegram's offset_date). It is applied to
// the first page only; later pages advance by OffsetID.
//
// MinDate, when non-zero, is a client-side floor: paging stops once a
// message older than it is reached, and older messages are dropped.
// getHistory has no server-side min_date, so the floor is enforced here.
//
// SinglePage disables auto-pagination: exactly one server page is
// fetched (still trimmed to Limit and the MinDate floor). A caller that
// wants a bounded window anchored at OffsetID — e.g. the symmetric
// context window around a message — sets this so the walk does not
// extend past the window when service messages thin out a page.
type HistoryOpts struct {
	Limit      int
	OffsetID   int
	OffsetDate int
	MinDate    int
	SinglePage bool
}

// SearchOpts configures message search.
type SearchOpts struct {
	Limit    int
	OffsetID int
	TopicID  int        // forum topic (top_msg_id); 0 = whole chat
	FromID   *InputPeer // only messages from this sender; nil = anyone
	Filter   string     // search filter name (see SearchFilters); "" = none
	MinDate  int        // unix; 0 = unbounded
	MaxDate  int        // unix; 0 = unbounded
}

// SearchGlobalOpts configures a cross-chat message search.
//
// The three offset fields form one compound pagination cursor: pass the
// previous page's NextRate as OffsetRate, and the last returned
// message's ID and peer as OffsetID/OffsetPeer. All three zero/nil
// means the first page.
type SearchGlobalOpts struct {
	Limit      int
	Filter     string // search filter name (see SearchFilters); "" = none
	MinDate    int    // unix; 0 = unbounded
	MaxDate    int    // unix; 0 = unbounded
	Scope      string // SearchScopeUsers/Groups/Channels; "" = all dialogs
	OffsetRate int
	OffsetID   int
	OffsetPeer *InputPeer // nil = first page (inputPeerEmpty)
}

// SearchGlobalPage is one page of cross-chat search results.
//
// NextRate is the next page's OffsetRate. When the server omits
// next_rate on a partial result, it falls back to the last message's
// date per the documented cursor contract; it is 0 only when there is
// no further page at all (a complete result or an empty page).
type SearchGlobalPage struct {
	Messages []Message
	Total    int // server's total match count across all pages
	NextRate int
}

// ParseMode values understood by the Telegram wrapper.
//
// ParseModePlain is the explicit "no formatting" mode; the tool layer
// requires callers to choose it or ParseModeCommonMark on every call.
//
// ParseModeMarkdown is a legacy alias for the same parser as
// ParseModeCommonMark. The tool boundary no longer accepts it — the
// wrapper keeps recognising it purely as internal defense in depth.
//
// ParseModeMarkdownV2 and ParseModeHTML are names the tool layer knows
// (it rejects them with ErrParseModeNotImplemented, and the input
// schema's enum turns them away earlier still) — the wrapper itself
// implements neither: it formats only what IsCommonMarkParseMode
// accepts and treats every other value as plain.
const (
	ParseModePlain      = "plain"
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

// UploadProgress is an optional callback invoked during a file upload with the
// running byte count and the total size. It carries the upload context so
// consumers (e.g. the MCP progress forwarder) never have to store one.
type UploadProgress func(ctx context.Context, uploaded, total int64)

// SendOpts configures message sending.
//
// SendAs names the identity the message is posted under. A nil value
// leaves the choice to the server, which applies the chat's saved
// default — the account itself unless SetDefaultSendAs changed it. Only
// identities the server lists in GetSendAs are accepted; anything else
// fails with SEND_AS_PEER_INVALID.
type SendOpts struct {
	ReplyTo      int
	TopicID      int
	ParseMode    string
	Silent       bool
	NoWebpage    bool
	ScheduleDate int
	SendAs       *InputPeer
	Progress     UploadProgress
}

// UploadOpts configures a bare file upload.
type UploadOpts struct {
	Progress UploadProgress
}

// ReactionCustomPrefix marks a reaction encoded as a premium custom emoji:
// "custom:<document_id>". GetReactions emits the same prefix when reading a
// custom-emoji reaction, so a reaction can be round-tripped read → send.
const ReactionCustomPrefix = "custom:"

// ReactionOpts configures a reaction send.
//
// Each entry in Emojis is either a standard unicode emoji ("👍") or a
// custom (premium) emoji encoded as "custom:<document_id>". Multiple
// entries set several reactions at once (a premium-only capability).
// Big requests the large animated reaction. Remove clears all reactions
// on the message and ignores Emojis.
type ReactionOpts struct {
	Emojis []string
	Big    bool
	Remove bool
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
