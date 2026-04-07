package telegram

import "context"

// Client defines the Telegram operations used by MCP tools.
//
//nolint:interfacebloat // composite interface embeds domain-specific sub-interfaces; each is small.
type Client interface {
	MessageClient
	MediaClient
	DialogClient
	ContactClient
	GroupClient
	ChatClient
	UserClient
	TopicClient
	StickerClient
	DraftClient
	FolderClient
	StatusClient
	PeerResolver
}

// MessageClient handles message operations.
//
//nolint:interfacebloat // covers all message-related Telegram RPCs as a cohesive group.
type MessageClient interface {
	GetMessages(ctx context.Context, peer InputPeer, ids []int) ([]Message, error)
	GetHistory(ctx context.Context, peer InputPeer, opts HistoryOpts) ([]Message, int, error)
	GetTopicMessages(ctx context.Context, peer InputPeer, topicID int, opts HistoryOpts) ([]Message, int, error)
	SearchMessages(ctx context.Context, peer InputPeer, query string, opts SearchOpts) ([]Message, error)
	SearchGlobal(ctx context.Context, query string, limit int) ([]Message, error)
	GetScheduledMessages(ctx context.Context, peer InputPeer) ([]Message, error)
	GetReactions(ctx context.Context, peer InputPeer, msgID int, limit int) ([]ReactionUser, error)
	SendMessage(ctx context.Context, peer InputPeer, text string, opts SendOpts) (*Message, error)
	EditMessage(ctx context.Context, peer InputPeer, msgID int, text string, parseMode string) (*Message, error)
	DeleteMessages(ctx context.Context, peer InputPeer, ids []int, revoke bool) error
	DeleteHistory(ctx context.Context, peer InputPeer, revoke bool) error
	ForwardMessages(ctx context.Context, from, dest InputPeer, ids []int) ([]Message, error)
	PinMessage(ctx context.Context, peer InputPeer, msgID int, unpin bool) error
	SendReaction(ctx context.Context, peer InputPeer, msgID int, emoji string, remove bool) error
	MarkRead(ctx context.Context, peer InputPeer, maxID int) error
}

// MediaClient handles file and media operations.
type MediaClient interface {
	SendFile(ctx context.Context, peer InputPeer, path, caption string, opts SendOpts) (*Message, error)
	SendAlbum(ctx context.Context, peer InputPeer, paths []string, caption string, opts SendOpts) ([]Message, error)
	DownloadMedia(ctx context.Context, peer InputPeer, msgID int, outputDir string) (string, error)
	UploadFile(ctx context.Context, path string) (*UploadedFile, error)
}

// DialogClient handles dialog listing and search.
type DialogClient interface {
	GetDialogs(ctx context.Context, opts DialogOpts) ([]Dialog, error)
	SearchDialogs(ctx context.Context, query string) ([]Dialog, error)
	GetPeerInfo(ctx context.Context, peer InputPeer) (*PeerInfo, error)
	PinDialog(ctx context.Context, peer InputPeer, pinned bool) error
	MarkDialogUnread(ctx context.Context, peer InputPeer, unread bool) error
}

// ContactClient handles contact operations.
type ContactClient interface {
	GetContact(ctx context.Context, peer InputPeer) (*User, error)
	SearchContacts(ctx context.Context, query string, limit int) ([]User, error)
	GetBlockedContacts(ctx context.Context, limit int) ([]User, error)
	GetContactStatuses(ctx context.Context) ([]ContactStatus, error)
	AddContact(ctx context.Context, peer InputPeer, firstName, lastName, phone string) error
	DeleteContact(ctx context.Context, peer InputPeer) error
}

