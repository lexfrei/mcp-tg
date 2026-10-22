package telegram

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// Button kinds. ButtonKindReply is a reply-keyboard button: pressing it sends
// its text as a message. The others are inline buttons attached to the
// message itself.
const (
	ButtonKindCallback     = "callback"
	ButtonKindURL          = "url"
	ButtonKindWebView      = "webview"
	ButtonKindURLAuth      = "url_auth"
	ButtonKindCopy         = "copy"
	ButtonKindSwitchInline = "switch_inline"
	ButtonKindGame         = "game"
	ButtonKindBuy          = "buy"
	ButtonKindUserProfile  = "user_profile"
	ButtonKindReply        = "reply"
	ButtonKindRequest      = "request"
	ButtonKindOther        = "other"
)

// ErrNoButtons is returned for a message that carries no keyboard.
var ErrNoButtons = errors.New("message has no buttons")

// ErrButtonNotFound is returned when row/column point outside the keyboard.
var ErrButtonNotFound = errors.New("no button at that row and column")

// ErrButtonUnsupported is returned for a button kind that cannot be pressed
// from a user account here (games, payments, switch-inline, requests).
var ErrButtonUnsupported = errors.New("pressing this kind of button is not supported")

// ErrButtonNeedsPassword is returned for a callback button that asks for the
// account's 2FA password (bot ownership transfer and the like).
var ErrButtonNeedsPassword = errors.New("this button requires the account's 2FA password and is not pressed by this server")

// ErrNotABot is returned when bot information is requested for a non-bot.
var ErrNotABot = errors.New("peer is not a bot")

// MessageButton is one button of a message's keyboard, addressed by its
// zero-based row and column.
type MessageButton struct {
	Row              int    `json:"row"`
	Column           int    `json:"column"`
	Text             string `json:"text"`
	Kind             string `json:"kind"`
	URL              string `json:"url,omitempty"`
	CopyText         string `json:"copyText,omitempty"`
	RequiresPassword bool   `json:"requiresPassword,omitempty"`
}

// ButtonPress is the outcome of pressing a button. A callback button returns
// the bot's answer (Message, Alert, URL); a link button returns its URL
// without opening it; a copy button returns the text; a reply-keyboard button
// sends its text and returns the new message's ID.
type ButtonPress struct {
	Kind          string `json:"kind"`
	Message       string `json:"message,omitempty"`
	Alert         bool   `json:"alert,omitempty"`
	URL           string `json:"url,omitempty"`
	CopyText      string `json:"copyText,omitempty"`
	SentMessageID int    `json:"sentMessageId,omitempty"`
}

// BotCommand is one entry of a bot's command menu.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// BotInfo describes a bot.
type BotInfo struct {
	UserID           int64        `json:"userId"`
	Description      string       `json:"description,omitempty"`
	Commands         []BotCommand `json:"commands,omitempty"`
	PrivacyPolicyURL string       `json:"privacyPolicyUrl,omitempty"`
}

// GetMessageButtons returns the buttons of a message, row by row.
func (w *Wrapper) GetMessageButtons(ctx context.Context, peer InputPeer, msgID int) ([]MessageButton, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, err
	}

	buttons := convertButtons(raw.ReplyMarkup)
	if len(buttons) == 0 {
		return nil, ErrNoButtons
	}

	return buttons, nil
}

// PressButton presses the button at row/column of a message. The message is
// read again first, so the press always uses the keyboard as it is now — a
// bot that edited its buttons since they were listed does not get yesterday's
// callback data.
func (w *Wrapper) PressButton(ctx context.Context, peer InputPeer, msgID, row, column int) (*ButtonPress, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, err
	}

	switch markup := raw.ReplyMarkup.(type) {
	case *tg.ReplyInlineMarkup:
		btn, found := inlineButtonAt(markup, row, column)
		if !found {
			return nil, ErrButtonNotFound
		}

		return w.pressInline(ctx, peer, msgID, btn)
	case *tg.ReplyKeyboardMarkup:
		btn, found := replyButtonAt(markup, row, column)
		if !found {
			return nil, ErrButtonNotFound
		}

		return w.pressReply(ctx, peer, btn)
	default:
		return nil, ErrNoButtons
	}
}

// GetBotInfo returns a bot's description and command menu.
func (w *Wrapper) GetBotInfo(ctx context.Context, peer InputPeer) (*BotInfo, error) {
	if peer.Type != PeerUser {
		return nil, ErrNotABot
	}

	full, err := w.api.UsersGetFullUser(ctx, InputUserFromPeer(peer))
	if err != nil {
		return nil, errors.Wrap(err, "getting bot info")
	}

	info, ok := full.FullUser.GetBotInfo()
	if !ok {
		return nil, ErrNotABot
	}

	commands := make([]BotCommand, 0, len(info.Commands))
	for _, cmd := range info.Commands {
		commands = append(commands, BotCommand{Command: cmd.Command, Description: cmd.Description})
	}

	return &BotInfo{
		UserID:           peer.ID,
		Description:      info.Description,
		Commands:         commands,
		PrivacyPolicyURL: info.PrivacyPolicyURL,
	}, nil
}

