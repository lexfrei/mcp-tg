package telegram

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// Wrapper implements Client using gotd/td.
type Wrapper struct {
	api   *tg.Client
	up    *uploader.Uploader
	down  *downloader.Downloader
	cache *PeerCache
}

// cryptoRandID generates a cryptographically random int64 for Telegram's RandomID field.
// Telegram requires a unique RandomID per request to prevent duplicate sends on retry.
func cryptoRandID() (int64, error) {
	var buf [8]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return 0, errors.Wrap(err, "generating random ID")
	}

	return int64(binary.LittleEndian.Uint64(buf[:])), nil //nolint:gosec // Intentional: RandomID is opaque, overflow is harmless.
}

func cryptoRandIDs(count int) ([]int64, error) {
	ids := make([]int64, count)

	for idx := range ids {
		randID, err := cryptoRandID()
		if err != nil {
			return nil, err
		}

		ids[idx] = randID
	}

	return ids, nil
}

// NewWrapper creates a new Wrapper around gotd/td client primitives.
func NewWrapper(api *tg.Client) *Wrapper {
	return &Wrapper{
		api:   api,
		up:    uploader.NewUploader(api),
		down:  downloader.NewDownloader(),
		cache: NewPeerCache(),
	}
}

// ResolvePeer resolves a string identifier to an InputPeer.
// Peers resolved with a valid AccessHash are cached so that
// subsequent numeric-ID lookups can reuse the hash.
func (w *Wrapper) ResolvePeer(
	ctx context.Context,
	identifier string,
) (InputPeer, error) {
	peer, err := Resolve(ctx, w.api, identifier)
	if err != nil {
		return InputPeer{}, err
	}

	if peer.AccessHash != 0 {
		w.cache.Store(peer)

		return peer, nil
	}

	if cached, hit := w.cache.Lookup(peer.Type, peer.ID); hit {
		return cached, nil
	}

	if resolved, ok := w.resolveViaDialogs(ctx, peer); ok {
		return resolved, nil
	}

	return peer, nil
}

// GetSelf returns the authenticated user's profile.
func (w *Wrapper) GetSelf(ctx context.Context) (*User, error) {
	full, err := w.api.UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		return nil, errors.Wrap(err, "getting self")
	}

	for _, usr := range full.Users {
		if u, ok := usr.(*tg.User); ok && u.Self {
			result := ConvertUser(u)
			result.Bio = full.FullUser.About

			return &result, nil
		}
	}

	return nil, errors.New("self user not found in response")
}

// GetDialogs returns a list of dialogs.
func (w *Wrapper) GetDialogs(ctx context.Context, opts DialogOpts) ([]Dialog, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
		Limit:      limit,
		OffsetDate: opts.OffsetDate,
		OffsetPeer: &tg.InputPeerEmpty{},
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting dialogs")
	}

	dialogs := extractDialogs(result)
	w.cacheDialogPeers(dialogs)

	return dialogs, nil
}

// SearchDialogs searches dialogs by query.
func (w *Wrapper) SearchDialogs(ctx context.Context, query string) ([]Dialog, error) {
	result, err := w.api.ContactsSearch(ctx, &tg.ContactsSearchRequest{
		Q:     query,
		Limit: defaultLimit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "searching dialogs")
	}

	dialogs := dialogsFromSearch(result)
	w.cacheDialogPeers(dialogs)

	return dialogs, nil
}

// GetPeerInfo returns metadata about a peer.
func (w *Wrapper) GetPeerInfo(ctx context.Context, peer InputPeer) (*PeerInfo, error) {
	switch peer.Type {
	case PeerUser:
		return w.getUserPeerInfo(ctx, peer)
	case PeerChat:
		return w.getChatPeerInfo(ctx, peer)
	case PeerChannel:
		return w.getChannelPeerInfo(ctx, peer)
	default:
		return nil, errors.New("unknown peer type")
	}
}

// GetHistory retrieves message history from a chat.
func (w *Wrapper) GetHistory(ctx context.Context, peer InputPeer, opts HistoryOpts) ([]Message, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     InputPeerToTG(peer),
		Limit:    limit,
		OffsetID: opts.OffsetID,
	})
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting history")
	}

	msgs, total := extractMessages(result, peer.ID)

	return msgs, total, nil
}

// GetTopicMessages retrieves messages from a specific forum topic.
func (w *Wrapper) GetTopicMessages(
	ctx context.Context, peer InputPeer, topicID int, opts HistoryOpts,
) ([]Message, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
		Peer:     InputPeerToTG(peer),
		MsgID:    topicID,
		Limit:    limit,
		OffsetID: opts.OffsetID,
	})
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting topic messages")
	}

	msgs, total := extractMessages(result, peer.ID)

	return msgs, total, nil
}

// GetMessages retrieves specific messages by ID.
func (w *Wrapper) GetMessages(ctx context.Context, peer InputPeer, ids []int) ([]Message, error) {
	inputIDs := make([]tg.InputMessageClass, len(ids))
	for idx, msgID := range ids {
		inputIDs[idx] = &tg.InputMessageID{ID: msgID}
	}

	var (
		result tg.MessagesMessagesClass
		err    error
	)

	if peer.Type == PeerChannel {
		result, err = w.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: InputChannelFromPeer(peer),
			ID:      inputIDs,
		})
	} else {
		result, err = w.api.MessagesGetMessages(ctx, inputIDs)
	}

	if err != nil {
		return nil, errors.Wrap(err, "getting messages")
	}

	msgs, _ := extractMessages(result, peer.ID)

	return msgs, nil
}

