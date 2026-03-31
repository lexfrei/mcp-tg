package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// mockClient implements telegram.Client for testing.
type mockClient struct {
	// Return values
	messages []telegram.Message
	message  *telegram.Message
	total    int
	dialogs  []telegram.Dialog
	user     *telegram.User
	users    []telegram.User
	group    *telegram.GroupInfo
	info     *telegram.PeerInfo
	infos    []telegram.PeerInfo
	photos   []telegram.Photo
	topics   []telegram.ForumTopic
	sets     []telegram.StickerSet
	setFull  *telegram.StickerSetFull
	folders  []telegram.Folder
	folder   *telegram.Folder
	uploaded *telegram.UploadedFile
	link     string
	filePath string
	peer     telegram.InputPeer

	// Error to return
	err error

	// Last call tracking
	lastPeer  telegram.InputPeer
	lastQuery string
}

func (m *mockClient) ResolvePeer(_ context.Context, identifier string) (telegram.InputPeer, error) {
	m.lastQuery = identifier

	return m.peer, m.err
}

func (m *mockClient) GetMessages(_ context.Context, peer telegram.InputPeer, _ []int) ([]telegram.Message, error) {
	m.lastPeer = peer

	return m.messages, m.err
}

func (m *mockClient) GetHistory(_ context.Context, peer telegram.InputPeer, _ telegram.HistoryOpts) ([]telegram.Message, int, error) {
	m.lastPeer = peer

	return m.messages, m.total, m.err
}

func (m *mockClient) SearchMessages(_ context.Context, peer telegram.InputPeer, query string, _ telegram.SearchOpts) ([]telegram.Message, error) {
	m.lastPeer = peer
	m.lastQuery = query

	return m.messages, m.err
}

func (m *mockClient) SendMessage(_ context.Context, peer telegram.InputPeer, _ string, _ telegram.SendOpts) (*telegram.Message, error) {
	m.lastPeer = peer

	return m.message, m.err
}

func (m *mockClient) EditMessage(_ context.Context, peer telegram.InputPeer, _ int, _ string) (*telegram.Message, error) {
	m.lastPeer = peer

	return m.message, m.err
}

func (m *mockClient) DeleteMessages(_ context.Context, peer telegram.InputPeer, _ []int, _ bool) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) ForwardMessages(_ context.Context, _, to telegram.InputPeer, _ []int) ([]telegram.Message, error) {
	m.lastPeer = to

	return m.messages, m.err
}

func (m *mockClient) PinMessage(_ context.Context, peer telegram.InputPeer, _ int, _ bool) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) SendReaction(_ context.Context, peer telegram.InputPeer, _ int, _ string, _ bool) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) MarkRead(_ context.Context, peer telegram.InputPeer, _ int) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) SendFile(_ context.Context, peer telegram.InputPeer, _, _ string) (*telegram.Message, error) {
	m.lastPeer = peer

	return m.message, m.err
}

func (m *mockClient) SendAlbum(_ context.Context, peer telegram.InputPeer, _ []string, _ string) ([]telegram.Message, error) {
	m.lastPeer = peer

	return m.messages, m.err
}

func (m *mockClient) DownloadMedia(_ context.Context, _ *telegram.Message, _ string) (string, error) {
	return m.filePath, m.err
}

func (m *mockClient) UploadFile(_ context.Context, _ string) (*telegram.UploadedFile, error) {
	return m.uploaded, m.err
}

func (m *mockClient) GetDialogs(_ context.Context, _ telegram.DialogOpts) ([]telegram.Dialog, error) {
	return m.dialogs, m.err
}

func (m *mockClient) SearchDialogs(_ context.Context, query string) ([]telegram.Dialog, error) {
	m.lastQuery = query

	return m.dialogs, m.err
}

func (m *mockClient) GetPeerInfo(_ context.Context, peer telegram.InputPeer) (*telegram.PeerInfo, error) {
	m.lastPeer = peer

	return m.info, m.err
}

func (m *mockClient) GetContact(_ context.Context, peer telegram.InputPeer) (*telegram.User, error) {
	m.lastPeer = peer

	return m.user, m.err
}

func (m *mockClient) SearchContacts(_ context.Context, query string, _ int) ([]telegram.User, error) {
	m.lastQuery = query

	return m.users, m.err
}

func (m *mockClient) GetGroupInfo(_ context.Context, peer telegram.InputPeer) (*telegram.GroupInfo, error) {
	m.lastPeer = peer

	return m.group, m.err
}

