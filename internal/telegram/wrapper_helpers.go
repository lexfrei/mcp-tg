package telegram

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

const (
	defaultLimit   = 100
	outputDirPerms = 0o750
	fallbackMIME   = "application/octet-stream"
	// messageLengthFastPath is the historical Telegram text-message length cap.
	// Messages within this many UTF-8 codepoints are accepted without
	// querying the live server config (hot path).
	messageLengthFastPath = 4096
	// captionLengthFastPath is the always-safe lower bound for media captions
	// (non-Premium default). Captions within this size never need a server
	// roundtrip; longer ones go through the live caption_length_max check
	// because Premium and non-Premium accounts have different caps.
	captionLengthFastPath = 1024
)

func uploadedFileID(file tg.InputFileClass) int64 {
	switch typed := file.(type) {
	case *tg.InputFile:
		return typed.ID
	case *tg.InputFileBig:
		return typed.ID
	default:
		return 0
	}
}

func isImagePath(path string) bool {
	mimeType := mimeByPath(path)

	return strings.HasPrefix(mimeType, "image/")
}

func buildMultiMediaRequest(
	peer InputPeer, media []tg.InputSingleMedia, opts SendOpts,
) *tg.MessagesSendMultiMediaRequest {
	req := &tg.MessagesSendMultiMediaRequest{
		Peer:       InputPeerToTG(peer),
		MultiMedia: media,
		Silent:     opts.Silent,
	}

	if reply := buildReplyTo(opts.TopicID, opts.ReplyTo); reply != nil {
		req.ReplyTo = reply
	}

	if opts.ScheduleDate > 0 {
		req.SetScheduleDate(opts.ScheduleDate)
	}

	return req
}

// buildReplyTo constructs an InputReplyToMessage from topic and reply IDs.
// Returns nil when neither topicID nor replyTo is set.
// When topicID is set without replyTo, sets ReplyToMsgID=topicID
// (Telegram requires reply_to_msg_id when top_msg_id is used).
func buildReplyTo(topicID, replyTo int) *tg.InputReplyToMessage {
	if topicID <= 0 && replyTo <= 0 {
		return nil
	}

	reply := &tg.InputReplyToMessage{}

	if replyTo > 0 {
		reply.ReplyToMsgID = replyTo
	} else if topicID > 0 {
		reply.ReplyToMsgID = topicID
	}

	if topicID > 0 {
		reply.SetTopMsgID(topicID)
	}

	return reply
}

// messageLengthResolver returns the server-side message_length_max cap.
// Called only when the text exceeds messageLengthFastPath, so it can be
// expensive (one MTProto round-trip on first call, cached afterwards).
type messageLengthResolver func(context.Context) (int, error)

// validateLengthAgainstServer is the shared core for message and caption
// length validation: skip the live config lookup when text fits the
// always-safe fast-path, otherwise compare against the server-reported cap.
// kind is used in the error message ("message" or "caption"). On resolver
// error returns nil so MTProto stays authoritative.
func validateLengthAgainstServer(
	ctx context.Context,
	text, kind string,
	fastPath int,
	resolveMax messageLengthResolver,
) error {
	codepoints := utf8.RuneCountInString(text)
	if codepoints <= fastPath {
		return nil
	}

	serverMax, err := resolveMax(ctx)
	if err != nil {
		// Resolver failure: skip the local check and let the actual send
		// call surface MESSAGE_TOO_LONG if applicable, instead of blocking
		// the send on a transient help.getConfig hiccup.
		return nil //nolint:nilerr // intentional fall-through to server-side validation
	}

	if codepoints > serverMax {
		return errors.Errorf(
			"%s length %d codepoints exceeds server limit of %d",
			kind, codepoints, serverMax,
		)
	}

	return nil
}