// SearchMessages searches for messages in a chat.
func (w *Wrapper) SearchMessages(ctx context.Context, peer InputPeer, query string, opts SearchOpts) ([]Message, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
		Peer:     InputPeerToTG(peer),
		Q:        query,
		Filter:   &tg.InputMessagesFilterEmpty{},
		Limit:    limit,
		OffsetID: opts.OffsetID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "searching messages")
	}

	msgs, _ := extractMessages(result, peer.ID)

	return msgs, nil
}

// SendMessage sends a text message.
func (w *Wrapper) SendMessage(ctx context.Context, peer InputPeer, text string, opts SendOpts) (*Message, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	req := &tg.MessagesSendMessageRequest{
		Peer:     InputPeerToTG(peer),
		Message:  text,
		RandomID: randID,
	}

	if opts.ParseMode == ParseModeMarkdown {
		plainText, entities := ParseMarkdown(text)
		req.Message = plainText

		if len(entities) > 0 {
			req.SetEntities(entities)
		}
	}

	validErr := validateMessageText(req.Message)
	if validErr != nil {
		return nil, validErr
	}

	if opts.TopicID > 0 || opts.ReplyTo > 0 {
		reply := &tg.InputReplyToMessage{ReplyToMsgID: opts.ReplyTo}
		if opts.TopicID > 0 {
			reply.SetTopMsgID(opts.TopicID)
		}

		req.ReplyTo = reply
	}

	req.Silent = opts.Silent
	req.NoWebpage = opts.NoWebpage

	if opts.ScheduleDate > 0 {
		req.SetScheduleDate(opts.ScheduleDate)
	}

	result, err := w.api.MessagesSendMessage(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending message")
	}

	return messageFromUpdate(result), nil
}

// EditMessage edits an existing message.
func (w *Wrapper) EditMessage(
	ctx context.Context, peer InputPeer, msgID int, text string, parseMode string,
) (*Message, error) {
	req := &tg.MessagesEditMessageRequest{
		Peer:    InputPeerToTG(peer),
		ID:      msgID,
		Message: text,
	}

	if parseMode == ParseModeMarkdown {
		plainText, entities := ParseMarkdown(text)
		req.Message = plainText

		if len(entities) > 0 {
			req.SetEntities(entities)
		}
	}

	validErr := validateMessageText(req.Message)
	if validErr != nil {
		return nil, validErr
	}

	result, err := w.api.MessagesEditMessage(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "editing message")
	}

	return messageFromUpdate(result), nil
}

// DeleteMessages deletes messages from a chat.
func (w *Wrapper) DeleteMessages(ctx context.Context, peer InputPeer, ids []int, revoke bool) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
			Channel: InputChannelFromPeer(peer),
			ID:      ids,
		})

		return errors.Wrap(err, "deleting channel messages")
	}

	_, err := w.api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		ID:     ids,
		Revoke: revoke,
	})

	return errors.Wrap(err, "deleting messages")
}

// ForwardMessages forwards messages from one chat to another.
func (w *Wrapper) ForwardMessages(ctx context.Context, from, dest InputPeer, ids []int) ([]Message, error) {
	randIDs, err := cryptoRandIDs(len(ids))
	if err != nil {
		return nil, err
	}

	result, err := w.api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: InputPeerToTG(from),
		ToPeer:   InputPeerToTG(dest),
		ID:       ids,
		RandomID: randIDs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "forwarding messages")
	}

	return messagesFromUpdates(result), nil
}

// PinMessage pins or unpins a message in a chat.
func (w *Wrapper) PinMessage(ctx context.Context, peer InputPeer, msgID int, unpin bool) error {
	_, err := w.api.MessagesUpdatePinnedMessage(ctx, &tg.MessagesUpdatePinnedMessageRequest{
		Peer:  InputPeerToTG(peer),
		ID:    msgID,
		Unpin: unpin,
	})

	return errors.Wrap(err, "updating pinned message")
}

// SendReaction adds or removes a reaction on a message.
func (w *Wrapper) SendReaction(ctx context.Context, peer InputPeer, msgID int, emoji string, remove bool) error {
	var reactions []tg.ReactionClass
	if !remove {
		reactions = []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
	}

	_, err := w.api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
		Peer:     InputPeerToTG(peer),
		MsgID:    msgID,
		Reaction: reactions,
	})

	return errors.Wrap(err, "sending reaction")
}

// MarkRead marks messages as read up to maxID.
func (w *Wrapper) MarkRead(ctx context.Context, peer InputPeer, maxID int) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
			Channel: InputChannelFromPeer(peer),
			MaxID:   maxID,
		})

		return errors.Wrap(err, "marking channel messages read")
	}

	_, err := w.api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
		Peer:  InputPeerToTG(peer),
		MaxID: maxID,
	})

	return errors.Wrap(err, "marking messages read")
}

