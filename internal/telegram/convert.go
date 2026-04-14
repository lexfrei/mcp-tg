package telegram

import "github.com/gotd/td/tg"

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
		ID:        raw.ID,
		Date:      raw.Date,
		Text:      raw.Message,
		Views:     raw.Views,
		Forwards:  raw.Forwards,
		EditDate:  raw.EditDate,
		MediaType: MessageMediaType(raw.Media),
	}

	msg.PeerID = extractPeerID(raw.PeerID)
	msg.FromID = extractFromID(raw.FromID)
	msg.ReplyTo = extractReplyTo(raw.ReplyTo)
	msg.TopicID = extractTopicID(raw.ReplyTo)

	return msg
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

func extractFromID(from tg.PeerClass) int64 {
	if from == nil {
		return 0
	}

	switch peer := from.(type) {
	case *tg.PeerUser:
		return peer.UserID
	case *tg.PeerChat:
		return peer.ChatID
	case *tg.PeerChannel:
		return peer.ChannelID
	default:
		return 0
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

// MessageMediaType returns a string label for a message media type.
func MessageMediaType(media tg.MessageMediaClass) string {
	if media == nil {
		return ""
	}

	switch media.(type) {
	case *tg.MessageMediaPhoto:
		return "photo"
	case *tg.MessageMediaDocument:
		return "document"
	case *tg.MessageMediaGeo:
		return "geo"
	case *tg.MessageMediaContact:
		return "contact"
	case *tg.MessageMediaVenue:
		return "venue"
	case *tg.MessageMediaWebPage:
		return "webpage"
	case *tg.MessageMediaPoll:
		return "poll"
	default:
		return "other"
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