// validateMessageLength rejects empty text-message bodies and bodies that
// exceed the server-reported message_length_max. Counting follows the
// Telegram documentation ("length in utf8 codepoints").
func validateMessageLength(
	ctx context.Context, text string, resolveMax messageLengthResolver,
) error {
	if text == "" {
		return errors.New("message text cannot be empty")
	}

	return validateLengthAgainstServer(ctx, text, "message", messageLengthFastPath, resolveMax)
}

// validateCaptionLength checks media captions against the server-reported
// caption_length_max. An empty caption is accepted (captions are optional).
func validateCaptionLength(
	ctx context.Context, caption string, resolveMax messageLengthResolver,
) error {
	if caption == "" {
		return nil
	}

	return validateLengthAgainstServer(ctx, caption, "caption", captionLengthFastPath, resolveMax)
}

// mimeByPath guesses MIME type from file extension, falling back to octet-stream.
func mimeByPath(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return fallbackMIME
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		return fallbackMIME
	}

	return mimeType
}

func typingAction(action string) tg.SendMessageActionClass {
	switch action {
	case "recording_voice":
		return &tg.SendMessageRecordAudioAction{}
	case "uploading_photo":
		return &tg.SendMessageUploadPhotoAction{}
	case "uploading_document":
		return &tg.SendMessageUploadDocumentAction{}
	case "choosing_sticker":
		return &tg.SendMessageChooseStickerAction{}
	case "cancel":
		return &tg.SendMessageCancelAction{}
	default:
		return &tg.SendMessageTypingAction{}
	}
}

func convertPermissions(perms ChatPermissions) tg.ChatBannedRights {
	var rights tg.ChatBannedRights

	rights.SetSendMessages(!perms.SendMessages)
	rights.SetSendMedia(!perms.SendMedia)
	rights.SetSendStickers(!perms.SendStickers)
	rights.SetSendGifs(!perms.SendGifs)
	rights.SetSendPolls(!perms.SendPolls)
	rights.SetInviteUsers(!perms.AddMembers)
	rights.SetPinMessages(!perms.PinMessages)
	rights.SetChangeInfo(!perms.ChangeInfo)

	return rights
}

func (w *Wrapper) getUserPeerInfo(ctx context.Context, peer InputPeer) (*PeerInfo, error) {
	full, err := w.api.UsersGetFullUser(ctx, InputUserFromPeer(peer))
	if err != nil {
		return nil, errors.Wrap(err, "getting user info")
	}

	for _, usr := range full.Users {
		if u, ok := usr.(*tg.User); ok && u.ID == peer.ID {
			return &PeerInfo{
				Peer:     peer,
				Title:    u.FirstName + " " + u.LastName,
				Username: u.Username,
				About:    full.FullUser.About,
				Type:     "user",
			}, nil
		}
	}

	return nil, errors.New("user not found")
}

func (w *Wrapper) getChatPeerInfo(ctx context.Context, peer InputPeer) (*PeerInfo, error) {
	full, err := w.api.MessagesGetFullChat(ctx, peer.ID)
	if err != nil {
		return nil, errors.Wrap(err, "getting chat info")
	}

	for _, ch := range full.Chats {
		if c, ok := ch.(*tg.Chat); ok && c.ID == peer.ID {
			about := ""

			if chatFull, ok := full.FullChat.(*tg.ChatFull); ok {
				about = chatFull.About
			}

			return &PeerInfo{
				Peer:  peer,
				Title: c.Title,
				About: about,
				Type:  "chat",
			}, nil
		}
	}

	return nil, errors.New("chat not found")
}

func (w *Wrapper) getChannelPeerInfo(ctx context.Context, peer InputPeer) (*PeerInfo, error) {
	full, err := w.api.ChannelsGetFullChannel(ctx, InputChannelFromPeer(peer))
	if err != nil {
		return nil, errors.Wrap(err, "getting channel info")
	}

	for _, ch := range full.Chats {
		if c, ok := ch.(*tg.Channel); ok && c.ID == peer.ID {
			about := ""

			if channelFull, ok := full.FullChat.(*tg.ChannelFull); ok {
				about = channelFull.About
			}

			return &PeerInfo{
				Peer:     peer,
				Title:    c.Title,
				Username: c.Username,
				About:    about,
				Type:     channelType(c),
			}, nil
		}
	}

	return nil, errors.New("channel not found")
}

