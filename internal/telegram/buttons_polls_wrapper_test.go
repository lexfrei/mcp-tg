package telegram

import (
	"bytes"
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// botInvoker serves one message for every message read and records the
// requests that act on it.
type botInvoker struct {
	message  *tg.Message
	fullUser *tg.UserFull
	sendEcho func(req *tg.MessagesSendMediaRequest) tg.UpdatesClass
	// pollResults answers messages.getPollResults; nil fails the call.
	pollResults tg.UpdatesClass
	resultReads int

	callbacks []*tg.MessagesGetBotCallbackAnswerRequest
	sends     []*tg.MessagesSendMessageRequest
	votes     []*tg.MessagesSendVoteRequest
}

func (b *botInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.MessagesGetMessagesRequest:
		return encodeAndDecode(&tg.MessagesMessages{Messages: []tg.MessageClass{b.message}}, output)
	case *tg.MessagesGetBotCallbackAnswerRequest:
		b.callbacks = append(b.callbacks, req)

		return encodeAndDecode(&tg.MessagesBotCallbackAnswer{Message: "done", CacheTime: 1}, output)
	case *tg.MessagesSendMessageRequest:
		b.sends = append(b.sends, req)

		return encodeAndDecode(&tg.UpdateShortSentMessage{ID: 77, Date: 1}, output)
	case *tg.MessagesSendVoteRequest:
		b.votes = append(b.votes, req)

		return encodeAndDecode(&tg.Updates{}, output)
	case *tg.UsersGetFullUserRequest:
		return encodeAndDecode(&tg.UsersUserFull{FullUser: *b.fullUser}, output)
	case *tg.MessagesSendMediaRequest:
		return encodeAndDecode(b.sendEcho(req), output)
	case *tg.MessagesGetPollResultsRequest:
		b.resultReads++

		if b.pollResults == nil {
			return errors.Wrapf(errUnexpectedRequest, "%T", input)
		}

		return encodeAndDecode(b.pollResults, output)
	default:
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}
}

var botPeer = InputPeer{Type: PeerUser, ID: 5, AccessHash: 50}

func inlineKeyboard(buttons ...tg.KeyboardInlineButton) *tg.ReplyInlineMarkup {
	return &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{{Buttons: buttons}}}
}

func messageWithMarkup(markup tg.ReplyMarkupClass) *tg.Message {
	return &tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 5}, ReplyMarkup: markup}
}

// The press uses the keyboard as the re-read message has it now.
func TestPressButton_CallbackCarriesTheCurrentData(t *testing.T) {
	invoker := &botInvoker{message: messageWithMarkup(inlineKeyboard(tg.KeyboardInlineButton{
		Text: "Go", Type: &tg.InlineButtonTypeCallback{Data: []byte("now")},
	}))}

	press, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), botPeer, 10, 0, 0)
	if err != nil {
		t.Fatalf("PressButton: %v", err)
	}

	if len(invoker.callbacks) != 1 || !bytes.Equal(invoker.callbacks[0].Data, []byte("now")) ||
		invoker.callbacks[0].MsgID != 10 || press.Message != "done" {
		t.Errorf("callbacks = %+v, press = %+v; want one press with data \"now\" on message 10", invoker.callbacks, press)
	}
}

func TestPressButton_RefusesWhatItCannotPress(t *testing.T) {
	cases := map[string]struct {
		markup tg.ReplyMarkupClass
		want   error
	}{
		"password": {inlineKeyboard(tg.KeyboardInlineButton{
			Text: "Transfer", Type: &tg.InlineButtonTypeCallback{Data: []byte("x"), RequiresPassword: true},
		}), ErrButtonNeedsPassword},
		"game": {inlineKeyboard(tg.KeyboardInlineButton{Text: "Play", Type: &tg.InlineButtonTypeGame{}}), ErrButtonUnsupported},
		"buy":  {inlineKeyboard(tg.KeyboardInlineButton{Text: "Buy", Type: &tg.InlineButtonTypeBuy{}}), ErrButtonUnsupported},
		"inline": {inlineKeyboard(tg.KeyboardInlineButton{
			Text: "Share", Type: &tg.InlineButtonTypeSwitchInline{Query: "q"},
		}), ErrButtonUnsupported},
		"request": {&tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButton{{
			Text: "Phone", Type: &tg.ButtonTypeRequestPhone{},
		}}}}}, ErrButtonUnsupported},
		"reply not found": {&tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButton{{
			Text: "Yes", Type: &tg.ButtonTypeDefault{},
		}}}}}, ErrButtonNotFound},
		"no keyboard": {nil, ErrNoButtons},
	}

	for name, tc := range cases {
		invoker := &botInvoker{message: messageWithMarkup(tc.markup)}

		column := 0
		if name == "reply not found" {
			column = 3
		}

		_, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), botPeer, 10, 0, column)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}

		if len(invoker.callbacks) != 0 || len(invoker.sends) != 0 {
			t.Errorf("%s: a refused press reached the server", name)
		}
	}
}

