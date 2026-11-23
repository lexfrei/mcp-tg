package telegram

import (
	"bytes"
	"context"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// ErrNotAPoll is returned when a message carries no poll.
var ErrNotAPoll = errors.New("message is not a poll")

// ErrPollOptionOutOfRange is returned when a vote names an option index the
// poll does not have.
var ErrPollOptionOutOfRange = errors.New("poll option index out of range")

// PollOption is one answer of a poll. Index is what tg_polls_vote takes.
// Voters, Chosen and Correct are only known once the account may see results
// (it voted, or the poll is closed).
type PollOption struct {
	Index   int    `json:"index"`
	Text    string `json:"text"`
	Voters  int    `json:"voters"`
	Chosen  bool   `json:"chosen,omitempty"`
	Correct bool   `json:"correct,omitempty"`
}

// Poll is a poll as the account sees it.
type Poll struct {
	MessageID      int          `json:"messageId"`
	Question       string       `json:"question"`
	Options        []PollOption `json:"options"`
	TotalVoters    int          `json:"totalVoters"`
	Closed         bool         `json:"closed,omitempty"`
	Quiz           bool         `json:"quiz,omitempty"`
	MultipleChoice bool         `json:"multipleChoice,omitempty"`
	PublicVoters   bool         `json:"publicVoters,omitempty"`
	// VoteUnknown is set when Telegram sent only min results, which leave
	// out which options this account chose, and messages.getPollResults did
	// not return the full ones: no option being chosen then says nothing
	// about the account's vote.
	VoteUnknown bool `json:"voteUnknown,omitempty"`
}

// PollOpts describes a poll to create. CorrectOption is the zero-based index
// of the right answer and is required for a quiz and ignored otherwise.
type PollOpts struct {
	Question       string
	Options        []string
	MultipleChoice bool
	Quiz           bool
	CorrectOption  int
	PublicVoters   bool
	ClosePeriod    int
}

// SendPoll posts a new poll.
func (w *Wrapper) SendPoll(ctx context.Context, peer InputPeer, opts *PollOpts) (*Message, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	result, err := w.api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:     InputPeerToTG(peer),
		Media:    inputMediaPoll(opts),
		RandomID: randID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "sending poll")
	}

	return messageFromUpdate(result, peer, randID, nil), nil
}

// GetPoll reads the poll in a message, with the results the account may see.
func (w *Wrapper) GetPoll(ctx context.Context, peer InputPeer, msgID int) (*Poll, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, err
	}

	media, ok := raw.Media.(*tg.MessageMediaPoll)
	if !ok {
		return nil, ErrNotAPoll
	}

	// Min results leave out which options this account chose. Telegram
	// documents messages.getPollResults as the way to get the full ones; if
	// that read fails, the poll is still returned, marked VoteUnknown.
	if media.Results.Min {
		if full, ok := w.fullPollResults(ctx, peer, msgID, media.Poll.ID); ok {
			media.Results = full
		}
	}

	poll := convertPoll(media)
	poll.MessageID = msgID

	return poll, nil
}

// fullPollResults asks for a poll's results with messages.getPollResults.
// They arrive as an updateMessagePoll; ok is false when the call fails or
// the reply carries no non-min results for this poll.
func (w *Wrapper) fullPollResults(ctx context.Context, peer InputPeer, msgID int, pollID int64) (tg.PollResults, bool) {
	result, err := w.api.MessagesGetPollResults(ctx, &tg.MessagesGetPollResultsRequest{
		Peer:  InputPeerToTG(peer),
		MsgID: msgID,
	})
	if err != nil {
		return tg.PollResults{}, false
	}

	updates, _, _, ok := unwrapUpdates(result)
	if !ok {
		if short, isShort := result.(*tg.UpdateShort); isShort {
			updates = []tg.UpdateClass{short.Update}
		}
	}

	for _, update := range updates {
		pollUpdate, isPoll := update.(*tg.UpdateMessagePoll)
		if isPoll && pollUpdate.PollID == pollID && !pollUpdate.Results.Min {
			return pollUpdate.Results, true
		}
	}

	return tg.PollResults{}, false
}

// VotePoll votes for the options at the given indexes; an empty list retracts
// the account's vote.
func (w *Wrapper) VotePoll(ctx context.Context, peer InputPeer, msgID int, indexes []int) error {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return err
	}

	media, ok := raw.Media.(*tg.MessageMediaPoll)
	if !ok {
		return ErrNotAPoll
	}

	options, err := pollOptionsAt(&media.Poll, indexes)
	if err != nil {
		return err
	}

	_, err = w.api.MessagesSendVote(ctx, &tg.MessagesSendVoteRequest{
		Peer:    InputPeerToTG(peer),
		MsgID:   msgID,
		Options: options,
	})
	if err != nil {
		return errors.Wrap(err, "voting in poll")
	}

	return nil
}

func inputMediaPoll(opts *PollOpts) *tg.InputMediaPoll {
	answers := make([]tg.PollAnswerClass, len(opts.Options))
	for idx, text := range opts.Options {
		answers[idx] = &tg.PollAnswer{
			Text:   tg.TextWithEntities{Text: text},
			Option: pollOptionID(idx),
		}
	}

	media := &tg.InputMediaPoll{
		Poll: tg.Poll{
			Question:       tg.TextWithEntities{Text: opts.Question},
			Answers:        answers,
			MultipleChoice: opts.MultipleChoice,
			Quiz:           opts.Quiz,
			PublicVoters:   opts.PublicVoters,
			ClosePeriod:    opts.ClosePeriod,
		},
	}

	if opts.Quiz {
		media.CorrectAnswers = []int{opts.CorrectOption}
	}

	return media
}

// pollOptionID is the opaque option identifier a new poll's answer carries.
// Clients pick them; the official ones use the answer's position.
func pollOptionID(idx int) []byte {
	return []byte(strconv.Itoa(idx))
}

func pollOptionsAt(poll *tg.Poll, indexes []int) ([][]byte, error) {
	options := make([][]byte, 0, len(indexes))

	for _, idx := range indexes {
		if idx < 0 || idx >= len(poll.Answers) {
			return nil, errors.Wrapf(ErrPollOptionOutOfRange, "index %d of %d options", idx, len(poll.Answers))
		}

		answer, ok := poll.Answers[idx].(*tg.PollAnswer)
		if !ok {
			return nil, errors.Wrapf(ErrPollOptionOutOfRange, "option %d cannot be voted for", idx)
		}

		options = append(options, answer.Option)
	}

	return options, nil
}

func convertPoll(media *tg.MessageMediaPoll) *Poll {
	poll := &Poll{
		Question:       media.Poll.Question.Text,
		TotalVoters:    media.Results.TotalVoters,
		Closed:         media.Poll.Closed,
		Quiz:           media.Poll.Quiz,
		MultipleChoice: media.Poll.MultipleChoice,
		PublicVoters:   media.Poll.PublicVoters,
		VoteUnknown:    media.Results.Min,
		Options:        make([]PollOption, 0, len(media.Poll.Answers)),
	}

	for idx, class := range media.Poll.Answers {
		answer, ok := class.(*tg.PollAnswer)
		if !ok {
			continue
		}

		option := PollOption{Index: idx, Text: answer.Text.Text}

		for _, result := range media.Results.Results {
			if bytes.Equal(result.Option, answer.Option) {
				option.Voters = result.Voters
				option.Chosen = result.Chosen
				option.Correct = result.Correct
			}
		}

		poll.Options = append(poll.Options, option)
	}

	return poll
}