// SendFile sends a file with an optional caption.
// Uses Silent, ScheduleDate from opts.
// ParseMode and NoWebpage are not applicable to media sends.
func (w *Wrapper) SendFile(ctx context.Context, peer InputPeer, path, caption string, opts SendOpts) (*Message, error) {
	file, err := w.up.FromPath(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "uploading file")
	}

	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	req := &tg.MessagesSendMediaRequest{
		Peer: InputPeerToTG(peer),
		Media: &tg.InputMediaUploadedDocument{
			File:     file,
			MimeType: mimeByPath(path),
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: filepath.Base(path)},
			},
		},
		Message:  caption,
		RandomID: randID,
		Silent:   opts.Silent,
	}

	if opts.TopicID > 0 {
		reply := &tg.InputReplyToMessage{}
		reply.SetTopMsgID(opts.TopicID)

		req.ReplyTo = reply
	}

	if opts.ScheduleDate > 0 {
		req.SetScheduleDate(opts.ScheduleDate)
	}

	result, err := w.api.MessagesSendMedia(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending file")
	}

	return messageFromUpdate(result), nil
}

// SendAlbum sends a group of media files.
// Uses Silent, ScheduleDate from opts.
// ParseMode and NoWebpage are not applicable to media sends.
func (w *Wrapper) SendAlbum(ctx context.Context, peer InputPeer, paths []string, caption string, opts SendOpts) ([]Message, error) {
	multiMedia := make([]tg.InputSingleMedia, 0, len(paths))

	for idx, path := range paths {
		file, err := w.up.FromPath(ctx, path)
		if err != nil {
			return nil, errors.Wrapf(err, "uploading file %d", idx)
		}

		randID, randErr := cryptoRandID()
		if randErr != nil {
			return nil, randErr
		}

		media := tg.InputSingleMedia{
			RandomID: randID,
			Media: &tg.InputMediaUploadedDocument{
				File:     file,
				MimeType: mimeByPath(path),
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeFilename{FileName: filepath.Base(path)},
				},
			},
		}

		if idx == 0 {
			media.Message = caption
		}

		multiMedia = append(multiMedia, media)
	}

	req := buildMultiMediaRequest(peer, multiMedia, opts)

	result, err := w.api.MessagesSendMultiMedia(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending album")
	}

	return messagesFromUpdates(result), nil
}

// DownloadMedia downloads media from a message to the specified directory.
func (w *Wrapper) DownloadMedia(ctx context.Context, peer InputPeer, msgID int, outputDir string) (string, error) {
	rawMsg, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return "", errors.Wrap(err, "getting message for download")
	}

	location, fileName := extractMediaLocation(rawMsg)
	if location == nil {
		return "", errors.New("message has no downloadable media")
	}

	mkdirErr := os.MkdirAll(outputDir, outputDirPerms)
	if mkdirErr != nil {
		return "", errors.Wrap(mkdirErr, "creating output directory")
	}

	outPath := filepath.Join(outputDir, fileName)

	_, err = w.down.Download(w.api, location).ToPath(ctx, outPath)
	if err != nil {
		return "", errors.Wrap(err, "downloading media")
	}

	return outPath, nil
}

// UploadFile uploads a file and returns its metadata.
func (w *Wrapper) UploadFile(ctx context.Context, path string) (*UploadedFile, error) {
	file, err := w.up.FromPath(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "uploading file")
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		return nil, errors.Wrap(statErr, "stat file")
	}

	return &UploadedFile{
		ID:   uploadedFileID(file),
		Name: filepath.Base(path),
		Size: info.Size(),
	}, nil
}

// GetContact returns a user by peer.
func (w *Wrapper) GetContact(ctx context.Context, peer InputPeer) (*User, error) {
	return w.GetUser(ctx, peer)
}

// SearchContacts searches contacts by query.
func (w *Wrapper) SearchContacts(ctx context.Context, query string, limit int) ([]User, error) {
	result, err := w.api.ContactsSearch(ctx, &tg.ContactsSearchRequest{
		Q:     query,
		Limit: limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "searching contacts")
	}

	users := make([]User, 0, len(result.Users))

	for _, usr := range result.Users {
		if u, ok := usr.(*tg.User); ok {
			users = append(users, ConvertUser(u))
		}
	}

	return users, nil
}

// GetUser returns user info by peer.
func (w *Wrapper) GetUser(ctx context.Context, peer InputPeer) (*User, error) {
	full, err := w.api.UsersGetFullUser(ctx, InputUserFromPeer(peer))
	if err != nil {
		return nil, errors.Wrap(err, "getting user")
	}

	for _, usr := range full.Users {
		if u, ok := usr.(*tg.User); ok && u.ID == peer.ID {
			result := ConvertUser(u)
			result.Bio = full.FullUser.About

			return &result, nil
		}
	}

	return nil, errors.New("user not found in response")
}

// GetUserPhotos returns profile photos for a user.
func (w *Wrapper) GetUserPhotos(ctx context.Context, peer InputPeer, limit int) ([]Photo, error) {
	result, err := w.api.PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{
		UserID: InputUserFromPeer(peer),
		Limit:  limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting user photos")
	}

	return extractPhotos(result), nil
}

// SetProfileName updates the authenticated user's name.
func (w *Wrapper) SetProfileName(ctx context.Context, firstName, lastName string) error {
	req := &tg.AccountUpdateProfileRequest{}
	req.SetFirstName(firstName)
	req.SetLastName(lastName)

	_, err := w.api.AccountUpdateProfile(ctx, req)

	return errors.Wrap(err, "updating profile name")
}

