package telegram

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// PeerInfo Type labels exposed in MCP JSON output. Distinct from the
// internal PeerType enum — these are user-facing wire strings.
const (
	PeerInfoTypeUser       = "user"
	PeerInfoTypeChat       = "chat"
	PeerInfoTypeChannel    = "channel"
	PeerInfoTypeSupergroup = "supergroup"
)

// DefaultLimit is the page size used by GetDialogs / GetHistory /
// SearchMessages when the caller does not pass an explicit Limit.
// Exported so the tools layer can compute hasMore without duplicating
// the constant.
const DefaultLimit = 100

const (
	defaultLimit   = DefaultLimit
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

	return strings.HasPrefix(mimeType, imageMIMEPrefix)
}

const (
	imageMIMEPrefix = "image/"
	videoMIMEPrefix = "video/"
)

// albumIsVisual reports whether every path is visual media (image or video).
// A visual album renders as a media grid; if any item is non-visual the album
// falls back to uniform documents, since Telegram rejects grouping photos with
// arbitrary documents in a single media group.
func albumIsVisual(paths []string) bool {
	if len(paths) == 0 {
		return false
	}

	for _, path := range paths {
		mimeType := mimeByPath(path)
		if !strings.HasPrefix(mimeType, imageMIMEPrefix) && !strings.HasPrefix(mimeType, videoMIMEPrefix) {
			return false
		}
	}

	return true
}

// uploadedAlbumMedia wraps a freshly-uploaded file into the InputMedia passed to
// messages.uploadMedia. In a visual album images become photos and videos become
// streamable documents; otherwise every item is a plain document.
func uploadedAlbumMedia(file tg.InputFileClass, path string, visual bool) tg.InputMediaClass {
	mimeType := mimeByPath(path)

	if visual && strings.HasPrefix(mimeType, imageMIMEPrefix) {
		return &tg.InputMediaUploadedPhoto{File: file}
	}

	attributes := []tg.DocumentAttributeClass{
		&tg.DocumentAttributeFilename{FileName: filepath.Base(path)},
	}

	if visual && strings.HasPrefix(mimeType, videoMIMEPrefix) {
		attributes = append(attributes, &tg.DocumentAttributeVideo{SupportsStreaming: true})
	}

	return &tg.InputMediaUploadedDocument{
		File:       file,
		MimeType:   mimeType,
		Attributes: attributes,
	}
}

// inputMediaFromUploaded converts the result of messages.uploadMedia into the
// referenced InputMedia required by messages.sendMultiMedia. Freshly-uploaded
// media cannot be used directly in an album (MEDIA_INVALID); it must first be
// finalized through uploadMedia, then re-wrapped here.
func inputMediaFromUploaded(media tg.MessageMediaClass) (tg.InputMediaClass, error) {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := m.GetPhoto()
		if !ok {
			return nil, errors.New("uploadMedia returned photo media without a photo")
		}

		notEmpty, ok := photo.AsNotEmpty()
		if !ok {
			return nil, errors.New("uploadMedia returned an empty photo")
		}

		return &tg.InputMediaPhoto{ID: notEmpty.AsInput()}, nil
	case *tg.MessageMediaDocument:
		doc, ok := m.GetDocument()
		if !ok {
			return nil, errors.New("uploadMedia returned document media without a document")
		}

		notEmpty, ok := doc.AsNotEmpty()
		if !ok {
			return nil, errors.New("uploadMedia returned an empty document")
		}

		return &tg.InputMediaDocument{ID: notEmpty.AsInput()}, nil
	default:
		return nil, errors.Errorf("unexpected media type %T from uploadMedia", media)
	}
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

	applySendAs(opts.SendAs, req.SetSendAs)

	return req
}