func channelType(ch *tg.Channel) string {
	if ch.Megagroup {
		return "supergroup"
	}

	return "channel"
}

func (w *Wrapper) getChannelGroupInfo(ctx context.Context, peer InputPeer) (*GroupInfo, error) {
	full, err := w.api.ChannelsGetFullChannel(ctx, InputChannelFromPeer(peer))
	if err != nil {
		return nil, errors.Wrap(err, "getting channel group info")
	}

	for _, ch := range full.Chats {
		if c, ok := ch.(*tg.Channel); ok && c.ID == peer.ID {
			info := &GroupInfo{
				Peer:         peer,
				Title:        c.Title,
				Username:     c.Username,
				IsChannel:    !c.Megagroup,
				IsSupergroup: c.Megagroup,
				IsForum:      c.Forum,
			}

			if channelFull, ok := full.FullChat.(*tg.ChannelFull); ok {
				info.About = channelFull.About
				info.MembersCount = channelFull.ParticipantsCount
			}

			return info, nil
		}
	}

	return nil, errors.New("channel not found")
}

func (w *Wrapper) getChatGroupInfo(ctx context.Context, peer InputPeer) (*GroupInfo, error) {
	full, err := w.api.MessagesGetFullChat(ctx, peer.ID)
	if err != nil {
		return nil, errors.Wrap(err, "getting chat group info")
	}

	for _, ch := range full.Chats {
		if c, ok := ch.(*tg.Chat); ok && c.ID == peer.ID {
			info := &GroupInfo{
				Peer:         peer,
				Title:        c.Title,
				MembersCount: c.ParticipantsCount,
			}

			if chatFull, ok := full.FullChat.(*tg.ChatFull); ok {
				info.About = chatFull.About
			}

			return info, nil
		}
	}

	return nil, errors.New("chat not found")
}

func (w *Wrapper) createChannel(ctx context.Context, title string) (*PeerInfo, error) {
	result, err := w.api.ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
		Title: title,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating channel")
	}

	info := firstChatFromUpdates(result)
	if info == nil {
		return nil, errors.New("channel created but could not extract info from response")
	}

	return info, nil
}

func (w *Wrapper) createBasicChat(ctx context.Context, title string, users []InputPeer) (*PeerInfo, error) {
	tgUsers := make([]tg.InputUserClass, len(users))
	for idx, usr := range users {
		tgUsers[idx] = InputUserFromPeer(usr)
	}

	result, err := w.api.MessagesCreateChat(ctx, &tg.MessagesCreateChatRequest{
		Title: title,
		Users: tgUsers,
	})
	if err != nil {
		return nil, errors.Wrap(err, "creating chat")
	}

	info := firstChatFromUpdates(result.Updates)
	if info == nil {
		return nil, errors.New("chat created but could not extract info from response")
	}

	return info, nil
}

func extractDialogs(result tg.MessagesDialogsClass) []Dialog {
	switch res := result.(type) {
	case *tg.MessagesDialogs:
		return buildDialogs(res.Dialogs, res.Chats, res.Users)
	case *tg.MessagesDialogsSlice:
		return buildDialogs(res.Dialogs, res.Chats, res.Users)
	default:
		return nil
	}
}

func buildDialogs(dialogs []tg.DialogClass, chats []tg.ChatClass, users []tg.UserClass) []Dialog {
	result := make([]Dialog, 0, len(dialogs))

	chatMap := buildChatMap(chats)
	userMap := buildUserMap(users)

	for _, dlg := range dialogs {
		dialog, ok := dlg.(*tg.Dialog)
		if !ok {
			continue
		}

		if dialog.Peer == nil {
			continue
		}

		entry := Dialog{UnreadCount: dialog.UnreadCount}
		fillDialogPeer(&entry, dialog.Peer, chatMap, userMap)

		result = append(result, entry)
	}

	return result
}