func (m *mockClient) JoinGroup(_ context.Context, peer telegram.InputPeer) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) LeaveGroup(_ context.Context, peer telegram.InputPeer) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) RenameGroup(_ context.Context, peer telegram.InputPeer, _ string) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) AddGroupMember(_ context.Context, group, _ telegram.InputPeer) error {
	m.lastPeer = group

	return m.err
}

func (m *mockClient) RemoveGroupMember(_ context.Context, group, _ telegram.InputPeer) error {
	m.lastPeer = group

	return m.err
}

func (m *mockClient) GetInviteLink(_ context.Context, peer telegram.InputPeer) (string, error) {
	m.lastPeer = peer

	return m.link, m.err
}

func (m *mockClient) RevokeInviteLink(_ context.Context, peer telegram.InputPeer, _ string) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) CreateChat(_ context.Context, _ string, _ []telegram.InputPeer, _ bool) (*telegram.PeerInfo, error) {
	return m.info, m.err
}

func (m *mockClient) ArchiveChat(_ context.Context, peer telegram.InputPeer, _ bool) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) MuteChat(_ context.Context, peer telegram.InputPeer, _ int) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) DeleteChat(_ context.Context, peer telegram.InputPeer) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) SetChatPhoto(_ context.Context, peer telegram.InputPeer, _ string) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) SetChatAbout(_ context.Context, peer telegram.InputPeer, _ string) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) GetChatAdmins(_ context.Context, peer telegram.InputPeer) ([]telegram.User, error) {
	m.lastPeer = peer

	return m.users, m.err
}

func (m *mockClient) SetChatPermissions(_ context.Context, peer telegram.InputPeer, _ telegram.ChatPermissions) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) GetSelf(_ context.Context) (*telegram.User, error) {
	return m.user, m.err
}

func (m *mockClient) GetUser(_ context.Context, peer telegram.InputPeer) (*telegram.User, error) {
	m.lastPeer = peer

	return m.user, m.err
}

func (m *mockClient) GetUserPhotos(_ context.Context, peer telegram.InputPeer, _ int) ([]telegram.Photo, error) {
	m.lastPeer = peer

	return m.photos, m.err
}

func (m *mockClient) SetProfileName(_ context.Context, _, _ string) error {
	return m.err
}

func (m *mockClient) SetProfileBio(_ context.Context, _ string) error {
	return m.err
}

func (m *mockClient) SetProfilePhoto(_ context.Context, _ string) error {
	return m.err
}

func (m *mockClient) BlockUser(_ context.Context, peer telegram.InputPeer, _ bool) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) GetCommonChats(_ context.Context, peer telegram.InputPeer) ([]telegram.PeerInfo, error) {
	m.lastPeer = peer

	return m.infos, m.err
}

func (m *mockClient) GetForumTopics(_ context.Context, peer telegram.InputPeer, _ telegram.TopicOpts) ([]telegram.ForumTopic, error) {
	m.lastPeer = peer

	return m.topics, m.err
}

func (m *mockClient) SearchStickerSets(_ context.Context, query string) ([]telegram.StickerSet, error) {
	m.lastQuery = query

	return m.sets, m.err
}

func (m *mockClient) GetStickerSet(_ context.Context, _ string) (*telegram.StickerSetFull, error) {
	return m.setFull, m.err
}

func (m *mockClient) SendSticker(_ context.Context, peer telegram.InputPeer, _ int64) (*telegram.Message, error) {
	m.lastPeer = peer

	return m.message, m.err
}

func (m *mockClient) SetDraft(_ context.Context, peer telegram.InputPeer, _ string, _ int) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) ClearDraft(_ context.Context, peer telegram.InputPeer) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) GetFolders(_ context.Context) ([]telegram.Folder, error) {
	return m.folders, m.err
}

func (m *mockClient) CreateFolder(_ context.Context, _ string, _ []telegram.InputPeer) (*telegram.Folder, error) {
	return m.folder, m.err
}

func (m *mockClient) EditFolder(_ context.Context, _ int, _ string, _ []telegram.InputPeer) error {
	return m.err
}

func (m *mockClient) DeleteFolder(_ context.Context, _ int) error {
	return m.err
}

func (m *mockClient) SendTyping(_ context.Context, peer telegram.InputPeer, _ string) error {
	m.lastPeer = peer

	return m.err
}

func (m *mockClient) SetOnlineStatus(_ context.Context, _ bool) error {
	return m.err
}
