// Package testutil provides test utilities for the mcp-tg project.
package testutil

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// NoopClient implements telegram.Client returning zero values for all methods.
// Used for registration sanity checks in tests.
type NoopClient struct{}

func (NoopClient) ResolvePeer(_ context.Context, _ string) (telegram.InputPeer, error) {
	return telegram.InputPeer{}, nil
}

func (NoopClient) GetMessages(_ context.Context, _ telegram.InputPeer, _ []int) ([]telegram.Message, error) {
	return nil, nil
}

func (NoopClient) GetHistory(_ context.Context, _ telegram.InputPeer, _ telegram.HistoryOpts) ([]telegram.Message, int, error) {
	return nil, 0, nil
}

func (NoopClient) SearchMessages(_ context.Context, _ telegram.InputPeer, _ string, _ telegram.SearchOpts) ([]telegram.Message, error) {
	return nil, nil
}

func (NoopClient) SendMessage(_ context.Context, _ telegram.InputPeer, _ string, _ telegram.SendOpts) (*telegram.Message, error) {
	return nil, nil
}

func (NoopClient) EditMessage(_ context.Context, _ telegram.InputPeer, _ int, _ string) (*telegram.Message, error) {
	return nil, nil
}

func (NoopClient) DeleteMessages(_ context.Context, _ telegram.InputPeer, _ []int, _ bool) error {
	return nil
}

func (NoopClient) ForwardMessages(_ context.Context, _, _ telegram.InputPeer, _ []int) ([]telegram.Message, error) {
	return nil, nil
}

func (NoopClient) PinMessage(_ context.Context, _ telegram.InputPeer, _ int, _ bool) error {
	return nil
}

func (NoopClient) SendReaction(_ context.Context, _ telegram.InputPeer, _ int, _ string, _ bool) error {
	return nil
}

func (NoopClient) MarkRead(_ context.Context, _ telegram.InputPeer, _ int) error {
	return nil
}

func (NoopClient) SendFile(_ context.Context, _ telegram.InputPeer, _, _ string) (*telegram.Message, error) {
	return nil, nil
}

func (NoopClient) SendAlbum(_ context.Context, _ telegram.InputPeer, _ []string, _ string) ([]telegram.Message, error) {
	return nil, nil
}

func (NoopClient) DownloadMedia(_ context.Context, _ telegram.InputPeer, _ int, _ string) (string, error) {
	return "", nil
}

func (NoopClient) UploadFile(_ context.Context, _ string) (*telegram.UploadedFile, error) {
	return nil, nil
}

func (NoopClient) GetDialogs(_ context.Context, _ telegram.DialogOpts) ([]telegram.Dialog, error) {
	return nil, nil
}

func (NoopClient) SearchDialogs(_ context.Context, _ string) ([]telegram.Dialog, error) {
	return nil, nil
}

func (NoopClient) GetPeerInfo(_ context.Context, _ telegram.InputPeer) (*telegram.PeerInfo, error) {
	return nil, nil
}

func (NoopClient) GetContact(_ context.Context, _ telegram.InputPeer) (*telegram.User, error) {
	return nil, nil
}

func (NoopClient) SearchContacts(_ context.Context, _ string, _ int) ([]telegram.User, error) {
	return nil, nil
}

func (NoopClient) GetGroupInfo(_ context.Context, _ telegram.InputPeer) (*telegram.GroupInfo, error) {
	return nil, nil
}

func (NoopClient) JoinGroup(_ context.Context, _ telegram.InputPeer) error  { return nil }
func (NoopClient) LeaveGroup(_ context.Context, _ telegram.InputPeer) error { return nil }

func (NoopClient) RenameGroup(_ context.Context, _ telegram.InputPeer, _ string) error {
	return nil
}

func (NoopClient) AddGroupMember(_ context.Context, _, _ telegram.InputPeer) error    { return nil }
func (NoopClient) RemoveGroupMember(_ context.Context, _, _ telegram.InputPeer) error { return nil }