func buildChatMap(chats []tg.ChatClass) map[int64]tg.ChatClass {
	chatMap := make(map[int64]tg.ChatClass, len(chats))

	for _, ch := range chats {
		switch c := ch.(type) {
		case *tg.Chat:
			chatMap[c.ID] = c
		case *tg.Channel:
			chatMap[c.ID] = c
		}
	}

	return chatMap
}

func buildUserMap(users []tg.UserClass) map[int64]*tg.User {
	userMap := make(map[int64]*tg.User, len(users))

	for _, usr := range users {
		if u, ok := usr.(*tg.User); ok {
			userMap[u.ID] = u
		}
	}

	return userMap
}

func fillDialogPeer(entry *Dialog, peer tg.PeerClass, chatMap map[int64]tg.ChatClass, userMap map[int64]*tg.User) {
	switch peerTyped := peer.(type) {
	case *tg.PeerUser:
		entry.Peer = InputPeer{Type: PeerUser, ID: peerTyped.UserID}

		if usr, ok := userMap[peerTyped.UserID]; ok {
			entry.Peer.AccessHash = usr.AccessHash
			entry.Title = fmt.Sprintf("%s %s", usr.FirstName, usr.LastName)
			entry.Username = usr.Username
		}
	case *tg.PeerChat:
		entry.Peer = InputPeer{Type: PeerChat, ID: peerTyped.ChatID}
		entry.IsGroup = true

		if raw, ok := chatMap[peerTyped.ChatID]; ok {
			if chat, ok := raw.(*tg.Chat); ok {
				entry.Title = chat.Title
			}
		}
	case *tg.PeerChannel:
		fillChannelDialog(entry, peerTyped, chatMap)
	}
}

func fillChannelDialog(entry *Dialog, peer *tg.PeerChannel, chatMap map[int64]tg.ChatClass) {
	raw, ok := chatMap[peer.ChannelID]
	if !ok {
		return
	}

	channel, ok := raw.(*tg.Channel)
	if !ok {
		return
	}

	entry.Peer = InputPeer{
		Type:       PeerChannel,
		ID:         channel.ID,
		AccessHash: channel.AccessHash,
	}
	entry.Title = channel.Title
	entry.Username = channel.Username
	entry.IsChannel = !channel.Megagroup
	entry.IsGroup = channel.Megagroup
}

func dialogsFromSearch(result *tg.ContactsFound) []Dialog {
	dialogs := make([]Dialog, 0, len(result.Chats)+len(result.Users))

	for _, ch := range result.Chats {
		if c, ok := ch.(*tg.Channel); ok {
			dialogs = append(dialogs, Dialog{
				Peer: InputPeer{
					Type:       PeerChannel,
					ID:         c.ID,
					AccessHash: c.AccessHash,
				},
				Title:     c.Title,
				Username:  c.Username,
				IsChannel: !c.Megagroup,
				IsGroup:   c.Megagroup,
			})
		}
	}

	for _, rawUser := range result.Users {
		if user, ok := rawUser.(*tg.User); ok {
			dialogs = append(dialogs, Dialog{
				Peer: InputPeer{
					Type:       PeerUser,
					ID:         user.ID,
					AccessHash: user.AccessHash,
				},
				Title:    fmt.Sprintf("%s %s", user.FirstName, user.LastName),
				Username: user.Username,
			})
		}
	}

	return dialogs
}