// SetProfileBio updates the authenticated user's bio.
func (w *Wrapper) SetProfileBio(ctx context.Context, bio string) error {
	req := &tg.AccountUpdateProfileRequest{}
	req.SetAbout(bio)

	_, err := w.api.AccountUpdateProfile(ctx, req)

	return errors.Wrap(err, "updating profile bio")
}

// SetProfilePhoto sets the authenticated user's profile photo.
func (w *Wrapper) SetProfilePhoto(ctx context.Context, path string) error {
	if !isImagePath(path) {
		return errors.New("file is not an image (expected jpg, png, or webp)")
	}

	file, err := w.up.FromPath(ctx, path)
	if err != nil {
		return errors.Wrap(err, "uploading profile photo")
	}

	_, err = w.api.PhotosUploadProfilePhoto(ctx, &tg.PhotosUploadProfilePhotoRequest{
		File: file,
	})

	return errors.Wrap(err, "setting profile photo")
}

// BlockUser blocks or unblocks a user.
func (w *Wrapper) BlockUser(ctx context.Context, peer InputPeer, block bool) error {
	if block {
		_, err := w.api.ContactsBlock(ctx, &tg.ContactsBlockRequest{
			ID: InputPeerToTG(peer),
		})

		return errors.Wrap(err, "blocking user")
	}

	_, err := w.api.ContactsUnblock(ctx, &tg.ContactsUnblockRequest{
		ID: InputPeerToTG(peer),
	})

	return errors.Wrap(err, "unblocking user")
}

// GetCommonChats returns chats shared with a user.
func (w *Wrapper) GetCommonChats(ctx context.Context, peer InputPeer) ([]PeerInfo, error) {
	result, err := w.api.MessagesGetCommonChats(ctx, &tg.MessagesGetCommonChatsRequest{
		UserID: InputUserFromPeer(peer),
		Limit:  defaultLimit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting common chats")
	}

	return peerInfosFromChats(result), nil
}

// GetGroupInfo returns detailed info about a group or channel.
func (w *Wrapper) GetGroupInfo(ctx context.Context, peer InputPeer) (*GroupInfo, error) {
	if peer.Type == PeerChannel {
		return w.getChannelGroupInfo(ctx, peer)
	}

	return w.getChatGroupInfo(ctx, peer)
}

// JoinGroup joins a group or channel.
func (w *Wrapper) JoinGroup(ctx context.Context, peer InputPeer) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsJoinChannel(ctx, InputChannelFromPeer(peer))

		return errors.Wrap(err, "joining channel")
	}

	return errors.New("joining basic chats requires an invite link")
}

// LeaveGroup leaves a group or channel.
func (w *Wrapper) LeaveGroup(ctx context.Context, peer InputPeer) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsLeaveChannel(ctx, InputChannelFromPeer(peer))

		return errors.Wrap(err, "leaving channel")
	}

	_, err := w.api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
		ChatID: peer.ID,
		UserID: &tg.InputUserSelf{},
	})

	return errors.Wrap(err, "leaving chat")
}

// RenameGroup renames a group or channel.
func (w *Wrapper) RenameGroup(ctx context.Context, peer InputPeer, title string) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsEditTitle(ctx, &tg.ChannelsEditTitleRequest{
			Channel: InputChannelFromPeer(peer),
			Title:   title,
		})

		return errors.Wrap(err, "renaming channel")
	}

	_, err := w.api.MessagesEditChatTitle(ctx, &tg.MessagesEditChatTitleRequest{
		ChatID: peer.ID,
		Title:  title,
	})

	return errors.Wrap(err, "renaming chat")
}

// AddGroupMember adds a user to a group.
func (w *Wrapper) AddGroupMember(ctx context.Context, group, user InputPeer) error {
	if group.Type == PeerChannel {
		_, err := w.api.ChannelsInviteToChannel(ctx, &tg.ChannelsInviteToChannelRequest{
			Channel: InputChannelFromPeer(group),
			Users:   []tg.InputUserClass{InputUserFromPeer(user)},
		})

		return errors.Wrap(err, "adding channel member")
	}

	_, err := w.api.MessagesAddChatUser(ctx, &tg.MessagesAddChatUserRequest{
		ChatID:   group.ID,
		UserID:   InputUserFromPeer(user),
		FwdLimit: defaultLimit,
	})

	return errors.Wrap(err, "adding chat member")
}

// RemoveGroupMember removes a user from a group.
func (w *Wrapper) RemoveGroupMember(ctx context.Context, group, user InputPeer) error {
	if group.Type == PeerChannel {
		rights := tg.ChatBannedRights{}
		rights.SetViewMessages(true)
		rights.UntilDate = int(time.Now().Add(time.Minute).Unix())

		_, err := w.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
			Channel:      InputChannelFromPeer(group),
			Participant:  InputPeerToTG(user),
			BannedRights: rights,
		})

		return errors.Wrap(err, "removing channel member")
	}

	_, err := w.api.MessagesDeleteChatUser(ctx, &tg.MessagesDeleteChatUserRequest{
		ChatID: group.ID,
		UserID: InputUserFromPeer(user),
	})

	return errors.Wrap(err, "removing chat member")
}

