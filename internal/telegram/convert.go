package telegram

import (
	"slices"

	"github.com/gotd/td/tg"
)

// ConvertUser converts a tg.User to our domain User.
func ConvertUser(raw *tg.User) User {
	if raw == nil {
		return User{}
	}

	return User{
		ID:        raw.ID,
		FirstName: raw.FirstName,
		LastName:  raw.LastName,
		Username:  raw.Username,
		Phone:     raw.Phone,
		Bot:       raw.Bot,
	}
}

// ConvertMessage converts a tg.Message to our domain Message.
func ConvertMessage(raw *tg.Message) Message {
	if raw == nil {
		return Message{}
	}

	msg := Message{
		ID:       raw.ID,
		Date:     raw.Date,
		Text:     raw.Message,
		Type:     MessageType(raw),
		Views:    raw.Views,
		Forwards: raw.Forwards,
		EditDate: raw.EditDate,
	}

	msg.PeerID = extractPeerID(raw.PeerID)
	msg.FromID, msg.FromType = extractFromIDAndType(raw.FromID)
	msg.ReplyTo = extractReplyTo(raw.ReplyTo)
	msg.TopicID = extractTopicID(raw.ReplyTo)
	msg.Entities = ConvertEntities(raw.Entities)
	msg.Forward = extractForward(raw)

	return msg
}

// extractForward pulls Telegram MessageFwdHeader fields into a domain
// ForwardInfo. Returns nil when the message is not a forward. Resolving
// PeerRef.Name and PeerRef.Username is deferred to the wrapper layer,
// which has access to the Users/Chats arrays from the MTProto response.
func extractForward(raw *tg.Message) *ForwardInfo {
	fwd, ok := raw.GetFwdFrom()
	if !ok {
		return nil
	}

	info := &ForwardInfo{
		Date: fwd.Date,
	}

	if fromName, hasFromName := fwd.GetFromName(); hasFromName {
		info.FromName = fromName
	}

	if channelPost, hasPost := fwd.GetChannelPost(); hasPost {
		info.ChannelPost = channelPost
	}

	if postAuthor, hasAuthor := fwd.GetPostAuthor(); hasAuthor {
		info.PostAuthor = postAuthor
	}

	if fromID, hasFromID := fwd.GetFromID(); hasFromID {
		peer := extractPeerID(fromID)
		if peer != (InputPeer{}) {
			info.From = &PeerRef{Peer: peer}
		}
	}

	return info
}

func extractPeerID(peer tg.PeerClass) InputPeer {
	if peer == nil {
		return InputPeer{}
	}

	switch typed := peer.(type) {
	case *tg.PeerUser:
		return InputPeer{Type: PeerUser, ID: typed.UserID}
	case *tg.PeerChat:
		return InputPeer{Type: PeerChat, ID: typed.ChatID}
	case *tg.PeerChannel:
		return InputPeer{Type: PeerChannel, ID: typed.ChannelID}
	default:
		return InputPeer{}
	}
}

// extractFromID returns just the numeric ID for a PeerClass — used
// where the peer kind has already been narrowed by context (e.g. the
// reaction-list endpoint where every PeerID is a user).
func extractFromID(from tg.PeerClass) int64 {
	id, _ := extractFromIDAndType(from)

	return id
}

// extractFromIDAndType returns the bare ID and the peer kind for a
// Message.FromID. The kind matters because Telegram supergroups let a
// channel admin post under the channel's own identity — the FromID is
// then a PeerChannel and the rendered label should read channel:N, not
// user:N. Returns (0, PeerUser) for a nil/unknown PeerClass; callers
// that care about absence check FromID == 0.
func extractFromIDAndType(from tg.PeerClass) (int64, PeerType) {
	if from == nil {
		return 0, PeerUser
	}

	switch peer := from.(type) {
	case *tg.PeerUser:
		return peer.UserID, PeerUser
	case *tg.PeerChat:
		return peer.ChatID, PeerChat
	case *tg.PeerChannel:
		return peer.ChannelID, PeerChannel
	default:
		return 0, PeerUser
	}
}