//nolint:gocritic // unnamedResult: names add no clarity for extraction.
func extractMessages(
	result tg.MessagesMessagesClass, peerID int64,
) ([]Message, int) {
	switch res := result.(type) {
	case *tg.MessagesMessages:
		names := buildUserNames(res.Users)

		return convertMessages(res.Messages, names, peerID), len(res.Messages)
	case *tg.MessagesMessagesSlice:
		names := buildUserNames(res.Users)

		return convertMessages(res.Messages, names, peerID), res.Count
	case *tg.MessagesChannelMessages:
		names := buildUserNames(res.Users)

		return convertMessages(res.Messages, names, peerID), res.Count
	default:
		return nil, 0
	}
}

func buildUserNames(users []tg.UserClass) map[int64]string {
	names := make(map[int64]string, len(users))

	for _, usr := range users {
		if typed, ok := usr.(*tg.User); ok {
			names[typed.ID] = userDisplayName(typed)
		}
	}

	return names
}

func userDisplayName(usr *tg.User) string {
	name := strings.TrimSpace(usr.FirstName + " " + usr.LastName)
	if name == "" {
		return usr.Username
	}

	return name
}

func convertMessages(
	raw []tg.MessageClass, names map[int64]string, peerID int64,
) []Message {
	msgs := make([]Message, 0, len(raw))

	for _, m := range raw {
		if msg, ok := m.(*tg.Message); ok {
			converted := ConvertMessage(msg)
			fillFromName(&converted, names, peerID)
			msgs = append(msgs, converted)
		}
	}

	return msgs
}

// fillFromName resolves sender name from user map.
// In DMs, FromID==0 means the peer (not owner).
func fillFromName(msg *Message, names map[int64]string, peerID int64) {
	if msg.FromID != 0 {
		msg.FromName = names[msg.FromID]

		return
	}

	if name, ok := names[peerID]; ok {
		msg.FromID = peerID
		msg.FromName = name
	}
}

func messagesFromUpdates(result tg.UpdatesClass) []Message {
	if result == nil {
		return nil
	}

	upd, ok := result.(*tg.Updates)
	if !ok {
		return nil
	}

	names := buildUserNames(upd.Users)

	var msgs []Message

	for _, update := range upd.Updates {
		if newMsg, ok := update.(*tg.UpdateNewMessage); ok {
			if msg, ok := newMsg.Message.(*tg.Message); ok {
				converted := ConvertMessage(msg)
				converted.FromName = names[converted.FromID]
				msgs = append(msgs, converted)
			}
		}

		if newMsg, ok := update.(*tg.UpdateNewChannelMessage); ok {
			if msg, ok := newMsg.Message.(*tg.Message); ok {
				converted := ConvertMessage(msg)
				converted.FromName = names[converted.FromID]
				msgs = append(msgs, converted)
			}
		}
	}

	return msgs
}

func messageFromUpdate(result tg.UpdatesClass) *Message {
	if result == nil {
		return nil
	}

	switch upd := result.(type) {
	case *tg.UpdateShortSentMessage:
		return &Message{
			ID:   upd.ID,
			Date: upd.Date,
		}
	case *tg.Updates:
		return firstMessageFromUpdates(upd.Updates)
	}

	return nil
}

func firstMessageFromUpdates(updates []tg.UpdateClass) *Message {
	for _, update := range updates {
		if msg := extractMessageFromUpdate(update); msg != nil {
			return msg
		}
	}

	return nil
}

func extractMessageFromUpdate(update tg.UpdateClass) *Message {
	switch upd := update.(type) {
	case *tg.UpdateNewMessage:
		if msg, ok := upd.Message.(*tg.Message); ok {
			converted := ConvertMessage(msg)

			return &converted
		}
	case *tg.UpdateNewChannelMessage:
		if msg, ok := upd.Message.(*tg.Message); ok {
			converted := ConvertMessage(msg)

			return &converted
		}
	case *tg.UpdateNewScheduledMessage:
		if msg, ok := upd.Message.(*tg.Message); ok {
			converted := ConvertMessage(msg)

			return &converted
		}
	}

	return nil
}