// applySendAs sets the conditional send_as field on an outgoing request
// when an identity was requested. Every gotd request that carries the
// field exposes the same SetSendAs method value, so passing the method
// keeps this one helper usable from sendMessage, sendMedia,
// sendMultiMedia, forwardMessages and createForumTopic alike.
//
// A nil identity leaves the flag bit clear, which hands the choice to the
// server: it posts under the chat's saved default, which is the account
// itself until SetDefaultSendAs says otherwise. Verified against a live
// account — an omitted send_as in a chat whose default is a channel
// posts as that channel, exactly as the official clients do.
func applySendAs(sendAs *InputPeer, set func(tg.InputPeerClass)) {
	if sendAs == nil {
		return
	}

	set(InputPeerToTG(*sendAs))
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
				Type:  PeerInfoTypeChat,
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
				info.DefaultSendAs = defaultSendAsFrom(channelFull, full.Chats, full.Users)
			}

			// The reply carries access hashes for every peer it mentions,
			// including the default send-as identity we just rendered as a
			// numeric peer string. Without remembering them, handing that
			// string back as sendAs resolves to access hash 0.
			w.cachePeersOf(full.Chats, full.Users)

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

// peerRef is the internal name+username pair used to enrich messages
// with display names and usernames resolved from the MTProto response's
// Users/Chats arrays.
type peerRef struct {
	Name     string
	Username string
}

//nolint:gocritic // unnamedResult: names add no clarity for extraction.
func extractMessages(
	result tg.MessagesMessagesClass, peer InputPeer,
) ([]Message, int) {
	switch res := result.(type) {
	case *tg.MessagesMessages:
		users := buildUserRefs(res.Users)
		chats := buildChatRefs(res.Chats)

		return convertMessages(res.Messages, users, chats, peer), len(res.Messages)
	case *tg.MessagesMessagesSlice:
		users := buildUserRefs(res.Users)
		chats := buildChatRefs(res.Chats)

		return convertMessages(res.Messages, users, chats, peer), res.Count
	case *tg.MessagesChannelMessages:
		users := buildUserRefs(res.Users)
		chats := buildChatRefs(res.Chats)

		return convertMessages(res.Messages, users, chats, peer), res.Count
	default:
		return nil, 0
	}
}

func buildUserRefs(users []tg.UserClass) map[int64]peerRef {
	refs := make(map[int64]peerRef, len(users))

	for _, usr := range users {
		if typed, ok := usr.(*tg.User); ok {
			refs[typed.ID] = peerRef{
				Name:     userDisplayName(typed),
				Username: typed.Username,
			}
		}
	}

	return refs
}

func buildChatRefs(chats []tg.ChatClass) map[int64]peerRef {
	refs := make(map[int64]peerRef, len(chats))

	for _, ch := range chats {
		switch typed := ch.(type) {
		case *tg.Channel:
			refs[typed.ID] = peerRef{Name: typed.Title, Username: typed.Username}
		case *tg.Chat:
			refs[typed.ID] = peerRef{Name: typed.Title}
		}
	}

	return refs
}

func userDisplayName(usr *tg.User) string {
	name := strings.TrimSpace(usr.FirstName + " " + usr.LastName)
	if name == "" {
		return usr.Username
	}

	return name
}

func convertMessages(
	raw []tg.MessageClass, users, chats map[int64]peerRef, peer InputPeer,
) []Message {
	msgs := make([]Message, 0, len(raw))

	for _, m := range raw {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}

		converted := ConvertMessage(msg)
		fillSenderRef(&converted, users, chats, peer)
		fillForwardRefs(&converted, users, chats)
		fillReplyToRef(&converted, users, chats)
		msgs = append(msgs, converted)
	}

	return msgs
}

// fillSenderRef resolves sender name and username from user/chat maps.
// In DMs and anonymous channel posts, FromID==0 means the peer (not
// owner) is the sender, so we fall back to the host peer for both ID
// and name — AND copy the peer's kind into msg.FromType so a channel
// host renders as channel:N, not user:N.
//
// Empty fields in the resolved peerRef do NOT overwrite any value
// already populated on msg. applyPeerRef is shared with the merge
// paths in buildSenderLookup and participantsFromMessages, where
// preserving a non-empty earlier value over a later empty one is the
// load-bearing rule for last-non-empty-wins semantics; keeping the
// same shape here means a future buildUserRefs that emits an empty
// name through a new UserClass variant can't silently wipe a preset.
func fillSenderRef(msg *Message, users, chats map[int64]peerRef, peer InputPeer) {
	if msg.FromID != 0 {
		fromPeer := InputPeer{Type: msg.FromType, ID: msg.FromID}
		if ref, ok := lookupRefByPeer(fromPeer, users, chats); ok {
			applyPeerRef(&msg.FromName, &msg.FromUsername, ref)
		}

		return
	}

	if peer.ID == 0 {
		return
	}

	if ref, ok := lookupRefByPeer(peer, users, chats); ok {
		msg.FromID = peer.ID
		msg.FromType = peer.Type
		applyPeerRef(&msg.FromName, &msg.FromUsername, ref)
	}
}