func (w *Wrapper) pressInline(
	ctx context.Context, peer InputPeer, msgID int, btn *tg.KeyboardInlineButton,
) (*ButtonPress, error) {
	switch kind := btn.Type.(type) {
	case *tg.InlineButtonTypeCallback:
		if kind.RequiresPassword {
			return nil, ErrButtonNeedsPassword
		}

		answer, err := w.api.MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
			Peer:  InputPeerToTG(peer),
			MsgID: msgID,
			Data:  kind.Data,
		})
		if err != nil {
			return nil, errors.Wrap(err, "pressing callback button")
		}

		return &ButtonPress{
			Kind:    ButtonKindCallback,
			Message: answer.Message,
			Alert:   answer.Alert,
			URL:     answer.URL,
		}, nil
	case *tg.InlineButtonTypeCopy:
		return &ButtonPress{Kind: ButtonKindCopy, CopyText: kind.CopyText}, nil
	default:
		converted := convertInlineButton(btn, 0, 0)
		if converted.URL == "" {
			return nil, errors.Wrapf(ErrButtonUnsupported, "kind %s", converted.Kind)
		}

		return &ButtonPress{Kind: converted.Kind, URL: converted.URL}, nil
	}
}

func (w *Wrapper) pressReply(ctx context.Context, peer InputPeer, btn *tg.KeyboardButton) (*ButtonPress, error) {
	if _, plain := btn.Type.(*tg.ButtonTypeDefault); !plain && btn.Type != nil {
		return nil, errors.Wrapf(ErrButtonUnsupported, "kind %s", ButtonKindRequest)
	}

	msg, err := w.SendMessage(ctx, peer, btn.Text, SendOpts{ParseMode: ParseModePlain})
	if err != nil {
		return nil, err
	}

	press := &ButtonPress{Kind: ButtonKindReply}
	if msg != nil {
		press.SentMessageID = msg.ID
	}

	return press, nil
}

func inlineButtonAt(markup *tg.ReplyInlineMarkup, row, column int) (*tg.KeyboardInlineButton, bool) {
	if row < 0 || row >= len(markup.Rows) || column < 0 || column >= len(markup.Rows[row].Buttons) {
		return nil, false
	}

	return &markup.Rows[row].Buttons[column], true
}

func replyButtonAt(markup *tg.ReplyKeyboardMarkup, row, column int) (*tg.KeyboardButton, bool) {
	if row < 0 || row >= len(markup.Rows) || column < 0 || column >= len(markup.Rows[row].Buttons) {
		return nil, false
	}

	return &markup.Rows[row].Buttons[column], true
}

// convertButtons flattens a message's keyboard. A message without one, or
// with a markup that only hides or forces a reply keyboard, has no buttons.
func convertButtons(markup tg.ReplyMarkupClass) []MessageButton {
	var buttons []MessageButton

	switch typed := markup.(type) {
	case *tg.ReplyInlineMarkup:
		for row := range typed.Rows {
			for column := range typed.Rows[row].Buttons {
				buttons = append(buttons, convertInlineButton(&typed.Rows[row].Buttons[column], row, column))
			}
		}
	case *tg.ReplyKeyboardMarkup:
		for row := range typed.Rows {
			for column := range typed.Rows[row].Buttons {
				buttons = append(buttons, convertReplyButton(&typed.Rows[row].Buttons[column], row, column))
			}
		}
	}

	return buttons
}

func convertInlineButton(btn *tg.KeyboardInlineButton, row, column int) MessageButton {
	out := MessageButton{Row: row, Column: column, Text: btn.Text, Kind: ButtonKindOther}

	switch kind := btn.Type.(type) {
	case *tg.InlineButtonTypeCallback:
		out.Kind = ButtonKindCallback
		out.RequiresPassword = kind.RequiresPassword
	case *tg.InlineButtonTypeURL:
		out.Kind, out.URL = ButtonKindURL, kind.URL
	case *tg.InlineButtonTypeWebView:
		out.Kind, out.URL = ButtonKindWebView, kind.URL
	case *tg.InlineButtonTypeURLAuth:
		out.Kind, out.URL = ButtonKindURLAuth, kind.URL
	case *tg.InlineButtonTypeCopy:
		out.Kind, out.CopyText = ButtonKindCopy, kind.CopyText
	default:
		out.Kind = otherInlineKind(btn.Type)
	}

	return out
}

func otherInlineKind(kind tg.InlineButtonTypeClass) string {
	switch kind.(type) {
	case *tg.InlineButtonTypeSwitchInline:
		return ButtonKindSwitchInline
	case *tg.InlineButtonTypeGame:
		return ButtonKindGame
	case *tg.InlineButtonTypeBuy:
		return ButtonKindBuy
	case *tg.InlineButtonTypeUserProfile:
		return ButtonKindUserProfile
	default:
		return ButtonKindOther
	}
}

func convertReplyButton(btn *tg.KeyboardButton, row, column int) MessageButton {
	kind := ButtonKindReply
	if _, plain := btn.Type.(*tg.ButtonTypeDefault); !plain && btn.Type != nil {
		kind = ButtonKindRequest
	}

	return MessageButton{Row: row, Column: column, Text: btn.Text, Kind: kind}
}