func extractPhotos(result tg.PhotosPhotosClass) []Photo {
	var rawPhotos []tg.PhotoClass

	switch res := result.(type) {
	case *tg.PhotosPhotos:
		rawPhotos = res.Photos
	case *tg.PhotosPhotosSlice:
		rawPhotos = res.Photos
	default:
		return nil
	}

	photos := make([]Photo, 0, len(rawPhotos))

	for _, p := range rawPhotos {
		if photo, ok := p.(*tg.Photo); ok {
			photos = append(photos, Photo{
				ID:   photo.ID,
				Date: photo.Date,
			})
		}
	}

	return photos
}

func extractForumTopics(result *tg.MessagesForumTopics) []ForumTopic {
	if result == nil {
		return nil
	}

	topics := make([]ForumTopic, 0, len(result.Topics))

	for _, t := range result.Topics {
		if topic, ok := t.(*tg.ForumTopic); ok {
			topics = append(topics, ForumTopic{
				ID:    topic.ID,
				Title: topic.Title,
				Date:  topic.Date,
			})
		}
	}

	return topics
}

func extractStickerSets(result tg.MessagesFoundStickerSetsClass) []StickerSet {
	found, ok := result.(*tg.MessagesFoundStickerSets)
	if !ok {
		return nil
	}

	sets := make([]StickerSet, 0, len(found.Sets))

	for _, s := range found.Sets {
		if set, ok := s.(*tg.StickerSetCovered); ok {
			sets = append(sets, StickerSet{
				ID:    set.Set.ID,
				Title: set.Set.Title,
				Name:  set.Set.ShortName,
				Count: set.Set.Count,
			})
		}
	}

	return sets
}

func convertStickerSetFull(result *tg.MessagesStickerSet) *StickerSetFull {
	if result == nil {
		return nil
	}

	full := &StickerSetFull{
		StickerSet: StickerSet{
			ID:    result.Set.ID,
			Title: result.Set.Title,
			Name:  result.Set.ShortName,
			Count: result.Set.Count,
		},
	}

	for _, doc := range result.Documents {
		if document, ok := doc.(*tg.Document); ok {
			sticker := Sticker{
				ID:         document.ID,
				FileID:     document.ID,
				AccessHash: document.AccessHash,
			}

			for _, attr := range document.Attributes {
				if s, ok := attr.(*tg.DocumentAttributeSticker); ok {
					sticker.Emoji = s.Alt
				}
			}

			full.Stickers = append(full.Stickers, sticker)
		}
	}

	return full
}

func usersFromParticipants(result tg.ChannelsChannelParticipantsClass) []User {
	participants, ok := result.(*tg.ChannelsChannelParticipants)
	if !ok {
		return nil
	}

	users := make([]User, 0, len(participants.Users))

	for _, usr := range participants.Users {
		if u, ok := usr.(*tg.User); ok {
			users = append(users, ConvertUser(u))
		}
	}

	return users
}

func peerInfosFromChats(result tg.MessagesChatsClass) []PeerInfo {
	var rawChats []tg.ChatClass

	switch res := result.(type) {
	case *tg.MessagesChats:
		rawChats = res.Chats
	case *tg.MessagesChatsSlice:
		rawChats = res.Chats
	default:
		return nil
	}

	infos := make([]PeerInfo, 0, len(rawChats))

	for _, ch := range rawChats {
		switch c := ch.(type) {
		case *tg.Chat:
			infos = append(infos, PeerInfo{
				Peer:  InputPeer{Type: PeerChat, ID: c.ID},
				Title: c.Title,
				Type:  "chat",
			})
		case *tg.Channel:
			infos = append(infos, PeerInfo{
				Peer: InputPeer{
					Type:       PeerChannel,
					ID:         c.ID,
					AccessHash: c.AccessHash,
				},
				Title:    c.Title,
				Username: c.Username,
				Type:     channelType(c),
			})
		}
	}

	return infos
}

