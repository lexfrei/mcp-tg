package telegram

import (
	"bytes"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func TestConvertButtons_InlineKeyboard(t *testing.T) {
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{
			{Text: "Yes", Type: &tg.InlineButtonTypeCallback{Data: []byte("y")}},
			{Text: "Site", Type: &tg.InlineButtonTypeURL{URL: "https://example.org"}},
		}},
		{Buttons: []tg.KeyboardInlineButton{
			{Text: "Code", Type: &tg.InlineButtonTypeCopy{CopyText: "1234"}},
			{Text: "Play", Type: &tg.InlineButtonTypeGame{}},
		}},
	}}

	got := convertButtons(markup)

	want := []MessageButton{
		{Row: 0, Column: 0, Text: "Yes", Kind: ButtonKindCallback},
		{Row: 0, Column: 1, Text: "Site", Kind: ButtonKindURL, URL: "https://example.org"},
		{Row: 1, Column: 0, Text: "Code", Kind: ButtonKindCopy, CopyText: "1234"},
		{Row: 1, Column: 1, Text: "Play", Kind: ButtonKindGame},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d buttons, want %d: %+v", len(got), len(want), got)
	}

	for idx := range want {
		if got[idx] != want[idx] {
			t.Errorf("button %d = %+v, want %+v", idx, got[idx], want[idx])
		}
	}
}

func TestConvertButtons_ReplyKeyboard(t *testing.T) {
	markup := &tg.ReplyKeyboardMarkup{Rows: []tg.KeyboardButtonRow{
		{Buttons: []tg.KeyboardButton{
			{Text: "Menu", Type: &tg.ButtonTypeDefault{}},
			{Text: "Share phone", Type: &tg.ButtonTypeRequestPhone{}},
		}},
	}}

	got := convertButtons(markup)

	if len(got) != 2 || got[0].Kind != ButtonKindReply || got[1].Kind != ButtonKindRequest {
		t.Fatalf("got %+v, want a reply button then a request button", got)
	}
}

// A markup that only hides the keyboard or forces a reply carries no
// buttons, and must not be reported as an empty keyboard.
func TestConvertButtons_NoKeyboard(t *testing.T) {
	for _, markup := range []tg.ReplyMarkupClass{nil, &tg.ReplyKeyboardHide{}, &tg.ReplyKeyboardForceReply{}} {
		if got := convertButtons(markup); len(got) != 0 {
			t.Errorf("convertButtons(%T) = %+v, want none", markup, got)
		}
	}
}

func TestInlineButtonAt_RejectsOutOfRange(t *testing.T) {
	markup := &tg.ReplyInlineMarkup{Rows: []tg.KeyboardInlineButtonRow{
		{Buttons: []tg.KeyboardInlineButton{{Text: "only"}}},
	}}

	for _, pos := range [][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 1}} {
		if _, found := inlineButtonAt(markup, pos[0], pos[1]); found {
			t.Errorf("inlineButtonAt(%d, %d) found a button", pos[0], pos[1])
		}
	}

	if btn, found := inlineButtonAt(markup, 0, 0); !found || btn.Text != "only" {
		t.Error("inlineButtonAt(0, 0) missed the only button")
	}
}

func samplePollMedia() *tg.MessageMediaPoll {
	return &tg.MessageMediaPoll{
		Poll: tg.Poll{
			Question: tg.TextWithEntities{Text: "Lunch?"},
			Quiz:     true,
			Answers: []tg.PollAnswerClass{
				&tg.PollAnswer{Text: tg.TextWithEntities{Text: "Pizza"}, Option: []byte("a")},
				&tg.PollAnswer{Text: tg.TextWithEntities{Text: "Soup"}, Option: []byte("b")},
			},
		},
		Results: tg.PollResults{
			TotalVoters: 5,
			Results: []tg.PollAnswerVoters{
				{Option: []byte("b"), Voters: 3, Chosen: true, Correct: true},
				{Option: []byte("a"), Voters: 2},
			},
		},
	}
}

// Results are matched to answers by option bytes, not by position: the
// server is free to list them in another order.
func TestConvertPoll_MatchesResultsByOption(t *testing.T) {
	poll := convertPoll(samplePollMedia())

	if poll.Question != "Lunch?" || !poll.Quiz || poll.TotalVoters != 5 || len(poll.Options) != 2 {
		t.Fatalf("poll = %+v", poll)
	}

	pizza, soup := poll.Options[0], poll.Options[1]
	if pizza.Text != "Pizza" || pizza.Voters != 2 || pizza.Chosen {
		t.Errorf("pizza = %+v, want 2 voters, not chosen", pizza)
	}

	if soup.Text != "Soup" || soup.Voters != 3 || !soup.Chosen || !soup.Correct || soup.Index != 1 {
		t.Errorf("soup = %+v, want index 1, 3 voters, chosen and correct", soup)
	}
}

func TestPollOptionsAt(t *testing.T) {
	media := samplePollMedia()

	options, err := pollOptionsAt(&media.Poll, []int{1, 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(options) != 2 || !bytes.Equal(options[0], []byte("b")) || !bytes.Equal(options[1], []byte("a")) {
		t.Errorf("options = %q, want b then a", options)
	}

	_, err = pollOptionsAt(&media.Poll, []int{2})
	if !errors.Is(err, ErrPollOptionOutOfRange) {
		t.Errorf("err = %v, want ErrPollOptionOutOfRange", err)
	}
}

func TestInputMediaPoll(t *testing.T) {
	media := inputMediaPoll(&PollOpts{
		Question:      "Capital of France?",
		Options:       []string{"Lyon", "Paris"},
		Quiz:          true,
		CorrectOption: 1,
	})

	if media.Poll.Question.Text != "Capital of France?" || !media.Poll.Quiz {
		t.Fatalf("poll = %+v", media.Poll)
	}

	if len(media.CorrectAnswers) != 1 || media.CorrectAnswers[0] != 1 {
		t.Errorf("CorrectAnswers = %v, want [1]", media.CorrectAnswers)
	}

	seen := map[string]bool{}

	for _, class := range media.Poll.Answers {
		answer, ok := class.(*tg.PollAnswer)
		if !ok || len(answer.Option) == 0 || seen[string(answer.Option)] {
			t.Fatalf("answers must carry distinct non-empty options, got %+v", media.Poll.Answers)
		}

		seen[string(answer.Option)] = true
	}
}