// GetInviteLink returns the invite link for a group or channel.
func (w *Wrapper) GetInviteLink(ctx context.Context, peer InputPeer) (string, error) {
	result, err := w.api.MessagesExportChatInvite(ctx, &tg.MessagesExportChatInviteRequest{
		Peer: InputPeerToTG(peer),
	})
	if err != nil {
		return "", errors.Wrap(err, "exporting invite link")
	}

	if link, ok := result.(*tg.ChatInviteExported); ok {
		return link.Link, nil
	}

	return "", errors.New("unexpected invite link type")
}

// RevokeInviteLink revokes an invite link.
func (w *Wrapper) RevokeInviteLink(ctx context.Context, peer InputPeer, link string) error {
	_, err := w.api.MessagesEditExportedChatInvite(ctx, &tg.MessagesEditExportedChatInviteRequest{
		Peer:    InputPeerToTG(peer),
		Link:    link,
		Revoked: true,
	})

	return errors.Wrap(err, "revoking invite link")
}

// CreateChat creates a new group or channel.
func (w *Wrapper) CreateChat(ctx context.Context, title string, users []InputPeer, isChannel bool) (*PeerInfo, error) {
	if isChannel {
		return w.createChannel(ctx, title)
	}

	return w.createBasicChat(ctx, title, users)
}

// ArchiveChat archives or unarchives a chat.
func (w *Wrapper) ArchiveChat(ctx context.Context, peer InputPeer, archive bool) error {
	folderID := 0
	if archive {
		folderID = 1
	}

	_, err := w.api.FoldersEditPeerFolders(ctx, []tg.InputFolderPeer{
		{Peer: InputPeerToTG(peer), FolderID: folderID},
	})

	return errors.Wrap(err, "archiving chat")
}

// MuteChat mutes or unmutes a chat's notifications.
func (w *Wrapper) MuteChat(ctx context.Context, peer InputPeer, muteUntil int) error {
	settings := tg.InputPeerNotifySettings{}
	settings.SetMuteUntil(muteUntil)

	_, err := w.api.AccountUpdateNotifySettings(ctx, &tg.AccountUpdateNotifySettingsRequest{
		Peer:     &tg.InputNotifyPeer{Peer: InputPeerToTG(peer)},
		Settings: settings,
	})

	return errors.Wrap(err, "muting chat")
}

// DeleteChat deletes a chat.
func (w *Wrapper) DeleteChat(ctx context.Context, peer InputPeer) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsDeleteChannel(ctx, InputChannelFromPeer(peer))

		return errors.Wrap(err, "deleting channel")
	}

	return errors.New("deleting basic chats is not supported by Telegram API")
}

// SetChatPhoto sets the photo for a group or channel.
func (w *Wrapper) SetChatPhoto(ctx context.Context, peer InputPeer, path string) error {
	if !isImagePath(path) {
		return errors.New("file is not an image (expected jpg, png, or webp)")
	}

	file, err := w.up.FromPath(ctx, path)
	if err != nil {
		return errors.Wrap(err, "uploading chat photo")
	}

	chatPhoto := &tg.InputChatUploadedPhoto{File: file}

	if peer.Type == PeerChannel {
		_, editErr := w.api.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{
			Channel: InputChannelFromPeer(peer),
			Photo:   chatPhoto,
		})

		return errors.Wrap(editErr, "setting channel photo")
	}

	_, editErr := w.api.MessagesEditChatPhoto(ctx, &tg.MessagesEditChatPhotoRequest{
		ChatID: peer.ID,
		Photo:  chatPhoto,
	})

	return errors.Wrap(editErr, "setting chat photo")
}

// SetChatAbout sets the description/about text for a group or channel.
func (w *Wrapper) SetChatAbout(ctx context.Context, peer InputPeer, about string) error {
	_, err := w.api.MessagesEditChatAbout(ctx, &tg.MessagesEditChatAboutRequest{
		Peer:  InputPeerToTG(peer),
		About: about,
	})

	return errors.Wrap(err, "setting chat about")
}

// GetChatAdmins returns the administrators of a chat.
func (w *Wrapper) GetChatAdmins(ctx context.Context, peer InputPeer) ([]User, error) {
	if peer.Type != PeerChannel {
		return nil, errors.New("admin list is only available for channels and supergroups")
	}

	result, err := w.api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: InputChannelFromPeer(peer),
		Filter:  &tg.ChannelParticipantsAdmins{},
		Limit:   defaultLimit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting chat admins")
	}

	return usersFromParticipants(result), nil
}

// SetChatPermissions sets default permissions for a chat.
func (w *Wrapper) SetChatPermissions(ctx context.Context, peer InputPeer, perms ChatPermissions) error {
	rights := convertPermissions(perms)

	_, err := w.api.MessagesEditChatDefaultBannedRights(ctx, &tg.MessagesEditChatDefaultBannedRightsRequest{
		Peer:         InputPeerToTG(peer),
		BannedRights: rights,
	})

	return errors.Wrap(err, "setting chat permissions")
}