func extractReplyTo(reply tg.MessageReplyHeaderClass) *ReplyToInfo {
	hdr := replyHeader(reply)
	if hdr == nil {
		return nil
	}

	info := &ReplyToInfo{MessageID: hdr.ReplyToMsgID}
	fillReplyTopID(info, hdr)
	fillReplyQuote(info, hdr)
	fillReplyPeer(info, hdr)

	return info
}

func replyHeader(reply tg.MessageReplyHeaderClass) *tg.MessageReplyHeader {
	if reply == nil {
		return nil
	}

	hdr, ok := reply.(*tg.MessageReplyHeader)
	if !ok {
		return nil
	}

	if hdr.ReplyToMsgID == 0 {
		return nil
	}

	// In forum topics without explicit ReplyToTopID,
	// ReplyToMsgID is the topic root, not a reply target.
	if hdr.ForumTopic {
		if _, hasTop := hdr.GetReplyToTopID(); !hasTop {
			return nil
		}
	}

	return hdr
}

func fillReplyTopID(info *ReplyToInfo, hdr *tg.MessageReplyHeader) {
	if topID, hasTop := hdr.GetReplyToTopID(); hasTop && topID != 0 {
		info.TopID = topID
	}
}

func fillReplyQuote(info *ReplyToInfo, hdr *tg.MessageReplyHeader) {
	if quote, hasQuote := hdr.GetQuoteText(); hasQuote && quote != "" {
		info.QuoteText = quote
	}
}

func fillReplyPeer(info *ReplyToInfo, hdr *tg.MessageReplyHeader) {
	peer, hasPeer := hdr.GetReplyToPeerID()
	if !hasPeer || peer == nil {
		return
	}

	extracted := extractPeerID(peer)
	if extracted == (InputPeer{}) {
		return
	}

	info.FromPeerID = &extracted
}

func extractTopicID(reply tg.MessageReplyHeaderClass) int {
	if reply == nil {
		return 0
	}

	hdr, ok := reply.(*tg.MessageReplyHeader)
	if !ok {
		return 0
	}

	// Check ReplyToTopID first — present for both forum topics
	// and General topic (where ForumTopic flag is false but
	// ReplyToTopID=1).
	topicID, hasTopicID := hdr.GetReplyToTopID()
	if hasTopicID {
		return topicID
	}

	// Fallback: ForumTopic flag set, ReplyToMsgID is the topic root.
	if hdr.ForumTopic {
		return hdr.ReplyToMsgID
	}

	return 0
}

// Message type labels returned by MessageType. Exported because tests
// and tools share the same stable API values.
const (
	MessageTypeText        = "text"
	MessageTypePhoto       = "photo"
	MessageTypeVoice       = "voice"
	MessageTypeVideoNote   = "video_note"
	MessageTypeVideo       = "video"
	MessageTypeAudio       = "audio"
	MessageTypeSticker     = "sticker"
	MessageTypeAnimation   = "animation"
	MessageTypeDocument    = "document"
	MessageTypeContact     = "contact"
	MessageTypeLocation    = "location"
	MessageTypeVenue       = "venue"
	MessageTypePoll        = "poll"
	MessageTypeWebpage     = "webpage"
	MessageTypeGame        = "game"
	MessageTypeInvoice     = "invoice"
	MessageTypeUnsupported = "unsupported"
)

func messageTypes() []string {
	return []string{
		MessageTypeText,
		MessageTypePhoto,
		MessageTypeVoice,
		MessageTypeVideoNote,
		MessageTypeVideo,
		MessageTypeAudio,
		MessageTypeSticker,
		MessageTypeAnimation,
		MessageTypeDocument,
		MessageTypeContact,
		MessageTypeLocation,
		MessageTypeVenue,
		MessageTypePoll,
		MessageTypeWebpage,
		MessageTypeGame,
		MessageTypeInvoice,
		MessageTypeUnsupported,
	}
}

// IsMessageType reports whether messageType is one of the public type
// labels emitted by MessageType.
func IsMessageType(messageType string) bool {
	return slices.Contains(messageTypes(), messageType)
}

// MessageType returns a stable, machine-readable type label for a
// Telegram message. It does not download media; document-like messages
// are classified from MTProto document attributes.
func MessageType(raw *tg.Message) string {
	if raw == nil {
		return ""
	}

	if raw.Message != "" && isWebpagePreview(raw.Media) {
		return MessageTypeText
	}

	return MessagePayloadType(raw.Media)
}