// GroupClient handles group-specific operations.
//
//nolint:interfacebloat // covers all group-related Telegram RPCs as a cohesive group.
type GroupClient interface {
	GetGroupInfo(ctx context.Context, peer InputPeer) (*GroupInfo, error)
	GetGroupMembers(ctx context.Context, peer InputPeer, filter string, limit int) ([]User, error)
	JoinGroup(ctx context.Context, peer InputPeer) error
	LeaveGroup(ctx context.Context, peer InputPeer) error
	RenameGroup(ctx context.Context, peer InputPeer, title string) error
	AddGroupMember(ctx context.Context, group, user InputPeer) error
	RemoveGroupMember(ctx context.Context, group, user InputPeer) error
	SetAdmin(ctx context.Context, group, user InputPeer, rights AdminRights, rank string) error
	SetSlowMode(ctx context.Context, peer InputPeer, seconds int) error
	GetInviteLink(ctx context.Context, peer InputPeer) (string, error)
	RevokeInviteLink(ctx context.Context, peer InputPeer, link string) error
}

// ChatClient handles chat management operations.
type ChatClient interface {
	CreateChat(ctx context.Context, title string, users []InputPeer, isChannel bool) (*PeerInfo, error)
	ArchiveChat(ctx context.Context, peer InputPeer, archive bool) error
	MuteChat(ctx context.Context, peer InputPeer, muteUntil int) error
	DeleteChat(ctx context.Context, peer InputPeer) error
	SetChatPhoto(ctx context.Context, peer InputPeer, path string) error
	SetChatAbout(ctx context.Context, peer InputPeer, about string) error
	GetChatAdmins(ctx context.Context, peer InputPeer) ([]User, error)
	SetChatPermissions(ctx context.Context, peer InputPeer, perms ChatPermissions) error
}

// UserClient handles user and profile operations.
type UserClient interface {
	GetSelf(ctx context.Context) (*User, error)
	GetUser(ctx context.Context, peer InputPeer) (*User, error)
	GetUserPhotos(ctx context.Context, peer InputPeer, limit int) ([]Photo, error)
	SetProfileName(ctx context.Context, firstName, lastName string) error
	SetProfileBio(ctx context.Context, bio string) error
	SetProfilePhoto(ctx context.Context, path string) error
	BlockUser(ctx context.Context, peer InputPeer, block bool) error
	GetCommonChats(ctx context.Context, peer InputPeer) ([]PeerInfo, error)
}

// TopicClient handles forum topic operations.
type TopicClient interface {
	GetForumTopics(ctx context.Context, peer InputPeer, opts TopicOpts) ([]ForumTopic, error)
	CreateForumTopic(ctx context.Context, peer InputPeer, title string) (*ForumTopic, error)
	EditForumTopic(ctx context.Context, peer InputPeer, topicID int, title string) error
}

// StickerClient handles sticker operations.
type StickerClient interface {
	SearchStickerSets(ctx context.Context, query string) ([]StickerSet, error)
	GetStickerSet(ctx context.Context, name string) (*StickerSetFull, error)
	SendSticker(ctx context.Context, peer InputPeer, stickerFileID int64) (*Message, error)
}

// DraftClient handles draft message operations.
type DraftClient interface {
	SetDraft(ctx context.Context, peer InputPeer, text string, replyTo int) error
	ClearDraft(ctx context.Context, peer InputPeer) error
	ClearAllDrafts(ctx context.Context) error
}

// FolderClient handles chat folder operations.
type FolderClient interface {
	GetFolders(ctx context.Context) ([]Folder, error)
	CreateFolder(ctx context.Context, title string, peers []InputPeer) (*Folder, error)
	EditFolder(ctx context.Context, folderID int, title string, peers []InputPeer) error
	DeleteFolder(ctx context.Context, folderID int) error
}

// StatusClient handles presence and typing operations.
type StatusClient interface {
	SendTyping(ctx context.Context, peer InputPeer, action string) error
	SetOnlineStatus(ctx context.Context, online bool) error
}

// PeerResolver resolves string identifiers to InputPeer.
type PeerResolver interface {
	ResolvePeer(ctx context.Context, identifier string) (InputPeer, error)
}