// A reply-keyboard press in a forum topic must post into that topic, not to
// General.
func TestPressButton_ReplyKeyboardStaysInTheTopic(t *testing.T) {
	msg := messageWithMarkup(&tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButton{{
		Text: "Yes", Type: &tg.ButtonTypeDefault{},
	}}}}})
	header := &tg.MessageReplyHeader{ForumTopic: true}
	header.SetReplyToMsgID(40)
	header.SetReplyToTopID(40)
	msg.ReplyTo = header

	invoker := &botInvoker{message: msg}

	press, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), botPeer, 10, 0, 0)
	if err != nil {
		t.Fatalf("PressButton: %v", err)
	}

	if len(invoker.sends) != 1 || invoker.sends[0].Message != "Yes" {
		t.Fatalf("sends = %+v, want the button text sent once", invoker.sends)
	}

	reply, ok := invoker.sends[0].ReplyTo.(*tg.InputReplyToMessage)
	if !ok {
		t.Fatalf("reply_to = %#v, want the topic", invoker.sends[0].ReplyTo)
	}

	if top, set := reply.GetTopMsgID(); !set || top != 40 {
		t.Errorf("top_msg_id = %d (set %v), want topic 40", top, set)
	}

	if press.SentMessageID != 77 {
		t.Errorf("sentMessageId = %d, want 77", press.SentMessageID)
	}
}

func replyKeyboardMessage(btn tg.KeyboardButton) *tg.Message {
	return messageWithMarkup(&tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{{Buttons: []tg.KeyboardButton{btn}}}})
}

// In a group the press goes out as a reply to the keyboard's message, as a
// client sends it: a bot in privacy mode never sees a bare text there. The
// topic is kept next to the reply target.
func TestPressButton_ReplyKeyboardInAGroupRepliesToTheKeyboard(t *testing.T) {
	msg := replyKeyboardMessage(tg.KeyboardButton{Text: "Yes", Type: &tg.ButtonTypeDefault{}})
	msg.PeerID = &tg.PeerChat{ChatID: 7}
	header := &tg.MessageReplyHeader{ForumTopic: true}
	header.SetReplyToMsgID(40)
	header.SetReplyToTopID(40)
	msg.ReplyTo = header

	invoker := &botInvoker{message: msg}
	group := InputPeer{Type: PeerChat, ID: 7}

	_, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), group, 10, 0, 0)
	if err != nil {
		t.Fatalf("PressButton: %v", err)
	}

	if len(invoker.sends) != 1 {
		t.Fatalf("sends = %+v, want the button text sent once", invoker.sends)
	}

	reply, ok := invoker.sends[0].ReplyTo.(*tg.InputReplyToMessage)
	if !ok {
		t.Fatalf("reply_to = %#v, want a reply to the keyboard message", invoker.sends[0].ReplyTo)
	}

	if reply.ReplyToMsgID != 10 {
		t.Errorf("reply_to_msg_id = %d, want the keyboard message 10", reply.ReplyToMsgID)
	}

	if top, set := reply.GetTopMsgID(); !set || top != 40 {
		t.Errorf("top_msg_id = %d (set %v), want topic 40", top, set)
	}
}

// In a private chat with the bot every message reaches it, so the press is
// a plain message with no reply target, as a client sends it.
func TestPressButton_ReplyKeyboardInAPrivateChatHasNoReplyTarget(t *testing.T) {
	invoker := &botInvoker{message: replyKeyboardMessage(tg.KeyboardButton{Text: "Yes", Type: &tg.ButtonTypeDefault{}})}

	_, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), botPeer, 10, 0, 0)
	if err != nil {
		t.Fatalf("PressButton: %v", err)
	}

	if len(invoker.sends) != 1 || invoker.sends[0].ReplyTo != nil {
		t.Errorf("sends = %+v, want one send with no reply target", invoker.sends)
	}
}

// A web-app button on a reply keyboard returns its URL, like an inline one,
// and sends nothing.
func TestPressButton_ReplyKeyboardWebAppReturnsTheURL(t *testing.T) {
	invoker := &botInvoker{message: replyKeyboardMessage(tg.KeyboardButton{
		Text: "Open", Type: &tg.ButtonTypeSimpleWebView{URL: "https://app.example/x"},
	})}

	press, err := newWrapperWithInvoker(invoker).PressButton(t.Context(), botPeer, 10, 0, 0)
	if err != nil {
		t.Fatalf("PressButton: %v", err)
	}

	if press.Kind != ButtonKindWebView || press.URL != "https://app.example/x" || len(invoker.sends) != 0 {
		t.Errorf("press = %+v, sends = %d; want kind webview with the URL and nothing sent", press, len(invoker.sends))
	}
}

func TestGetBotInfo_RefusesWhatIsNotABot(t *testing.T) {
	invoker := &botInvoker{fullUser: &tg.UserFull{ID: 5}}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.GetBotInfo(t.Context(), InputPeer{Type: PeerChannel, ID: 5, AccessHash: 1})
	if !errors.Is(err, ErrNotABot) {
		t.Errorf("channel: err = %v, want ErrNotABot", err)
	}

	_, err = wrap.GetBotInfo(t.Context(), botPeer)
	if !errors.Is(err, ErrNotABot) {
		t.Errorf("user without bot_info: err = %v, want ErrNotABot", err)
	}
}