// MessagePayloadType returns a stable type label for a message media
// payload. A nil media payload is still a regular text message.
func MessagePayloadType(media tg.MessageMediaClass) string {
	if media == nil {
		return MessageTypeText
	}

	if typed, ok := media.(*tg.MessageMediaDocument); ok {
		return documentMessageType(typed)
	}

	if messageType, ok := simpleMediaMessageType(media); ok {
		return messageType
	}

	return MessageTypeUnsupported
}

func isWebpagePreview(media tg.MessageMediaClass) bool {
	_, ok := media.(*tg.MessageMediaWebPage)

	return ok
}

func simpleMediaMessageType(media tg.MessageMediaClass) (string, bool) {
	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return MessageTypePhoto, true
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive:
		return MessageTypeLocation, true
	case *tg.MessageMediaContact:
		return MessageTypeContact, true
	case *tg.MessageMediaVenue:
		return MessageTypeVenue, true
	case *tg.MessageMediaWebPage:
		return MessageTypeWebpage, true
	case *tg.MessageMediaPoll:
		return MessageTypePoll, true
	case *tg.MessageMediaGame:
		return MessageTypeGame, true
	case *tg.MessageMediaInvoice:
		return MessageTypeInvoice, true
	default:
		return "", false
	}
}

func documentMessageType(media *tg.MessageMediaDocument) string {
	if media == nil {
		return MessageTypeDocument
	}

	doc, ok := media.Document.(*tg.Document)
	if !ok {
		return MessageTypeDocument
	}

	return documentAttributesType(doc.Attributes)
}

func documentAttributesType(attrs []tg.DocumentAttributeClass) string {
	var flags documentAttributeFlags

	for _, attr := range attrs {
		messageType, terminal := classifyDocumentAttribute(attr, &flags)
		if terminal {
			return messageType
		}
	}

	return documentAttributeFlagsType(flags)
}

type documentAttributeFlags struct {
	sawAudio    bool
	sawVideo    bool
	sawAnimated bool
}

func classifyDocumentAttribute(
	attr tg.DocumentAttributeClass,
	flags *documentAttributeFlags,
) (string, bool) {
	switch typed := attr.(type) {
	case *tg.DocumentAttributeSticker:
		return MessageTypeSticker, true
	case *tg.DocumentAttributeAudio:
		if typed.GetVoice() {
			return MessageTypeVoice, true
		}

		flags.sawAudio = true
	case *tg.DocumentAttributeVideo:
		if typed.GetRoundMessage() {
			return MessageTypeVideoNote, true
		}

		flags.sawVideo = true
	case *tg.DocumentAttributeAnimated:
		flags.sawAnimated = true
	}

	return "", false
}

func documentAttributeFlagsType(flags documentAttributeFlags) string {
	switch {
	case flags.sawAnimated:
		return MessageTypeAnimation
	case flags.sawVideo:
		return MessageTypeVideo
	case flags.sawAudio:
		return MessageTypeAudio
	default:
		return MessageTypeDocument
	}
}

// InputPeerToTG converts our domain InputPeer to a tg.InputPeerClass.
func InputPeerToTG(peer InputPeer) tg.InputPeerClass {
	switch peer.Type {
	case PeerUser:
		return &tg.InputPeerUser{
			UserID:     peer.ID,
			AccessHash: peer.AccessHash,
		}
	case PeerChat:
		return &tg.InputPeerChat{
			ChatID: peer.ID,
		}
	case PeerChannel:
		return &tg.InputPeerChannel{
			ChannelID:  peer.ID,
			AccessHash: peer.AccessHash,
		}
	default:
		return &tg.InputPeerEmpty{}
	}
}

// InputChannelFromPeer creates a tg.InputChannel from our InputPeer.
func InputChannelFromPeer(peer InputPeer) *tg.InputChannel {
	return &tg.InputChannel{
		ChannelID:  peer.ID,
		AccessHash: peer.AccessHash,
	}
}

// InputUserFromPeer creates a tg.InputUser from our InputPeer.
func InputUserFromPeer(peer InputPeer) *tg.InputUser {
	return &tg.InputUser{
		UserID:     peer.ID,
		AccessHash: peer.AccessHash,
	}
}