// GetForumTopics returns forum topics for a supergroup.
func (w *Wrapper) GetForumTopics(ctx context.Context, peer InputPeer, opts TopicOpts) ([]ForumTopic, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesGetForumTopics(ctx, &tg.MessagesGetForumTopicsRequest{
		Peer:  InputPeerToTG(peer),
		Limit: limit,
		Q:     opts.Query,
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting forum topics")
	}

	return extractForumTopics(result), nil
}

// SearchStickerSets searches for sticker sets by query.
func (w *Wrapper) SearchStickerSets(ctx context.Context, query string) ([]StickerSet, error) {
	result, err := w.api.MessagesSearchStickerSets(ctx, &tg.MessagesSearchStickerSetsRequest{
		Q: query,
	})
	if err != nil {
		return nil, errors.Wrap(err, "searching sticker sets")
	}

	return extractStickerSets(result), nil
}

// GetStickerSet returns a sticker set by short name.
func (w *Wrapper) GetStickerSet(ctx context.Context, name string) (*StickerSetFull, error) {
	result, err := w.api.MessagesGetStickerSet(ctx, &tg.MessagesGetStickerSetRequest{
		Stickerset: &tg.InputStickerSetShortName{ShortName: name},
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting sticker set")
	}

	stickerSet, ok := result.(*tg.MessagesStickerSet)
	if !ok {
		return nil, errors.New("unexpected sticker set type")
	}

	return convertStickerSetFull(stickerSet), nil
}

// SendSticker sends a sticker to a chat.
func (w *Wrapper) SendSticker(ctx context.Context, peer InputPeer, stickerFileID int64) (*Message, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	result, err := w.api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: InputPeerToTG(peer),
		Media: &tg.InputMediaDocument{
			ID: &tg.InputDocument{ID: stickerFileID},
		},
		RandomID: randID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "sending sticker")
	}

	return messageFromUpdate(result), nil
}

// SetDraft sets a draft message in a chat.
func (w *Wrapper) SetDraft(ctx context.Context, peer InputPeer, text string, replyTo int) error {
	req := &tg.MessagesSaveDraftRequest{
		Peer:    InputPeerToTG(peer),
		Message: text,
	}

	if replyTo > 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyTo}
	}

	_, err := w.api.MessagesSaveDraft(ctx, req)

	return errors.Wrap(err, "setting draft")
}

// ClearDraft clears a draft message in a chat.
func (w *Wrapper) ClearDraft(ctx context.Context, peer InputPeer) error {
	_, err := w.api.MessagesSaveDraft(ctx, &tg.MessagesSaveDraftRequest{
		Peer:    InputPeerToTG(peer),
		Message: "",
	})

	return errors.Wrap(err, "clearing draft")
}

// GetFolders returns chat folders.
func (w *Wrapper) GetFolders(ctx context.Context) ([]Folder, error) {
	result, err := w.api.MessagesGetDialogFilters(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting folders")
	}

	return extractFolders(result), nil
}

// CreateFolder creates a new chat folder.
func (w *Wrapper) CreateFolder(ctx context.Context, title string, peers []InputPeer) (*Folder, error) {
	includePeers := make([]tg.InputPeerClass, len(peers))
	for idx, peer := range peers {
		includePeers[idx] = InputPeerToTG(peer)
	}

	filter := tg.DialogFilter{
		Title:        tg.TextWithEntities{Text: title},
		IncludePeers: includePeers,
	}

	_, err := w.api.MessagesUpdateDialogFilter(ctx, &tg.MessagesUpdateDialogFilterRequest{
		Filter: &filter,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating folder")
	}

	return w.findFolderByTitle(ctx, title, peers)
}

// EditFolder updates an existing chat folder.
func (w *Wrapper) EditFolder(ctx context.Context, folderID int, title string, peers []InputPeer) error {
	includePeers := make([]tg.InputPeerClass, len(peers))
	for idx, peer := range peers {
		includePeers[idx] = InputPeerToTG(peer)
	}

	filter := tg.DialogFilter{
		ID:           folderID,
		Title:        tg.TextWithEntities{Text: title},
		IncludePeers: includePeers,
	}

	_, err := w.api.MessagesUpdateDialogFilter(ctx, &tg.MessagesUpdateDialogFilterRequest{
		ID:     folderID,
		Filter: &filter,
	})

	return errors.Wrap(err, "editing folder")
}

// DeleteFolder deletes a chat folder.
func (w *Wrapper) DeleteFolder(ctx context.Context, folderID int) error {
	_, err := w.api.MessagesUpdateDialogFilter(ctx, &tg.MessagesUpdateDialogFilterRequest{
		ID: folderID,
	})

	return errors.Wrap(err, "deleting folder")
}

// SendTyping sends a typing indicator.
func (w *Wrapper) SendTyping(ctx context.Context, peer InputPeer, action string) error {
	tgAction := typingAction(action)

	_, err := w.api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{
		Peer:   InputPeerToTG(peer),
		Action: tgAction,
	})

	return errors.Wrap(err, "sending typing")
}

// SetOnlineStatus sets the online/offline status.
func (w *Wrapper) SetOnlineStatus(ctx context.Context, online bool) error {
	_, err := w.api.AccountUpdateStatus(ctx, !online)

	return errors.Wrap(err, "setting online status")
}

