package telegram

import (
	"context"
	"strings"
	"unicode"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// maxTopicDeleteRounds bounds DeleteForumTopic's batch loop; see
// maxUnpinRounds for the reasoning.
const maxTopicDeleteRounds = 1000

// ErrChannelOnly is returned for an operation Telegram offers only on
// channels and supergroups.
var ErrChannelOnly = errors.New("this works only for channels and supergroups")

// ErrBasicGroupOnly is returned when a supergroup upgrade is asked of
// something that is not a basic group.
var ErrBasicGroupOnly = errors.New("only a basic group can be upgraded to a supergroup")

// ErrInviteNeedsWebView is returned when an invite link leads to a paid
// subscription, which can only be joined through the bot's web view.
var ErrInviteNeedsWebView = errors.New("this invite link needs a paid subscription confirmed in an official Telegram app")

// ErrTopicDeleteIncomplete is returned when a topic still has history after
// maxTopicDeleteRounds batches.
var ErrTopicDeleteIncomplete = errors.New("the topic still has messages after the maximum number of delete rounds")

// InviteInfo describes the chat behind an invite link.
type InviteInfo struct {
	Title             string    `json:"title"`
	About             string    `json:"about,omitempty"`
	Type              string    `json:"type"`
	ParticipantsCount int       `json:"participantsCount,omitempty"`
	RequestNeeded     bool      `json:"requestNeeded,omitempty"`
	AlreadyJoined     bool      `json:"alreadyJoined"`
	Peer              InputPeer `json:"peer,omitzero"`
}

// AdminLogEvent is one entry of a channel's or supergroup's admin log.
type AdminLogEvent struct {
	ID     int64  `json:"id"`
	Date   int    `json:"date"`
	UserID int64  `json:"userId"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// AdminLogOpts filters an admin log read.
type AdminLogOpts struct {
	Query string
	Limit int
	MaxID int64
}

// BanMember bans a user from a channel or supergroup until untilDate (0:
// forever), or lifts the ban when unban is set. Basic groups have no bans;
// tg_groups_members_remove is the only way out there.
func (w *Wrapper) BanMember(ctx context.Context, group, user InputPeer, untilDate int, unban bool) error {
	if group.Type != PeerChannel {
		return ErrChannelOnly
	}

	rights := tg.ChatBannedRights{UntilDate: untilDate}
	if !unban {
		rights.SetViewMessages(true)
	}

	_, err := w.api.ChannelsEditBanned(ctx, &tg.ChannelsEditBannedRequest{
		Channel:      InputChannelFromPeer(group),
		Participant:  InputPeerToTG(user),
		BannedRights: rights,
	})
	if err != nil {
		return errors.Wrap(err, "editing ban")
	}

	return nil
}

// CheckInvite reads the chat behind an invite link without joining it.
func (w *Wrapper) CheckInvite(ctx context.Context, link string) (*InviteInfo, error) {
	invite, err := w.api.MessagesCheckChatInvite(ctx, inviteHash(link))
	if err != nil {
		return nil, errors.Wrap(err, "checking invite link")
	}

	return inviteInfo(invite), nil
}

// JoinByInvite joins the chat behind a private invite link and returns it.
func (w *Wrapper) JoinByInvite(ctx context.Context, link string) (*PeerInfo, error) {
	result, err := w.api.MessagesImportChatInvite(ctx, inviteHash(link))
	if tgerr.Is(err, "USER_ALREADY_PARTICIPANT") {
		return w.alreadyJoined(ctx, link)
	}

	if err != nil {
		return nil, errors.Wrap(err, "joining by invite link")
	}

	joined, ok := result.(*tg.MessagesChatInviteJoinResultOk)
	if !ok {
		return nil, ErrInviteNeedsWebView
	}

	chat, found := firstChat(joined.Updates)
	if !found {
		return &PeerInfo{Type: PeerInfoTypeChat}, nil
	}

	return chat, nil
}

// alreadyJoined answers a join of a chat the account is already in with
// that chat, which is what the caller was after — verified live, a repeat
// join otherwise surfaces as a bare USER_ALREADY_PARTICIPANT.
func (w *Wrapper) alreadyJoined(ctx context.Context, link string) (*PeerInfo, error) {
	info, err := w.CheckInvite(ctx, link)
	if err != nil {
		return nil, err
	}

	return &PeerInfo{Peer: info.Peer, Title: info.Title, Type: info.Type}, nil
}

// UpgradeToSupergroup migrates a basic group into a new supergroup and
// returns the supergroup. The basic group stops accepting messages.
func (w *Wrapper) UpgradeToSupergroup(ctx context.Context, peer InputPeer) (*PeerInfo, error) {
	if peer.Type != PeerChat {
		return nil, ErrBasicGroupOnly
	}

	updates, err := w.api.MessagesMigrateChat(ctx, peer.ID)
	if err != nil {
		return nil, errors.Wrap(err, "upgrading to supergroup")
	}

	channel, found := migratedChannel(updates, peer.ID)
	if !found {
		return &PeerInfo{Type: PeerInfoTypeSupergroup}, nil
	}

	w.cache.Store(channel.Peer)

	return channel, nil
}

// SetForum turns forum topics on or off in a supergroup.
func (w *Wrapper) SetForum(ctx context.Context, peer InputPeer, enabled bool) error {
	if peer.Type != PeerChannel {
		return ErrChannelOnly
	}

	_, err := w.api.ChannelsToggleForum(ctx, &tg.ChannelsToggleForumRequest{
		Channel: InputChannelFromPeer(peer),
		Enabled: enabled,
	})
	if err != nil {
		return errors.Wrap(err, "toggling forum")
	}

	return nil
}

// DeleteForumTopic deletes a forum topic with all its messages.
func (w *Wrapper) DeleteForumTopic(ctx context.Context, peer InputPeer, topicID int) error {
	req := &tg.MessagesDeleteTopicHistoryRequest{Peer: InputPeerToTG(peer), TopMsgID: topicID}

	for range maxTopicDeleteRounds {
		result, err := w.api.MessagesDeleteTopicHistory(ctx, req)
		if err != nil {
			return errors.Wrap(err, "deleting forum topic")
		}

		if result.Offset == 0 {
			return nil
		}
	}

	return ErrTopicDeleteIncomplete
}

// GetAdminLog reads the recent admin actions of a channel or supergroup,
// newest first.
func (w *Wrapper) GetAdminLog(ctx context.Context, peer InputPeer, opts AdminLogOpts) ([]AdminLogEvent, error) {
	if peer.Type != PeerChannel {
		return nil, ErrChannelOnly
	}

	result, err := w.api.ChannelsGetAdminLog(ctx, &tg.ChannelsGetAdminLogRequest{
		Channel: InputChannelFromPeer(peer),
		Q:       opts.Query,
		MaxID:   opts.MaxID,
		Limit:   opts.Limit,
	})
	if err != nil {
		return nil, errors.Wrap(err, "reading admin log")
	}

	events := make([]AdminLogEvent, 0, len(result.Events))
	for _, event := range result.Events {
		events = append(events, AdminLogEvent{
			ID:     event.ID,
			Date:   event.Date,
			UserID: event.UserID,
			Action: adminLogActionName(event.Action),
			Detail: adminLogDetail(event.Action),
		})
	}

	return events, nil
}

// DeleteChatPhoto removes the photo of a group or channel.
func (w *Wrapper) DeleteChatPhoto(ctx context.Context, peer InputPeer) error {
	if peer.Type == PeerChannel {
		_, err := w.api.ChannelsEditPhoto(ctx, &tg.ChannelsEditPhotoRequest{
			Channel: InputChannelFromPeer(peer),
			Photo:   &tg.InputChatPhotoEmpty{},
		})

		return errors.Wrap(err, "deleting channel photo")
	}

	_, err := w.api.MessagesEditChatPhoto(ctx, &tg.MessagesEditChatPhotoRequest{
		ChatID: peer.ID,
		Photo:  &tg.InputChatPhotoEmpty{},
	})

	return errors.Wrap(err, "deleting chat photo")
}

// inviteHash takes an invite link in any common spelling — a full
// t.me/+hash or t.me/joinchat/hash URL, "+hash", or the bare hash — and
// returns the hash.
func inviteHash(link string) string {
	ident := normalizePeerIdentifier(strings.TrimSpace(link))
	if hash := extractInviteHash(ident); hash != "" {
		return hash
	}

	return ident
}

func inviteInfo(invite tg.ChatInviteClass) *InviteInfo {
	switch typed := invite.(type) {
	case *tg.ChatInvite:
		return &InviteInfo{
			Title:             typed.Title,
			About:             typed.About,
			Type:              inviteType(typed),
			ParticipantsCount: typed.ParticipantsCount,
			RequestNeeded:     typed.RequestNeeded,
		}
	case *tg.ChatInviteAlready:
		info := chatInfo(typed.Chat)
		info.AlreadyJoined = true

		return info
	case *tg.ChatInvitePeek:
		return chatInfo(typed.Chat)
	default:
		return &InviteInfo{}
	}
}

func inviteType(invite *tg.ChatInvite) string {
	switch {
	case invite.Broadcast:
		return PeerInfoTypeChannel
	case invite.Megagroup:
		return PeerInfoTypeSupergroup
	default:
		return PeerInfoTypeChat
	}
}

func chatInfo(chat tg.ChatClass) *InviteInfo {
	info := &InviteInfo{}

	switch typed := chat.(type) {
	case *tg.Channel:
		info.Title = typed.Title
		info.Type = PeerInfoTypeSupergroup
		info.ParticipantsCount = typed.ParticipantsCount

		if typed.Broadcast {
			info.Type = PeerInfoTypeChannel
		}
	case *tg.Chat:
		info.Title = typed.Title
		info.Type = PeerInfoTypeChat
		info.ParticipantsCount = typed.ParticipantsCount
	}

	peer, err := peerFromChat(chat)
	if err == nil {
		info.Peer = peer
	}

	return info
}

// firstChat returns the first group or channel carried by an updates
// envelope, which is the joined chat for importChatInvite.
func firstChat(updates tg.UpdatesClass) (*PeerInfo, bool) {
	envelope, ok := updates.(*tg.Updates)
	if !ok {
		return nil, false
	}

	for _, chat := range envelope.Chats {
		if info, found := chatPeerInfo(chat); found {
			return info, true
		}
	}

	return nil, false
}

// migratedChannel finds the supergroup a basic group was migrated into. The
// envelope carries the old chat with migrated_to naming the new channel; when
// it does not, the only channel in the envelope is the answer.
func migratedChannel(updates tg.UpdatesClass, chatID int64) (*PeerInfo, bool) {
	envelope, ok := updates.(*tg.Updates)
	if !ok {
		return nil, false
	}

	var target int64

	for _, chat := range envelope.Chats {
		old, isChat := chat.(*tg.Chat)
		if !isChat || old.ID != chatID {
			continue
		}

		if moved, set := old.GetMigratedTo(); set {
			if channel, isChannel := moved.(*tg.InputChannel); isChannel {
				target = channel.ChannelID
			}
		}
	}

	var fallback *PeerInfo

	for _, chat := range envelope.Chats {
		channel, isChannel := chat.(*tg.Channel)
		if !isChannel {
			continue
		}

		info, _ := chatPeerInfo(channel)
		if channel.ID == target {
			return info, true
		}

		fallback = info
	}

	return fallback, fallback != nil
}

func chatPeerInfo(chat tg.ChatClass) (*PeerInfo, bool) {
	switch typed := chat.(type) {
	case *tg.Channel:
		kind := PeerInfoTypeSupergroup
		if typed.Broadcast {
			kind = PeerInfoTypeChannel
		}

		return &PeerInfo{
			Peer:     InputPeer{Type: PeerChannel, ID: typed.ID, AccessHash: typed.AccessHash},
			Title:    typed.Title,
			Username: typed.Username,
			Type:     kind,
		}, true
	case *tg.Chat:
		return &PeerInfo{Peer: InputPeer{Type: PeerChat, ID: typed.ID}, Title: typed.Title, Type: PeerInfoTypeChat}, true
	default:
		return nil, false
	}
}

// adminLogActionName turns an action constructor into a snake_case label:
// channelAdminLogEventActionChangeTitle becomes change_title.
func adminLogActionName(action tg.ChannelAdminLogEventActionClass) string {
	if action == nil {
		return "unrecognized"
	}

	name := strings.TrimPrefix(action.TypeName(), "channelAdminLogEventAction")

	var buf strings.Builder

	for idx, char := range name {
		if unicode.IsUpper(char) {
			if idx > 0 {
				buf.WriteByte('_')
			}

			char = unicode.ToLower(char)
		}

		buf.WriteRune(char)
	}

	return buf.String()
}

// adminLogDetail summarizes the actions an admin most often looks for. The
// rest are named by adminLogActionName alone.
func adminLogDetail(action tg.ChannelAdminLogEventActionClass) string {
	switch typed := action.(type) {
	case *tg.ChannelAdminLogEventActionChangeTitle:
		return typed.PrevValue + " → " + typed.NewValue
	case *tg.ChannelAdminLogEventActionChangeAbout:
		return typed.PrevValue + " → " + typed.NewValue
	case *tg.ChannelAdminLogEventActionDeleteMessage:
		return logMessageText(typed.Message)
	case *tg.ChannelAdminLogEventActionEditMessage:
		return logMessageText(typed.PrevMessage) + " → " + logMessageText(typed.NewMessage)
	case *tg.ChannelAdminLogEventActionUpdatePinned:
		return logMessageText(typed.Message)
	default:
		return ""
	}
}

func logMessageText(msg tg.MessageClass) string {
	if typed, ok := msg.(*tg.Message); ok {
		return typed.Message
	}

	return ""
}