func (NoopClient) GetInviteLink(_ context.Context, _ telegram.InputPeer) (string, error) {
	return "", nil
}

func (NoopClient) RevokeInviteLink(_ context.Context, _ telegram.InputPeer, _ string) error {
	return nil
}

func (NoopClient) CreateChat(_ context.Context, _ string, _ []telegram.InputPeer, _ bool) (*telegram.PeerInfo, error) {
	return nil, nil
}

func (NoopClient) ArchiveChat(_ context.Context, _ telegram.InputPeer, _ bool) error { return nil }
func (NoopClient) MuteChat(_ context.Context, _ telegram.InputPeer, _ int) error     { return nil }
func (NoopClient) DeleteChat(_ context.Context, _ telegram.InputPeer) error          { return nil }

func (NoopClient) SetChatPhoto(_ context.Context, _ telegram.InputPeer, _ string) error { return nil }
func (NoopClient) SetChatAbout(_ context.Context, _ telegram.InputPeer, _ string) error { return nil }

func (NoopClient) GetChatAdmins(_ context.Context, _ telegram.InputPeer) ([]telegram.User, error) {
	return nil, nil
}

func (NoopClient) SetChatPermissions(_ context.Context, _ telegram.InputPeer, _ telegram.ChatPermissions) error {
	return nil
}

func (NoopClient) GetSelf(_ context.Context) (*telegram.User, error) { return nil, nil }
func (NoopClient) GetUser(_ context.Context, _ telegram.InputPeer) (*telegram.User, error) {
	return nil, nil
}

func (NoopClient) GetUserPhotos(_ context.Context, _ telegram.InputPeer, _ int) ([]telegram.Photo, error) {
	return nil, nil
}

func (NoopClient) SetProfileName(_ context.Context, _, _ string) error             { return nil }
func (NoopClient) SetProfileBio(_ context.Context, _ string) error                 { return nil }
func (NoopClient) SetProfilePhoto(_ context.Context, _ string) error               { return nil }
func (NoopClient) BlockUser(_ context.Context, _ telegram.InputPeer, _ bool) error { return nil }

func (NoopClient) GetCommonChats(_ context.Context, _ telegram.InputPeer) ([]telegram.PeerInfo, error) {
	return nil, nil
}

func (NoopClient) GetForumTopics(_ context.Context, _ telegram.InputPeer, _ telegram.TopicOpts) ([]telegram.ForumTopic, error) {
	return nil, nil
}

func (NoopClient) SearchStickerSets(_ context.Context, _ string) ([]telegram.StickerSet, error) {
	return nil, nil
}

func (NoopClient) GetStickerSet(_ context.Context, _ string) (*telegram.StickerSetFull, error) {
	return nil, nil
}

func (NoopClient) SendSticker(_ context.Context, _ telegram.InputPeer, _ int64) (*telegram.Message, error) {
	return nil, nil
}

func (NoopClient) SetDraft(_ context.Context, _ telegram.InputPeer, _ string, _ int) error {
	return nil
}
func (NoopClient) ClearDraft(_ context.Context, _ telegram.InputPeer) error { return nil }

func (NoopClient) GetFolders(_ context.Context) ([]telegram.Folder, error) { return nil, nil }

func (NoopClient) CreateFolder(_ context.Context, _ string, _ []telegram.InputPeer) (*telegram.Folder, error) {
	return nil, nil
}

func (NoopClient) EditFolder(_ context.Context, _ int, _ string, _ []telegram.InputPeer) error {
	return nil
}

func (NoopClient) DeleteFolder(_ context.Context, _ int) error                        { return nil }
func (NoopClient) SendTyping(_ context.Context, _ telegram.InputPeer, _ string) error { return nil }
func (NoopClient) SetOnlineStatus(_ context.Context, _ bool) error                    { return nil }