func applyPeerRef(name, username *string, ref peerRef) {
	if ref.Name != "" {
		*name = ref.Name
	}

	if ref.Username != "" {
		*username = ref.Username
	}
}

// fillForwardRefs resolves PeerRef.Name and PeerRef.Username on
// msg.Forward.From using the response's Users/Chats arrays. Privacy-
// hidden forwards (From == nil, only FromName set) are passed through
// unchanged. Empty fields in the resolved peerRef do not overwrite a
// value already populated by ConvertMessage.
func fillForwardRefs(msg *Message, users, chats map[int64]peerRef) {
	if msg.Forward == nil || msg.Forward.From == nil {
		return
	}

	ref, ok := lookupRefByPeer(msg.Forward.From.Peer, users, chats)
	if !ok {
		return
	}

	applyPeerRef(&msg.Forward.From.Name, &msg.Forward.From.Username, ref)
}

// fillReplyToRef resolves FromName/FromUsername on the reply parent
// when ReplyTo.FromPeerID is present. The Telegram client populates
// FromPeerID for cross-chat quote-replies and may also populate it for
// in-chat quote-replies; either way the resolved name lands in the
// advisory FromName/FromUsername slots. Same-chat replies without a
// FromPeerID skip resolution because the parent author is reachable
// via msg.FromID at the call site that needs it. Empty resolved
// fields do not overwrite values already populated upstream.
func fillReplyToRef(msg *Message, users, chats map[int64]peerRef) {
	if msg.ReplyTo == nil || msg.ReplyTo.FromPeerID == nil {
		return
	}

	ref, ok := lookupRefByPeer(*msg.ReplyTo.FromPeerID, users, chats)
	if !ok {
		return
	}

	applyPeerRef(&msg.ReplyTo.FromName, &msg.ReplyTo.FromUsername, ref)
}

// lookupRefByPeer routes to the correct map based on the peer kind.
// User IDs and channel IDs technically share an int64 namespace and
// can collide, so a type-blind union lookup would let a same-ID user
// stamp its name onto a channel-shaped PeerRef (and vice versa).
// Knowing the kind at the call site lets us go straight to the right
// half. PeerChat shares the chats map with PeerChannel (gotd packs
// basic groups, supergroups, and broadcast channels all into Chats[]
// — same numeric ID across these three is vanishingly rare because
// gotd populates them from disjoint MTProto constructors, but if it
// happened the {Type, ID} dedup in participantsFromMessages would
// still keep them distinct in the JSON).
//
// The default branch returns (peerRef{}, false), so a future PeerType
// extension would silently lose name resolution for that kind. If you
// add a new PeerType, extend the switch here too — the missing branch
// is otherwise discoverable only by noticing names stop populating.
func lookupRefByPeer(peer InputPeer, users, chats map[int64]peerRef) (peerRef, bool) {
	switch peer.Type {
	case PeerUser:
		ref, ok := users[peer.ID]

		return ref, ok
	case PeerChat, PeerChannel:
		ref, ok := chats[peer.ID]

		return ref, ok
	default:
		return peerRef{}, false
	}
}

