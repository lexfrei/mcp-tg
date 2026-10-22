package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const minPollOptions = 2

// ErrPollQuestionRequired is returned when a poll has no question.
var ErrPollQuestionRequired = errors.New("question is required")

// ErrPollTooFewOptions is returned when a poll has fewer than two options.
var ErrPollTooFewOptions = errors.New("a poll needs at least 2 options")

// ErrQuizCorrectOption is returned when a quiz's correctOption is not one of its options.
var ErrQuizCorrectOption = errors.New("correctOption must be the index of one of the options")

// ErrQuizMultipleChoice is returned for a quiz that also allows several answers.
var ErrQuizMultipleChoice = errors.New("a quiz has exactly one correct answer and cannot be multiple choice")

// PollsCreateParams defines the parameters for the tg_polls_create tool.
type PollsCreateParams struct {
	Peer           string   `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	Question       string   `json:"question"                 jsonschema:"Poll question"`
	Options        []string `json:"options"                  jsonschema:"Answer options, at least 2"`
	MultipleChoice *bool    `json:"multipleChoice,omitempty" jsonschema:"Allow choosing several options (default false)"`
	Quiz           *bool    `json:"quiz,omitempty"           jsonschema:"Make it a quiz with one correct answer (default false)"`
	CorrectOption  int      `json:"correctOption,omitempty"  jsonschema:"Zero-based index of the correct option (quiz only)"`
	PublicVoters   *bool    `json:"publicVoters,omitempty"   jsonschema:"Show who voted for what (default false: anonymous)"`
	ClosePeriod    int      `json:"closePeriod,omitempty"    jsonschema:"Close the poll automatically after this many seconds"`
}

// PollsCreateResult is the output of the tg_polls_create tool.
type PollsCreateResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewPollsCreateHandler creates a handler for the tg_polls_create tool.
func NewPollsCreateHandler(client telegram.Client) mcp.ToolHandlerFor[PollsCreateParams, PollsCreateResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params PollsCreateParams,
	) (*mcp.CallToolResult, PollsCreateResult, error) {
		opts, err := pollOptsFromParams(&params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsCreateResult{}, validationErr(err)
		}

		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, PollsCreateResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsCreateResult{},
				telegramErr("failed to resolve peer", err)
		}

		msg, err := client.SendPoll(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsCreateResult{},
				telegramErr("failed to send poll", err)
		}

		result := PollsCreateResult{Output: "Poll sent"}
		if msg != nil {
			result.MessageID = msg.ID
			result.Output = fmt.Sprintf("Poll sent as message %d", msg.ID)
		}

		return nil, result, nil
	}
}

func pollOptsFromParams(params *PollsCreateParams) (*telegram.PollOpts, error) {
	if strings.TrimSpace(params.Question) == "" {
		return nil, ErrPollQuestionRequired
	}

	if len(params.Options) < minPollOptions {
		return nil, ErrPollTooFewOptions
	}

	opts := &telegram.PollOpts{
		Question:       params.Question,
		Options:        params.Options,
		MultipleChoice: deref(params.MultipleChoice),
		Quiz:           deref(params.Quiz),
		CorrectOption:  params.CorrectOption,
		PublicVoters:   deref(params.PublicVoters),
		ClosePeriod:    params.ClosePeriod,
	}

	if opts.Quiz && opts.MultipleChoice {
		return nil, ErrQuizMultipleChoice
	}

	if opts.Quiz && (opts.CorrectOption < 0 || opts.CorrectOption >= len(opts.Options)) {
		return nil, ErrQuizCorrectOption
	}

	return opts, nil
}

// PollsCreateTool returns the MCP tool definition for tg_polls_create.
func PollsCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_polls_create",
		Description: "Send a poll or quiz to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

// PollsGetParams defines the parameters for the tg_polls_get tool.
type PollsGetParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message carrying the poll"`
}

// PollsGetResult is the output of the tg_polls_get tool.
type PollsGetResult struct {
	telegram.Poll

	Output string `json:"output"`
}

// NewPollsGetHandler creates a handler for the tg_polls_get tool.
func NewPollsGetHandler(client telegram.Client) mcp.ToolHandlerFor[PollsGetParams, PollsGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params PollsGetParams,
	) (*mcp.CallToolResult, PollsGetResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsGetResult{}, err
		}

		poll, err := client.GetPoll(ctx, peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsGetResult{}, telegramErr("failed to read the poll", err)
		}

		return nil, PollsGetResult{Poll: *poll, Output: formatPoll(poll)}, nil
	}
}

// PollsGetTool returns the MCP tool definition for tg_polls_get.
func PollsGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_polls_get",
		Description: "Read a poll in a Telegram message: question, options with their indexes, and the results the account may see",
		Annotations: readOnlyAnnotations(),
	}
}

// PollsVoteParams defines the parameters for the tg_polls_vote tool.
type PollsVoteParams struct {
	Peer      string `json:"peer"              jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId"         jsonschema:"ID of the message carrying the poll"`
	Options   []int  `json:"options,omitempty" jsonschema:"Zero-based option indexes from tg_polls_get; empty retracts the vote"`
}

// PollsVoteResult is the output of the tg_polls_vote tool.
type PollsVoteResult struct {
	Output string `json:"output"`
}

// NewPollsVoteHandler creates a handler for the tg_polls_vote tool.
func NewPollsVoteHandler(client telegram.Client) mcp.ToolHandlerFor[PollsVoteParams, PollsVoteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params PollsVoteParams,
	) (*mcp.CallToolResult, PollsVoteResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsVoteResult{}, err
		}

		err = client.VotePoll(ctx, peer, params.MessageID, params.Options)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, PollsVoteResult{}, telegramErr("failed to vote", err)
		}

		if len(params.Options) == 0 {
			return nil, PollsVoteResult{Output: fmt.Sprintf("Retracted the vote in poll %d", params.MessageID)}, nil
		}

		return nil, PollsVoteResult{
			Output: fmt.Sprintf("Voted for option(s) %v in poll %d", params.Options, params.MessageID),
		}, nil
	}
}

// PollsVoteTool returns the MCP tool definition for tg_polls_vote.
func PollsVoteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_polls_vote",
		Description: "Vote in a Telegram poll by option index, or retract the vote with an empty list",
		Annotations: idempotentAnnotations(),
	}
}

func formatPoll(poll *telegram.Poll) string {
	var buf strings.Builder

	kind := "Poll"
	if poll.Quiz {
		kind = "Quiz"
	}

	fmt.Fprintf(&buf, "%s %d: %s (%d voter(s)", kind, poll.MessageID, poll.Question, poll.TotalVoters)

	if poll.Closed {
		buf.WriteString(", closed")
	}

	buf.WriteString(")")

	for _, option := range poll.Options {
		fmt.Fprintf(&buf, "\n[%d] %s — %d", option.Index, option.Text, option.Voters)

		if option.Chosen {
			buf.WriteString(" (your vote)")
		}

		if option.Correct {
			buf.WriteString(" (correct)")
		}
	}

	return buf.String()
}
