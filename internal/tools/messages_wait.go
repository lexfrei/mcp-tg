package tools

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultWaitTimeout = 60
	maxWaitTimeout     = 300

	// waitPollInterval re-reads the history even without a wake-up: the
	// update stream is the fast path, not a guarantee, and a gap in it must
	// cost a short delay rather than a missed message.
	waitPollInterval = 15 * time.Second
)

// ErrWaitTimeoutRange is returned when timeoutSeconds is outside 1..300.
var ErrWaitTimeoutRange = errors.New("timeoutSeconds must be between 1 and 300")

// MessageListener wakes a waiter when a new message arrives in a peer.
// *telegram.SubscriptionBroker is the implementation.
type MessageListener interface {
	Listen(peer telegram.InputPeer) telegram.Listener
}

// MessagesWaitParams defines the parameters for the tg_messages_wait tool.
type MessagesWaitParams struct {
	Peer           string `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	AfterMessageID int    `json:"afterMessageId,omitempty" jsonschema:"Return messages newer than this ID (default: newest at call time)"`
	From           string `json:"from,omitempty"           jsonschema:"Only count messages from this sender (@username or numeric ID)"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty" jsonschema:"How long to wait, 1-300 seconds (default 60)"`
	IncludeOwn     *bool  `json:"includeOwn,omitempty"     jsonschema:"Also count this account's own messages (default false)"`
}

// MessagesWaitResult is the output of the tg_messages_wait tool.
type MessagesWaitResult struct {
	Count         int               `json:"count"`
	TimedOut      bool              `json:"timedOut"`
	LastMessageID int               `json:"lastMessageId"`
	Participants  []ParticipantItem `json:"participants,omitempty"`
	Messages      []MessageItem     `json:"messages,omitempty"`
	Output        string            `json:"output"`
}

// waitFilter decides which new messages end the wait.
type waitFilter struct {
	from       *telegram.InputPeer
	includeOwn bool
	selfID     int64
}

// NewMessagesWaitHandler creates a handler for the tg_messages_wait tool.
func NewMessagesWaitHandler(
	client telegram.Client, listener MessageListener,
) mcp.ToolHandlerFor[MessagesWaitParams, MessagesWaitResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesWaitParams,
	) (*mcp.CallToolResult, MessagesWaitResult, error) {
		timeout, err := waitTimeout(params.TimeoutSeconds)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{}, validationErr(err)
		}

		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{},
				telegramErr("failed to resolve peer", err)
		}

		filter, err := newWaitFilter(ctx, client, params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{}, err
		}

		// Listen BEFORE the first read: a message landing between the read
		// and the wait would otherwise go unnoticed until the next poll.
		listening := listener.Listen(peer)
		defer listening.Stop()

		after := params.AfterMessageID
		if after == 0 {
			after, err = newestMessageID(ctx, client, peer)
			if err != nil {
				return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{},
					telegramErr("failed to read the chat", err)
			}
		}

		result, err := waitForMessages(ctx, client, peer, after, &filter, listening.Signal, timeout)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesWaitResult{}, err
		}

		return nil, result, nil
	}
}

func waitTimeout(seconds int) (time.Duration, error) {
	if seconds == 0 {
		return defaultWaitTimeout * time.Second, nil
	}

	if seconds < 1 || seconds > maxWaitTimeout {
		return 0, ErrWaitTimeoutRange
	}

	return time.Duration(seconds) * time.Second, nil
}

func newWaitFilter(ctx context.Context, client telegram.Client, params MessagesWaitParams) (waitFilter, error) {
	from, err := resolveOptionalPeer(ctx, client, params.From, "from")
	if err != nil {
		return waitFilter{}, err
	}

	filter := waitFilter{from: from, includeOwn: deref(params.IncludeOwn)}
	if filter.includeOwn {
		return filter, nil
	}

	self, err := client.GetSelf(ctx)
	if err != nil {
		return waitFilter{}, telegramErr("failed to identify the account", err)
	}

	if self != nil {
		filter.selfID = self.ID
	}

	return filter, nil
}

func (filter *waitFilter) admits(msg *telegram.Message) bool {
	if filter.from != nil && (msg.FromID != filter.from.ID || msg.FromType != filter.from.Type) {
		return false
	}

	ownMessage := msg.FromType == telegram.PeerUser && msg.FromID == filter.selfID

	return filter.includeOwn || !ownMessage
}

func newestMessageID(ctx context.Context, client telegram.Client, peer telegram.InputPeer) (int, error) {
	msgs, _, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{Limit: 1, SinglePage: true})
	if err != nil {
		return 0, errors.Wrap(err, "reading the newest message")
	}

	if len(msgs) == 0 {
		return 0, nil
	}

	return msgs[0].ID, nil
}

// waitForMessages re-reads the chat on every wake-up and poll tick until a
// message newer than after passes the filter, or the timeout runs out.
func waitForMessages(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, after int,
	filter *waitFilter, signal <-chan struct{}, timeout time.Duration,
) (MessagesWaitResult, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(waitPollInterval)
	defer ticker.Stop()

	for {
		msgs, err := newMessages(ctx, client, peer, after, filter)
		if err != nil {
			return MessagesWaitResult{}, telegramErr("failed to read new messages", err)
		}

		if len(msgs) > 0 {
			return buildMessagesWaitResult(msgs), nil
		}

		select {
		case <-ctx.Done():
			return MessagesWaitResult{}, errors.Wrap(ctx.Err(), "waiting for messages")
		case <-deadline.C:
			return MessagesWaitResult{
				TimedOut:      true,
				LastMessageID: after,
				Output:        fmt.Sprintf("No new messages after %d within %s", after, timeout),
			}, nil
		case <-signal:
		case <-ticker.C:
		}
	}
}

// newMessages returns the messages newer than after that pass the filter,
// oldest first. One page of up to 100 is read: a wait reports arrivals, and
// a caller that fell further behind should page with tg_messages_list.
func newMessages(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, after int, filter *waitFilter,
) ([]telegram.Message, error) {
	msgs, _, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{Limit: telegram.DefaultLimit, SinglePage: true})
	if err != nil {
		return nil, errors.Wrap(err, "reading history")
	}

	fresh := make([]telegram.Message, 0, len(msgs))

	for idx := range msgs {
		if msgs[idx].ID > after && filter.admits(&msgs[idx]) {
			fresh = append(fresh, msgs[idx])
		}
	}

	slices.SortFunc(fresh, func(left, right telegram.Message) int { return left.ID - right.ID })

	return fresh, nil
}

func buildMessagesWaitResult(msgs []telegram.Message) MessagesWaitResult {
	return MessagesWaitResult{
		Count:         len(msgs),
		LastMessageID: msgs[len(msgs)-1].ID,
		Participants:  participantsFromMessages(msgs),
		Messages:      messagesToItems(msgs),
		Output:        formatMessages(msgs),
	}
}

// MessagesWaitTool returns the MCP tool definition for tg_messages_wait.
func MessagesWaitTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_wait",
		Description: "Wait until a new message arrives in a Telegram chat, then return it. " +
			"To wait for a reply, pass the messageId of the message you sent as afterMessageId: " +
			"a quick reply can land before this call starts, and the default baseline would then skip it. " +
			"Pass the returned lastMessageId back to keep waiting without missing anything in between",
		Annotations: readOnlyAnnotations(),
	}
}