func TestGetBotInfo_ReadsTheBotInfo(t *testing.T) {
	full := &tg.UserFull{ID: 5}
	full.SetBotInfo(tg.BotInfo{Description: "helps", Commands: []tg.BotCommand{{Command: "start", Description: "begin"}}})

	info, err := newWrapperWithInvoker(&botInvoker{fullUser: full}).GetBotInfo(t.Context(), botPeer)
	if err != nil || info.Description != "helps" || len(info.Commands) != 1 || info.Commands[0].Command != "start" {
		t.Errorf("info = %+v, err = %v", info, err)
	}
}

func TestPolls_RefuseAMessageWithoutAPoll(t *testing.T) {
	invoker := &botInvoker{message: &tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 5}, Message: "text"}}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.GetPoll(t.Context(), botPeer, 10)
	if !errors.Is(err, ErrNotAPoll) {
		t.Errorf("GetPoll: err = %v, want ErrNotAPoll", err)
	}

	err = wrap.VotePoll(t.Context(), botPeer, 10, []int{0})
	if !errors.Is(err, ErrNotAPoll) || len(invoker.votes) != 0 {
		t.Errorf("VotePoll: err = %v, votes sent = %d; want ErrNotAPoll and no vote", err, len(invoker.votes))
	}
}

// Min results leave out which options the account chose.
func TestConvertPoll_MinResultsMarkTheVoteUnknown(t *testing.T) {
	media := &tg.MessageMediaPoll{
		Poll:    tg.Poll{Question: tg.TextWithEntities{Text: "q"}},
		Results: tg.PollResults{Min: true},
	}

	if !convertPoll(media).VoteUnknown {
		t.Error("min results must set VoteUnknown")
	}
}

func minPollMessage() *tg.Message {
	return &tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 5}, Media: &tg.MessageMediaPoll{
		Poll: tg.Poll{ID: 3, Question: tg.TextWithEntities{Text: "q"}, Answers: []tg.PollAnswerClass{
			&tg.PollAnswer{Text: tg.TextWithEntities{Text: "a"}, Option: []byte("0")},
			&tg.PollAnswer{Text: tg.TextWithEntities{Text: "b"}, Option: []byte("1")},
		}},
		Results: tg.PollResults{Min: true},
	}}
}

// Min results are replaced by the full ones from messages.getPollResults,
// which say which option the account chose.
func TestGetPoll_MinResultsFetchTheFullOnes(t *testing.T) {
	voters := tg.PollAnswerVoters{Option: []byte("1"), Chosen: true}
	voters.SetVoters(3)

	var full tg.PollResults
	full.SetTotalVoters(3)
	full.SetResults([]tg.PollAnswerVoters{voters})

	invoker := &botInvoker{message: minPollMessage(), pollResults: &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateMessagePoll{PollID: 3, Results: full},
	}}}

	poll, err := newWrapperWithInvoker(invoker).GetPoll(t.Context(), botPeer, 10)
	if err != nil {
		t.Fatalf("GetPoll: %v", err)
	}

	if invoker.resultReads != 1 || poll.VoteUnknown || !poll.Options[1].Chosen || poll.TotalVoters != 3 {
		t.Errorf("reads = %d, poll = %+v; want the full results with option 1 chosen", invoker.resultReads, poll)
	}
}

// A failed results read leaves the poll readable, marked VoteUnknown.
func TestGetPoll_MinResultsStayUnknownWhenTheFullReadFails(t *testing.T) {
	invoker := &botInvoker{message: minPollMessage()}

	poll, err := newWrapperWithInvoker(invoker).GetPoll(t.Context(), botPeer, 10)
	if err != nil {
		t.Fatalf("GetPoll: %v", err)
	}

	if invoker.resultReads != 1 || !poll.VoteUnknown {
		t.Errorf("reads = %d, voteUnknown = %v; want one results read and the vote marked unknown",
			invoker.resultReads, poll.VoteUnknown)
	}
}

// The poll's ID comes from the echo by random_id, not from the first new
// message in it: a channel post can bundle its discussion-group copy.
func TestSendPoll_TakesItsIDByRandomID(t *testing.T) {
	invoker := &botInvoker{sendEcho: func(req *tg.MessagesSendMediaRequest) tg.UpdatesClass {
		stranger := &tg.Message{ID: 900, PeerID: &tg.PeerUser{UserID: 5}}
		ours := &tg.Message{ID: 31, PeerID: &tg.PeerUser{UserID: 5}}

		return &tg.Updates{Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 31, RandomID: req.RandomID},
			&tg.UpdateNewMessage{Message: stranger},
			&tg.UpdateNewMessage{Message: ours},
		}}
	}}

	msg, err := newWrapperWithInvoker(invoker).SendPoll(t.Context(), botPeer, &PollOpts{
		Question: "q", Options: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("SendPoll: %v", err)
	}

	if msg == nil || msg.ID != 31 {
		t.Errorf("message = %+v, want id 31 from the random_id pairing", msg)
	}
}