func firstChatFromUpdates(result tg.UpdatesClass) *PeerInfo {
	upd, ok := result.(*tg.Updates)
	if !ok {
		return nil
	}

	for _, ch := range upd.Chats {
		switch c := ch.(type) {
		case *tg.Chat:
			return &PeerInfo{
				Peer:  InputPeer{Type: PeerChat, ID: c.ID},
				Title: c.Title,
				Type:  "chat",
			}
		case *tg.Channel:
			return &PeerInfo{
				Peer: InputPeer{
					Type:       PeerChannel,
					ID:         c.ID,
					AccessHash: c.AccessHash,
				},
				Title:    c.Title,
				Username: c.Username,
				Type:     channelType(c),
			}
		}
	}

	return nil
}

func extractFolders(result *tg.MessagesDialogFilters) []Folder {
	if result == nil {
		return nil
	}

	folders := make([]Folder, 0, len(result.Filters))

	for _, f := range result.Filters {
		if filter, ok := f.(*tg.DialogFilter); ok {
			folders = append(folders, Folder{
				ID:    filter.ID,
				Title: filter.Title.Text,
			})
		}
	}

	return folders
}

func firstRawMessage(result tg.MessagesMessagesClass) (*tg.Message, error) {
	var rawMsgs []tg.MessageClass

	switch res := result.(type) {
	case *tg.MessagesMessages:
		rawMsgs = res.Messages
	case *tg.MessagesMessagesSlice:
		rawMsgs = res.Messages
	case *tg.MessagesChannelMessages:
		rawMsgs = res.Messages
	}

	for _, raw := range rawMsgs {
		if msg, ok := raw.(*tg.Message); ok {
			return msg, nil
		}
	}

	return nil, errors.New("message not found")
}

//nolint:gocritic // unnamedResult: names add no clarity for extraction functions.
func extractMediaLocation(msg *tg.Message) (tg.InputFileLocationClass, string) {
	if msg == nil || msg.Media == nil {
		return nil, ""
	}

	switch media := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		return extractDocumentLocation(media)
	case *tg.MessageMediaPhoto:
		return extractPhotoLocation(media)
	default:
		return nil, ""
	}
}

//nolint:gocritic // unnamedResult: names add no clarity for extraction functions.
func extractDocumentLocation(media *tg.MessageMediaDocument) (tg.InputFileLocationClass, string) {
	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return nil, ""
	}

	fileName := documentFileName(doc)

	return &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
	}, fileName
}

//nolint:gocritic // unnamedResult: names add no clarity for extraction functions.
func extractPhotoLocation(media *tg.MessageMediaPhoto) (tg.InputFileLocationClass, string) {
	photo, ok := media.Photo.(*tg.Photo)
	if !ok || len(photo.Sizes) == 0 {
		return nil, ""
	}

	return &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     largestPhotoSize(photo.Sizes),
	}, fmt.Sprintf("photo_%d.jpg", photo.ID)
}

func documentFileName(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if fname, ok := attr.(*tg.DocumentAttributeFilename); ok {
			return fname.FileName
		}
	}

	return fmt.Sprintf("document_%d", doc.ID)
}

func largestPhotoSize(sizes []tg.PhotoSizeClass) string {
	best := ""

	for _, size := range sizes {
		switch typed := size.(type) {
		case *tg.PhotoSize:
			best = typed.Type
		case *tg.PhotoSizeProgressive:
			best = typed.Type
		}
	}

	if best == "" {
		best = "x"
	}

	return best
}

func extractBlockedUsers(
	result tg.ContactsBlockedClass,
) []User {
	var rawUsers []tg.UserClass

	switch res := result.(type) {
	case *tg.ContactsBlocked:
		rawUsers = res.Users
	case *tg.ContactsBlockedSlice:
		rawUsers = res.Users
	}

	users := make([]User, 0, len(rawUsers))

	for _, usr := range rawUsers {
		if typed, ok := usr.(*tg.User); ok {
			users = append(users, ConvertUser(typed))
		}
	}

	return users
}