// GetScheduledMessages returns scheduled messages for a chat.
func (w *Wrapper) GetScheduledMessages(
	ctx context.Context, peer InputPeer,
) ([]Message, error) {
	result, err := w.api.MessagesGetScheduledHistory(
		ctx,
		&tg.MessagesGetScheduledHistoryRequest{
			Peer: InputPeerToTG(peer),
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting scheduled messages")
	}

	msgs, _ := extractMessages(result, peer.ID)

	return msgs, nil
}

// SearchGlobal searches messages across all chats.
func (w *Wrapper) SearchGlobal(
	ctx context.Context, query string, limit int,
) ([]Message, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesSearchGlobal(
		ctx,
		&tg.MessagesSearchGlobalRequest{
			Q:          query,
			Limit:      limit,
			OffsetPeer: &tg.InputPeerEmpty{},
			Filter:     &tg.InputMessagesFilterEmpty{},
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "searching global messages")
	}

	msgs, _ := extractMessages(result, 0)

	return msgs, nil
}

// GetBlockedContacts returns a list of blocked users.
func (w *Wrapper) GetBlockedContacts(
	ctx context.Context, limit int,
) ([]User, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.ContactsGetBlocked(
		ctx,
		&tg.ContactsGetBlockedRequest{Limit: limit},
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting blocked contacts")
	}

	return extractBlockedUsers(result), nil
}

// GetReactions returns users who reacted to a message.
func (w *Wrapper) GetReactions(
	ctx context.Context, peer InputPeer, msgID int, limit int,
) ([]ReactionUser, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.MessagesGetMessageReactionsList(
		ctx,
		&tg.MessagesGetMessageReactionsListRequest{
			Peer:  InputPeerToTG(peer),
			ID:    msgID,
			Limit: limit,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting reactions")
	}

	return extractReactionUsers(result), nil
}

// GetGroupMembers returns members of a channel/supergroup.
func (w *Wrapper) GetGroupMembers(
	ctx context.Context,
	peer InputPeer,
	filter string,
	limit int,
) ([]User, error) {
	if peer.Type != PeerChannel {
		return nil, errors.New(
			"listing members is only supported for channels and supergroups",
		)
	}

	if limit <= 0 {
		limit = defaultLimit
	}

	result, err := w.api.ChannelsGetParticipants(
		ctx,
		&tg.ChannelsGetParticipantsRequest{
			Channel: InputChannelFromPeer(peer),
			Filter:  participantFilter(filter),
			Limit:   limit,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "getting group members")
	}

	return usersFromParticipants(result), nil
}

// GetContactStatuses returns online statuses of contacts.
func (w *Wrapper) GetContactStatuses(
	ctx context.Context,
) ([]ContactStatus, error) {
	result, err := w.api.ContactsGetStatuses(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting contact statuses")
	}

	return convertContactStatuses(result), nil
}

// PinDialog pins or unpins a dialog.
func (w *Wrapper) PinDialog(
	ctx context.Context, peer InputPeer, pinned bool,
) error {
	_, err := w.api.MessagesToggleDialogPin(
		ctx,
		&tg.MessagesToggleDialogPinRequest{
			Peer:   &tg.InputDialogPeer{Peer: InputPeerToTG(peer)},
			Pinned: pinned,
		},
	)

	return errors.Wrap(err, "toggling dialog pin")
}

// MarkDialogUnread marks a dialog as read or unread.
func (w *Wrapper) MarkDialogUnread(
	ctx context.Context, peer InputPeer, unread bool,
) error {
	_, err := w.api.MessagesMarkDialogUnread(
		ctx,
		&tg.MessagesMarkDialogUnreadRequest{
			Peer:   &tg.InputDialogPeer{Peer: InputPeerToTG(peer)},
			Unread: unread,
		},
	)

	return errors.Wrap(err, "marking dialog unread")
}

// SetSlowMode sets slowmode delay for a channel/supergroup.
func (w *Wrapper) SetSlowMode(
	ctx context.Context, peer InputPeer, seconds int,
) error {
	if peer.Type != PeerChannel {
		return errors.New(
			"slowmode is only supported for channels and supergroups",
		)
	}

	_, err := w.api.ChannelsToggleSlowMode(
		ctx,
		&tg.ChannelsToggleSlowModeRequest{
			Channel: InputChannelFromPeer(peer),
			Seconds: seconds,
		},
	)

	return errors.Wrap(err, "setting slow mode")
}

// CreateForumTopic creates a new forum topic.
func (w *Wrapper) CreateForumTopic(
	ctx context.Context, peer InputPeer, title string,
) (*ForumTopic, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	result, err := w.api.MessagesCreateForumTopic(
		ctx,
		&tg.MessagesCreateForumTopicRequest{
			Peer:     InputPeerToTG(peer),
			Title:    title,
			RandomID: randID,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "creating forum topic")
	}

	return topicFromUpdates(result), nil
}

// EditForumTopic edits a forum topic's title.
func (w *Wrapper) EditForumTopic(
	ctx context.Context,
	peer InputPeer,
	topicID int,
	title string,
) error {
	req := &tg.MessagesEditForumTopicRequest{
		Peer:    InputPeerToTG(peer),
		TopicID: topicID,
	}
	req.SetTitle(title)

	_, err := w.api.MessagesEditForumTopic(ctx, req)

	return errors.Wrap(err, "editing forum topic")
}

// AddContact adds a user to contacts.
func (w *Wrapper) AddContact(
	ctx context.Context,
	peer InputPeer,
	firstName, lastName, phone string,
) error {
	_, err := w.api.ContactsAddContact(
		ctx,
		&tg.ContactsAddContactRequest{
			ID:        InputUserFromPeer(peer),
			FirstName: firstName,
			LastName:  lastName,
			Phone:     phone,
		},
	)

	return errors.Wrap(err, "adding contact")
}

// DeleteContact removes a user from contacts.
func (w *Wrapper) DeleteContact(
	ctx context.Context, peer InputPeer,
) error {
	_, err := w.api.ContactsDeleteContacts(
		ctx,
		[]tg.InputUserClass{InputUserFromPeer(peer)},
	)

	return errors.Wrap(err, "deleting contact")
}

// SetAdmin sets admin rights for a user in a channel.
func (w *Wrapper) SetAdmin(
	ctx context.Context,
	group, user InputPeer,
	rights AdminRights,
	rank string,
) error {
	if group.Type != PeerChannel {
		return errors.New(
			"setting admins is only supported for channels and supergroups",
		)
	}

	_, err := w.api.ChannelsEditAdmin(
		ctx,
		&tg.ChannelsEditAdminRequest{
			Channel:     InputChannelFromPeer(group),
			UserID:      InputUserFromPeer(user),
			AdminRights: convertAdminRights(rights),
			Rank:        rank,
		},
	)

	return errors.Wrap(err, "setting admin rights")
}

// DeleteHistory deletes all messages in a chat.
// Only users and basic groups are supported; channels
// must use different API methods.
func (w *Wrapper) DeleteHistory(
	ctx context.Context, peer InputPeer, revoke bool,
) error {
	if peer.Type == PeerChannel {
		return errors.New(
			"delete history is only supported for users and basic groups",
		)
	}

	for {
		result, err := w.api.MessagesDeleteHistory(
			ctx,
			&tg.MessagesDeleteHistoryRequest{
				Peer:   InputPeerToTG(peer),
				MaxID:  math.MaxInt32,
				Revoke: revoke,
			},
		)
		if err != nil {
			return errors.Wrap(err, "deleting history")
		}

		if result.Offset == 0 {
			break
		}
	}

	return nil
}

// ClearAllDrafts clears all message drafts.
func (w *Wrapper) ClearAllDrafts(ctx context.Context) error {
	_, err := w.api.MessagesClearAllDrafts(ctx)

	return errors.Wrap(err, "clearing all drafts")
}

// resolveViaDialogs fetches peer details using GetPeerDialogs
// to obtain a valid access hash for numeric IDs.
func (w *Wrapper) resolveViaDialogs(
	ctx context.Context, peer InputPeer,
) (InputPeer, bool) {
	result, err := w.api.MessagesGetPeerDialogs(ctx, []tg.InputDialogPeerClass{
		&tg.InputDialogPeer{Peer: InputPeerToTG(peer)},
	})
	if err != nil {
		return InputPeer{}, false
	}

	w.cacheFromPeerDialogs(result)

	cached, hit := w.cache.Lookup(peer.Type, peer.ID)

	return cached, hit
}

func (w *Wrapper) cacheFromPeerDialogs(result *tg.MessagesPeerDialogs) {
	if result == nil {
		return
	}

	for _, usr := range result.Users {
		if typed, ok := usr.(*tg.User); ok {
			w.cache.Store(InputPeer{
				Type: PeerUser, ID: typed.ID, AccessHash: typed.AccessHash,
			})
		}
	}

	for _, chat := range result.Chats {
		if typed, ok := chat.(*tg.Channel); ok {
			w.cache.Store(InputPeer{
				Type: PeerChannel, ID: typed.ID, AccessHash: typed.AccessHash,
			})
		}
	}
}

// cacheDialogPeers stores all dialog peers with valid access hashes.
func (w *Wrapper) cacheDialogPeers(dialogs []Dialog) {
	peers := make([]InputPeer, 0, len(dialogs))

	for _, dlg := range dialogs {
		peers = append(peers, dlg.Peer)
	}

	w.cache.StoreAll(peers)
}

func (w *Wrapper) getRawMessage(ctx context.Context, peer InputPeer, msgID int) (*tg.Message, error) {
	inputIDs := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}

	var (
		result tg.MessagesMessagesClass
		err    error
	)

	if peer.Type == PeerChannel {
		result, err = w.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: InputChannelFromPeer(peer),
			ID:      inputIDs,
		})
	} else {
		result, err = w.api.MessagesGetMessages(ctx, inputIDs)
	}

	if err != nil {
		return nil, errors.Wrap(err, "fetching message")
	}

	return firstRawMessage(result)
}

func (w *Wrapper) findFolderByTitle(ctx context.Context, title string, peers []InputPeer) (*Folder, error) {
	folders, err := w.GetFolders(ctx)
	if err != nil {
		return &Folder{Title: title, Peers: peers}, nil //nolint:nilerr // best-effort: return without ID if listing fails.
	}

	for _, folder := range folders {
		if folder.Title == title {
			return &Folder{ID: folder.ID, Title: title, Peers: peers}, nil
		}
	}

	return &Folder{Title: title, Peers: peers}, nil
}
