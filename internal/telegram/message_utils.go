package telegram

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// maxUnpinRounds bounds UnpinAllMessages. The server unpins in batches and
// reports an offset while more remain; a chat with more pinned messages than
// this many batches is not a real case, and the bound keeps a misbehaving
// answer from looping forever.
const maxUnpinRounds = 100

// ErrMessageLinkUnsupported is returned for a message link outside a channel
// or supergroup: Telegram exports links only for those.
var ErrMessageLinkUnsupported = errors.New(
	"message links exist only for channels and supergroups; private chats and basic groups have none",
)

// ErrUnpinIncomplete is returned when the server still reports pinned
// messages after maxUnpinRounds batches.
var ErrUnpinIncomplete = errors.New("messages are still pinned after the maximum number of unpin rounds")

// ReadReceipt is one member who read a message, and when.
type ReadReceipt struct {
	UserID int64 `json:"userId"`
	Date   int   `json:"date"`
}

// Draft is an unsent message saved in a chat.
type Draft struct {
	Peer     InputPeer `json:"peer"`
	Name     string    `json:"name,omitempty"`
	Username string    `json:"username,omitempty"`
	TopicID  int       `json:"topicId,omitempty"`
	ReplyTo  int       `json:"replyTo,omitempty"`
	Text     string    `json:"text"`
	Date     int       `json:"date"`
}

// Contact is a contact card to send.
type Contact struct {
	Phone     string
	FirstName string
	LastName  string
}

// GetMessageLink returns the t.me link of a message in a channel or
// supergroup. thread asks for the link to open the message inside its
// comment thread or forum topic.
func (w *Wrapper) GetMessageLink(ctx context.Context, peer InputPeer, msgID int, thread bool) (string, error) {
	if peer.Type != PeerChannel {
		return "", ErrMessageLinkUnsupported
	}

	link, err := w.api.ChannelsExportMessageLink(ctx, &tg.ChannelsExportMessageLinkRequest{
		Channel: InputChannelFromPeer(peer),
		ID:      msgID,
		Thread:  thread,
	})
	if err != nil {
		return "", errors.Wrap(err, "exporting message link")
	}

	return link.Link, nil
}

// GetMessageReadBy returns who read one of the account's own messages.
// Telegram answers only in small groups, and only for recent messages.
func (w *Wrapper) GetMessageReadBy(ctx context.Context, peer InputPeer, msgID int) ([]ReadReceipt, error) {
	dates, err := w.api.MessagesGetMessageReadParticipants(ctx, &tg.MessagesGetMessageReadParticipantsRequest{
		Peer:  InputPeerToTG(peer),
		MsgID: msgID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting read participants")
	}

	receipts := make([]ReadReceipt, 0, len(dates))
	for _, date := range dates {
		receipts = append(receipts, ReadReceipt{UserID: date.UserID, Date: date.Date})
	}

	return receipts, nil
}

// UnpinAllMessages unpins every pinned message in a chat, or in one forum
// topic when topicID is set.
func (w *Wrapper) UnpinAllMessages(ctx context.Context, peer InputPeer, topicID int) error {
	req := &tg.MessagesUnpinAllMessagesRequest{Peer: InputPeerToTG(peer)}
	if topicID > 0 {
		req.SetTopMsgID(topicID)
	}

	for range maxUnpinRounds {
		result, err := w.api.MessagesUnpinAllMessages(ctx, req)
		if err != nil {
			return errors.Wrap(err, "unpinning all messages")
		}

		if result.Offset == 0 {
			return nil
		}
	}

	return ErrUnpinIncomplete
}

// GetAllDrafts returns the drafts saved in every chat.
func (w *Wrapper) GetAllDrafts(ctx context.Context) ([]Draft, error) {
	result, err := w.api.MessagesGetAllDrafts(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting drafts")
	}

	updates, ok := result.(*tg.Updates)
	if !ok {
		return nil, nil
	}

	return draftsFromUpdates(updates), nil
}

// SendContact sends a contact card.
func (w *Wrapper) SendContact(ctx context.Context, peer InputPeer, contact Contact, opts SendOpts) (*Message, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	req := &tg.MessagesSendMediaRequest{
		Peer: InputPeerToTG(peer),
		Media: &tg.InputMediaContact{
			PhoneNumber: contact.Phone,
			FirstName:   contact.FirstName,
			LastName:    contact.LastName,
		},
		RandomID: randID,
		Silent:   opts.Silent,
	}

	if reply := buildReplyTo(opts.TopicID, opts.ReplyTo); reply != nil {
		req.ReplyTo = reply
	}

	applySendAs(opts.SendAs, req.SetSendAs)

	result, err := w.api.MessagesSendMedia(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending contact")
	}

	return messageFromUpdate(result, peer, randID, nil), nil
}

func draftsFromUpdates(updates *tg.Updates) []Draft {
	users := buildUserRefs(updates.Users)
	chats := buildChatRefs(updates.Chats)

	var drafts []Draft

	for _, update := range updates.Updates {
		draftUpdate, ok := update.(*tg.UpdateDraftMessage)
		if !ok {
			continue
		}

		draft, ok := draftUpdate.Draft.(*tg.DraftMessage)
		if !ok {
			continue
		}

		peer := extractPeerID(draftUpdate.Peer)
		item := Draft{Peer: peer, TopicID: draftUpdate.TopMsgID, Text: draft.Message, Date: draft.Date}

		refs := chats
		if peer.Type == PeerUser {
			refs = users
		}

		applyPeerRef(&item.Name, &item.Username, refs[peer.ID])

		if reply, isMessage := draft.ReplyTo.(*tg.InputReplyToMessage); isMessage {
			item.ReplyTo = reply.ReplyToMsgID
		}

		drafts = append(drafts, item)
	}

	return drafts
}