// shortSentEntities resolves the entity set of an updateShortSentMessage
// echo, falling back to what the request submitted when the echo carries
// none.
//
// The short form's entities field is conditional, and Telegram documents
// neither when the flag is set nor whether an unchanged entity set is
// echoed back at all. Probing a live account did not settle it: a Saved
// Messages send — the case this form exists for — answers with the full
// updates envelope instead, so the short branch never fired. Rather than
// rest the documented "entitiesParsed: 0 means nothing parsed" contract
// on an unobservable flag, fall back to the submitted set here. That is
// safe because Telegram rejects malformed entities outright
// (ENTITY_BOUND_INVALID and friends) instead of dropping them silently,
// so a request that returned successfully applied every entity it
// carried.
//
// The fallback is deliberately scoped to THIS shape. On the full-updates
// path the server sends back its own message, and a zero there is a real
// "the server applied none" — the exact signal entitiesParsed exists to
// carry. Do not widen this.
func shortSentEntities(upd *tg.UpdateShortSentMessage, submitted []tg.MessageEntityClass) []Entity {
	if echoed := ConvertEntities(upd.Entities); len(echoed) > 0 {
		return echoed
	}

	return ConvertEntities(submitted)
}

func messagesFromUpdates(result tg.UpdatesClass) []Message {
	if result == nil {
		return nil
	}

	updates, users, chats, ok := unwrapUpdates(result)
	if !ok {
		return nil
	}

	userRefs := buildUserRefs(users)
	chatRefs := buildChatRefs(chats)

	var msgs []Message

	// extractNewMessage covers every new-message shape, scheduled ones
	// included — a scheduled album echoes updateNewScheduledMessage, and
	// a hand-rolled subset of this switch silently returned no messages
	// at all for it.
	for _, update := range updates {
		if msg := extractNewMessage(update, userRefs, chatRefs); msg != nil {
			msgs = append(msgs, *msg)
		}
	}

	return msgs
}

// unwrapUpdates accepts either *tg.Updates or *tg.UpdatesCombined and
// returns the common (updates, users, chats) triple. UpdatesCombined is
// what the server returns when the client's sequence diverges enough to
// need both Seq and SeqStart; ignoring it would silently drop the
// returned messages in messageFromUpdate / messagesFromUpdates.
//
// The short variants — *tg.UpdateShort, *tg.UpdateShortMessage,
// *tg.UpdateShortChatMessage, *tg.UpdateShortSentMessage,
// *tg.UpdatesTooLong — are deliberately NOT handled here. They don't
// carry parallel Users[]/Chats[] arrays (their inline message field
// references peers by bare ID only). SendMessage / EditMessage /
// ForwardMessages responses route the SentMessage shape directly in
// messageFromUpdate, and incoming-update paths don't currently feed
// through this helper. If a future tool subscribes to live updates,
// it must add cases for those variants rather than extending this
// switch — they need a different enrichment strategy.
func unwrapUpdates(result tg.UpdatesClass) ([]tg.UpdateClass, []tg.UserClass, []tg.ChatClass, bool) {
	switch upd := result.(type) {
	case *tg.Updates:
		return upd.Updates, upd.Users, upd.Chats, true
	case *tg.UpdatesCombined:
		return upd.Updates, upd.Users, upd.Chats, true
	default:
		return nil, nil, nil, false
	}
}

func enrichUpdateMessage(raw *tg.Message, users, chats map[int64]peerRef) Message {
	converted := ConvertMessage(raw)
	// UpdateNewMessage / UpdateNewChannelMessage don't carry the host
	// peer separately — pass an empty InputPeer so the DM-fallback in
	// fillSenderRef short-circuits. Sender resolution still works for
	// the common case where the message has a non-zero FromID.
	fillSenderRef(&converted, users, chats, InputPeer{})
	fillForwardRefs(&converted, users, chats)
	fillReplyToRef(&converted, users, chats)

	return converted
}

// messageFromUpdate converts a send/edit echo. submitted is the entity
// set the request carried; it is used ONLY to repair the short echo (see
// shortSentEntities) and is ignored on every other shape, where the
// server's message is authoritative — an echo that reports no entities
// there really means the server applied none.
func messageFromUpdate(result tg.UpdatesClass, submitted []tg.MessageEntityClass) *Message {
	if result == nil {
		return nil
	}

	if upd, ok := result.(*tg.UpdateShortSentMessage); ok {
		return &Message{
			ID:       upd.ID,
			Date:     upd.Date,
			Entities: shortSentEntities(upd, submitted),
		}
	}

	updates, users, chats, ok := unwrapUpdates(result)
	if !ok {
		return nil
	}

	return firstMessageFromUpdates(updates, buildUserRefs(users), buildChatRefs(chats))
}

