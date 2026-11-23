package tools

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func botPeer() telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerUser, ID: 77, AccessHash: 1}
}

func TestMessagesGetButtons_ListsWithPositions(t *testing.T) {
	mock := &mockClient{
		peer: botPeer(),
		buttons: []telegram.MessageButton{
			{Row: 0, Column: 0, Text: "Yes", Kind: telegram.ButtonKindCallback},
			{Row: 0, Column: 1, Text: "Docs", Kind: telegram.ButtonKindURL, URL: "https://example.org"},
		},
	}

	_, result, err := NewMessagesGetButtonsHandler(mock)(context.Background(), nil,
		MessagesGetButtonsParams{Peer: "@bot", MessageID: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 2 || !strings.Contains(result.Output, "[0,1] Docs (url) https://example.org") {
		t.Errorf("result = %+v", result)
	}
}

func TestMessageTargetValidation(t *testing.T) {
	mock := &mockClient{}

	_, _, err := NewMessagesPressButtonHandler(mock)(context.Background(), nil, MessagesPressButtonParams{MessageID: 1})
	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("missing peer: err = %v, want ErrPeerRequired", err)
	}

	_, _, err = NewPollsGetHandler(mock)(context.Background(), nil, PollsGetParams{Peer: "@x"})
	if !errors.Is(err, ErrMessageIDRequired) {
		t.Errorf("missing message: err = %v, want ErrMessageIDRequired", err)
	}
}

func TestMessagesPressButton_PassesPositionAndReportsAnswer(t *testing.T) {
	mock := &mockClient{
		peer:  botPeer(),
		press: &telegram.ButtonPress{Kind: telegram.ButtonKindCallback, Message: "Saved!"},
	}

	_, result, err := NewMessagesPressButtonHandler(mock)(context.Background(), nil,
		MessagesPressButtonParams{Peer: "@bot", MessageID: 5, Row: 1, Column: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastPress != [2]int{1, 2} {
		t.Errorf("pressed %v, want row 1 column 2", mock.lastPress)
	}

	if result.Message != "Saved!" || result.Output != "Bot answered: Saved!" {
		t.Errorf("result = %+v", result)
	}
}

func TestPollsCreate_Validation(t *testing.T) {
	cases := map[string]struct {
		params PollsCreateParams
		want   error
	}{
		"no question": {PollsCreateParams{Peer: "@x", Options: []string{"a", "b"}}, ErrPollQuestionRequired},
		"one option":  {PollsCreateParams{Peer: "@x", Question: "q", Options: []string{"a"}}, ErrPollTooFewOptions},
		"quiz bad index": {
			PollsCreateParams{Peer: "@x", Question: "q", Options: []string{"a", "b"}, Quiz: new(true), CorrectOption: new(2)},
			ErrQuizCorrectOption,
		},
		// An omitted index must not read as 0 and mark the first option correct.
		"quiz without correctOption": {
			PollsCreateParams{Peer: "@x", Question: "q", Options: []string{"a", "b"}, Quiz: new(true)},
			ErrQuizCorrectOption,
		},
		"quiz multiple": {
			PollsCreateParams{Peer: "@x", Question: "q", Options: []string{"a", "b"}, Quiz: new(true), MultipleChoice: new(true)},
			ErrQuizMultipleChoice,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewPollsCreateHandler(&mockClient{})(context.Background(), nil, tc.params)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPollsCreate_SendsOptions(t *testing.T) {
	mock := &mockClient{peer: botPeer(), message: &telegram.Message{ID: 42}}

	_, result, err := NewPollsCreateHandler(mock)(context.Background(), nil, PollsCreateParams{
		Peer: "@chat", Question: "Lunch?", Options: []string{"Pizza", "Soup"}, PublicVoters: new(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MessageID != 42 {
		t.Errorf("MessageID = %d, want 42", result.MessageID)
	}

	opts := mock.lastPollOpts
	if opts == nil || opts.Question != "Lunch?" || !slices.Equal(opts.Options, []string{"Pizza", "Soup"}) || !opts.PublicVoters {
		t.Errorf("poll opts = %+v", opts)
	}
}

func TestPollsGet_FormatsResults(t *testing.T) {
	mock := &mockClient{peer: botPeer(), poll: &telegram.Poll{
		MessageID: 9, Question: "Lunch?", TotalVoters: 3,
		Options: []telegram.PollOption{{Index: 0, Text: "Pizza", Voters: 1}, {Index: 1, Text: "Soup", Voters: 2, Chosen: true}},
	}}

	_, result, err := NewPollsGetHandler(mock)(context.Background(), nil, PollsGetParams{Peer: "@chat", MessageID: 9})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Output, "[1] Soup — 2 (your vote)") || len(result.Options) != 2 {
		t.Errorf("result = %+v", result)
	}
}

func TestPollsVote_PassesIndexesAndRetracts(t *testing.T) {
	mock := &mockClient{peer: botPeer()}
	handler := NewPollsVoteHandler(mock)

	_, result, err := handler(context.Background(), nil, PollsVoteParams{Peer: "@chat", MessageID: 9, Options: []int{1}})
	if err != nil || !slices.Equal(mock.lastVote, []int{1}) {
		t.Fatalf("vote: err = %v, indexes = %v", err, mock.lastVote)
	}

	if !strings.Contains(result.Output, "Voted") {
		t.Errorf("output = %q", result.Output)
	}

	_, result, err = handler(context.Background(), nil, PollsVoteParams{Peer: "@chat", MessageID: 9, Options: []int{}})
	if err != nil || len(mock.lastVote) != 0 || !strings.Contains(result.Output, "Retracted") {
		t.Errorf("retract: err = %v, indexes = %v, output = %q", err, mock.lastVote, result.Output)
	}
}

// Leaving options out must not retract the vote: only an explicit [] does.
func TestPollsVote_OmittedOptionsAreRefused(t *testing.T) {
	mock := &mockClient{peer: botPeer(), lastVote: []int{7}}

	_, _, err := NewPollsVoteHandler(mock)(context.Background(), nil, PollsVoteParams{Peer: "@chat", MessageID: 9})
	if !errors.Is(err, ErrPollVoteOptionsRequired) || !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrPollVoteOptionsRequired as a validation error", err)
	}

	if !slices.Equal(mock.lastVote, []int{7}) {
		t.Errorf("an omitted options list reached VotePoll with %v", mock.lastVote)
	}

	schema, err := jsonschema.For[PollsVoteParams](nil)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	if !slices.Contains(schema.Required, "options") {
		t.Errorf("schema required = %v, want options in it", schema.Required)
	}
}

func TestBotsGetInfo(t *testing.T) {
	mock := &mockClient{peer: botPeer(), botInfo: &telegram.BotInfo{
		UserID: 77, Description: "Weather bot",
		Commands: []telegram.BotCommand{{Command: "today", Description: "Forecast for today"}},
	}}

	_, result, err := NewBotsGetInfoHandler(mock)(context.Background(), nil, BotsGetInfoParams{Peer: "@weather_bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Output, "/today — Forecast for today") || result.Description != "Weather bot" {
		t.Errorf("result = %+v", result)
	}
}

// BOT_RESPONSE_TIMEOUT means the bot got the press, so the explanation must
// steer the caller away from pressing again blindly.
func TestExplainMTProtoCode_BotResponseTimeout(t *testing.T) {
	got := explainMTProtoCode("rpc error code 400: BOT_RESPONSE_TIMEOUT")
	if !strings.Contains(got, "may already have acted") || !strings.Contains(got, "before pressing again") {
		t.Errorf("explanation = %q, want it to warn against a blind second press", got)
	}
}

// Min results leave the vote out, and the text must not read as "no vote".
func TestFormatPoll_SaysWhenTheVoteIsUnknown(t *testing.T) {
	out := formatPoll(&telegram.Poll{Question: "q", VoteUnknown: true, Options: []telegram.PollOption{{Text: "a"}}})
	if !strings.Contains(out, "does not show your vote") || strings.Contains(out, "read the poll again") {
		t.Errorf("output = %q, want the unknown-vote note", out)
	}
}