func extractReactionUsers(
	result *tg.MessagesMessageReactionsList,
) []ReactionUser {
	if result == nil {
		return nil
	}

	names := buildUserMap(result.Users)
	items := make([]ReactionUser, 0, len(result.Reactions))

	for idx := range result.Reactions {
		reaction := &result.Reactions[idx]
		item := ReactionUser{
			UserID: extractFromID(reaction.PeerID),
			Emoji:  reactionEmoji(reaction.Reaction),
		}

		if usr, ok := names[item.UserID]; ok {
			item.UserName = userDisplayName(usr)
		}

		items = append(items, item)
	}

	return items
}

func reactionEmoji(reaction tg.ReactionClass) string {
	switch typed := reaction.(type) {
	case *tg.ReactionEmoji:
		return typed.Emoticon
	case *tg.ReactionCustomEmoji:
		return fmt.Sprintf("custom:%d", typed.DocumentID)
	default:
		return ""
	}
}

func participantFilter(
	filter string,
) tg.ChannelParticipantsFilterClass {
	switch filter {
	case "admins":
		return &tg.ChannelParticipantsAdmins{}
	case "banned":
		return &tg.ChannelParticipantsBanned{}
	case "bots":
		return &tg.ChannelParticipantsBots{}
	case "kicked":
		return &tg.ChannelParticipantsKicked{}
	case "", "recent":
		return &tg.ChannelParticipantsRecent{}
	default:
		return &tg.ChannelParticipantsSearch{Q: filter}
	}
}

func convertContactStatuses(
	statuses []tg.ContactStatus,
) []ContactStatus {
	result := make([]ContactStatus, len(statuses))

	for idx := range statuses {
		result[idx] = ContactStatus{
			UserID:   statuses[idx].UserID,
			Status:   userStatusString(statuses[idx].Status),
			LastSeen: userStatusLastSeen(statuses[idx].Status),
		}
	}

	return result
}

func userStatusString(status tg.UserStatusClass) string {
	switch status.(type) {
	case *tg.UserStatusOnline:
		return "online"
	case *tg.UserStatusOffline:
		return "offline"
	case *tg.UserStatusRecently:
		return "recently"
	case *tg.UserStatusLastWeek:
		return "last_week"
	case *tg.UserStatusLastMonth:
		return "last_month"
	default:
		return "unknown"
	}
}

func userStatusLastSeen(status tg.UserStatusClass) int {
	if offline, ok := status.(*tg.UserStatusOffline); ok {
		return offline.WasOnline
	}

	return 0
}

func convertAdminRights(rights AdminRights) tg.ChatAdminRights {
	return tg.ChatAdminRights{
		ChangeInfo:     rights.ChangeInfo,
		PostMessages:   rights.PostMessages,
		EditMessages:   rights.EditMessages,
		DeleteMessages: rights.DeleteMsgs,
		BanUsers:       rights.BanUsers,
		InviteUsers:    rights.InviteUsers,
		PinMessages:    rights.PinMessages,
		ManageCall:     rights.ManageCall,
		AddAdmins:      rights.AddAdmins,
		ManageTopics:   rights.ManageTopics,
	}
}

func topicFromUpdates(result tg.UpdatesClass) *ForumTopic {
	upd, ok := result.(*tg.Updates)
	if !ok {
		return nil
	}

	for _, update := range upd.Updates {
		if ft, ok := update.(*tg.UpdateNewChannelMessage); ok {
			if msg, ok := ft.Message.(*tg.MessageService); ok {
				return &ForumTopic{
					ID:    msg.ID,
					Title: topicTitleFromAction(msg.Action),
					Date:  msg.Date,
				}
			}
		}
	}

	return nil
}

func topicTitleFromAction(
	action tg.MessageActionClass,
) string {
	if act, ok := action.(*tg.MessageActionTopicCreate); ok {
		return act.Title
	}

	return ""
}