// firstMessageFromUpdates returns the NEW message an echo carries. A
// send response can also carry an edit update for a PARENT message —
// the topic root's reply-counter bump — and the server's update order
// is not a contract, so edited messages are deliberately not eligible
// here: a send must never report the parent's ID as the message it
// just sent. The edit echo has its own extractor.
func firstMessageFromUpdates(updates []tg.UpdateClass, users, chats map[int64]peerRef) *Message {
	for _, update := range updates {
		if msg := extractNewMessage(update, users, chats); msg != nil {
			return msg
		}
	}

	return nil
}

// editedMessageFromUpdate converts the messages.editMessage echo, which
// carries an edit update and no new-message update at all. Kept apart
// from messageFromUpdate so the send paths cannot reach it.
func editedMessageFromUpdate(result tg.UpdatesClass, submitted []tg.MessageEntityClass) *Message {
	if result == nil {
		return nil
	}

	updates, users, chats, ok := unwrapUpdates(result)
	if !ok {
		return messageFromUpdate(result, submitted)
	}

	userRefs := buildUserRefs(users)
	chatRefs := buildChatRefs(chats)

	for _, update := range updates {
		if msg := extractEditedMessage(update, userRefs, chatRefs); msg != nil {
			return msg
		}
	}

	// Deliberately no new-message fallback: an editMessage envelope
	// carries an edit update, and returning some other new message the
	// envelope happened to bundle would report a foreign ID as "the
	// message you edited" — the mirror image of the bug the send path
	// just closed.
	return nil
}

func extractNewMessage(update tg.UpdateClass, users, chats map[int64]peerRef) *Message {
	switch upd := update.(type) {
	case *tg.UpdateNewMessage:
		return enrichFromMessageClass(upd.Message, users, chats)
	case *tg.UpdateNewChannelMessage:
		return enrichFromMessageClass(upd.Message, users, chats)
	case *tg.UpdateNewScheduledMessage:
		return enrichFromMessageClass(upd.Message, users, chats)
	}

	return nil
}

func extractEditedMessage(update tg.UpdateClass, users, chats map[int64]peerRef) *Message {
	switch upd := update.(type) {
	case *tg.UpdateEditMessage:
		return enrichFromMessageClass(upd.Message, users, chats)
	case *tg.UpdateEditChannelMessage:
		return enrichFromMessageClass(upd.Message, users, chats)
	}

	return nil
}

// enrichFromMessageClass converts one update's message payload when it
// is a regular message; service messages and empty stubs yield nil.
func enrichFromMessageClass(mc tg.MessageClass, users, chats map[int64]peerRef) *Message {
	msg, ok := mc.(*tg.Message)
	if !ok {
		return nil
	}

	enriched := enrichUpdateMessage(msg, users, chats)

	return &enriched
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
				Type:  PeerInfoTypeChat,
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
				Type:  PeerInfoTypeChat,
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

	users := buildUserRefs(result.Users)
	chats := buildChatRefs(result.Chats)
	items := make([]ReactionUser, 0, len(result.Reactions))

	for idx := range result.Reactions {
		reaction := &result.Reactions[idx]
		peerID, peerType := extractFromIDAndType(reaction.PeerID)

		item := ReactionUser{
			UserID:   peerID,
			PeerType: peerType,
			Emoji:    reactionEmoji(reaction.Reaction),
		}

		// A channel reactor's title lives in Chats, a user's name in
		// Users; lookupRefByPeer picks the right one by kind.
		ref, _ := lookupRefByPeer(InputPeer{Type: peerType, ID: peerID}, users, chats)
		item.Name = ref.Name
		item.Username = ref.Username

		items = append(items, item)
	}

	return items
}

func reactionEmoji(reaction tg.ReactionClass) string {
	switch typed := reaction.(type) {
	case *tg.ReactionEmoji:
		return typed.Emoticon
	case *tg.ReactionCustomEmoji:
		return ReactionCustomPrefix + strconv.FormatInt(typed.DocumentID, 10)
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
