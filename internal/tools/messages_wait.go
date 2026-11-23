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

	// waitMinReadGap is the least time between two history reads. The
	// listener coalesces a burst into one signal but does not limit how
	// often signals come: in a busy chat every message would cost a
	// getHistory, and a FLOOD_WAIT there stalls every tool on the connection.
	waitMinReadGap = 2 * time.Second
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
	Truncated     bool              `json:"truncated,omitempty"`
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

		wait := waiter{
			client: client, peer: peer, filter: &filter, signal: listening.Signal,
			timeout: timeout, poll: waitPollInterval, minGap: waitMinReadGap,
		}

		result, err := wait.fromBaseline(ctx, params.AfterMessageID)
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

// newestMessageID is the default baseline: the newest visible message of
// the same one-page read that newMessages makes. A one-message read would
// not do: service messages (a join, a pin, a call record) are dropped from
// the history, so a chat whose newest message is one would read as empty,
// the baseline would be 0, and the first wait read would hand back up to
// 100 OLD messages as new. When the whole page is service messages, 0 is
// safe: the read never pages, so a visible message can only enter that
// window by being newer than the page.
func newestMessageID(ctx context.Context, client telegram.Client, peer telegram.InputPeer) (int, error) {
	msgs, _, _, err := client.GetHistory(ctx, peer, waitHistoryOpts())
	if err != nil {
		return 0, errors.Wrap(err, "reading the newest messages")
	}

	newest := 0
	for idx := range msgs {
		newest = max(newest, msgs[idx].ID)
	}

	return newest, nil
}

func waitHistoryOpts() telegram.HistoryOpts {
	return telegram.HistoryOpts{Limit: telegram.DefaultLimit, SinglePage: true}
}

// waiter re-reads the chat on every wake-up and poll tick until a message
// newer than the baseline passes the filter, or the timeout runs out.
type waiter struct {
	client  telegram.Client
	peer    telegram.InputPeer
	filter  *waitFilter
	signal  <-chan struct{}
	timeout time.Duration
	poll    time.Duration
	minGap  time.Duration
}

// fromBaseline waits for messages newer than after, or, when after is 0,
// newer than the newest one at call time. The baseline read then doubles as
// the first look: nothing on it is newer than itself, and reading the same
// page again right away would break the minGap between reads.
func (wait *waiter) fromBaseline(ctx context.Context, after int) (MessagesWaitResult, error) {
	var baselineAt time.Time

	if after == 0 {
		baselineAt = time.Now()

		newest, err := newestMessageID(ctx, wait.client, wait.peer)
		if err != nil {
			return MessagesWaitResult{}, telegramErr("failed to read the chat", err)
		}

		after = newest
	}

	return wait.run(ctx, after, baselineAt)
}

// run waits for messages newer than after. A non-zero lastRead is the time
// of a read already made at that baseline, which stands in for the first.
func (wait *waiter) run(ctx context.Context, after int, lastRead time.Time) (MessagesWaitResult, error) {
	deadline := time.NewTimer(wait.timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(wait.poll)
	defer ticker.Stop()

	readAt, read, expired := lastRead, lastRead.IsZero(), false

	for {
		if read {
			readAt = time.Now()

			result, found, err := wait.look(ctx, after)
			if err != nil || found {
				return result, err
			}

			if expired {
				return wait.timedOut(after), nil
			}
		}

		read = true

		outcome, err := wait.rest(ctx, deadline.C, ticker.C, readAt)
		if err != nil {
			return MessagesWaitResult{}, err
		}

		if outcome == giveUp {
			return wait.timedOut(after), nil
		}

		expired = outcome == readLast
	}
}

// restOutcome is how a rest between reads ended.
type restOutcome int

const (
	readDue  restOutcome = iota // read again and keep waiting
	readLast                    // the timeout ran out during the pause: one last read is owed
	giveUp                      // the timeout ran out with no read owed
)

// rest blocks until the next read is due.
func (wait *waiter) rest(ctx context.Context, deadline, ticker <-chan time.Time, readAt time.Time) (restOutcome, error) {
	woken, err := wait.next(ctx, deadline, ticker)
	if err != nil {
		return giveUp, err
	}

	if !woken {
		return giveUp, nil
	}

	// A signal may come right after a read; hold the next read back so a
	// burst of messages costs one read per minGap, not one per message.
	expired, err := wait.pause(ctx, deadline, time.Until(readAt.Add(wait.minGap)))
	if expired {
		return readLast, err
	}

	return readDue, err
}

// look reads one page and reports whether it ends the wait. A truncated
// page does even when the filter rejects all of it: the awaited message may
// be among the unread older ones, and waiting on would report that none came.
func (wait *waiter) look(ctx context.Context, after int) (MessagesWaitResult, bool, error) {
	page, err := newMessages(ctx, wait.client, wait.peer, after, wait.filter)
	if err != nil {
		return MessagesWaitResult{}, false, telegramErr("failed to read new messages", err)
	}

	if len(page.fresh) == 0 && !page.truncated {
		return MessagesWaitResult{}, false, nil
	}

	return buildMessagesWaitResult(page), true, nil
}

// next blocks until a reason to read again. It reports false when the
// timeout ran out first.
func (wait *waiter) next(ctx context.Context, deadline, ticker <-chan time.Time) (bool, error) {
	select {
	case <-ctx.Done():
		return false, errors.Wrap(ctx.Err(), "waiting for messages")
	case <-deadline:
		return false, nil
	case <-wait.signal:
		return true, nil
	case <-ticker:
		return true, nil
	}
}

// pause sleeps for gap, cut short by cancellation or the deadline. It
// reports whether the deadline fired: its timer fires once, so the loop must
// be told, make its last read and stop, rather than wait on it again.
func (wait *waiter) pause(ctx context.Context, deadline <-chan time.Time, gap time.Duration) (bool, error) {
	if gap <= 0 {
		return false, nil
	}

	pause := time.NewTimer(gap)
	defer pause.Stop()

	select {
	case <-ctx.Done():
		return false, errors.Wrap(ctx.Err(), "waiting for messages")
	case <-deadline:
		return true, nil
	case <-pause.C:
		return false, nil
	}
}

func (wait *waiter) timedOut(after int) MessagesWaitResult {
	return MessagesWaitResult{
		TimedOut:      true,
		LastMessageID: after,
		Output:        fmt.Sprintf("No new messages after %d within %s", after, wait.timeout),
	}
}

// newPage is one wait read: the new messages that pass the filter, oldest
// first, whether newer-than-baseline messages may lie beyond the page, and
// the page's oldest and newest message IDs.
type newPage struct {
	fresh          []telegram.Message
	truncated      bool
	oldest, newest int
}

// newMessages reads one page of up to 100 messages. When more than that
// arrived since after, the page holds only the newest and the rest are
// reported as truncated, for the caller to page with tg_messages_list.
func newMessages(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, after int, filter *waitFilter,
) (newPage, error) {
	msgs, _, hasMore, err := client.GetHistory(ctx, peer, waitHistoryOpts())
	if err != nil {
		return newPage{}, errors.Wrap(err, "reading history")
	}

	page := newPage{fresh: make([]telegram.Message, 0, len(msgs))}

	for idx := range msgs {
		if page.oldest == 0 || msgs[idx].ID < page.oldest {
			page.oldest = msgs[idx].ID
		}

		page.newest = max(page.newest, msgs[idx].ID)

		if msgs[idx].ID > after && filter.admits(&msgs[idx]) {
			page.fresh = append(page.fresh, msgs[idx])
		}
	}

	// The page was full and even its oldest message is new: what arrived
	// between after and that one was not read.
	page.truncated = hasMore && page.oldest > after+1

	slices.SortFunc(page.fresh, func(left, right telegram.Message) int { return left.ID - right.ID })

	return page, nil
}

// buildMessagesWaitResult reports a page with new messages or a truncated
// one. lastMessageId is the newest message returned, or, when the filter
// rejected the whole page, the page's newest, so passing it back waits past
// the page instead of reading it again.
func buildMessagesWaitResult(page newPage) MessagesWaitResult {
	msgs := page.fresh
	output := formatMessages(msgs)
	last := page.newest

	if len(msgs) == 0 {
		output = "No new message on the newest page passed the filter.\n"
	} else {
		last = msgs[len(msgs)-1].ID
	}

	if page.truncated {
		output = fmt.Sprintf("More than one page of messages arrived; older ones were not read. "+
			"Page them with tg_messages_list (offsetId %d).\n", page.oldest) + output
	}

	return MessagesWaitResult{
		Count:         len(msgs),
		LastMessageID: last,
		Truncated:     page.truncated,
		Participants:  participantsFromMessages(msgs),
		Messages:      messagesToItems(msgs),
		Output:        output,
	}
}

// MessagesWaitTool returns the MCP tool definition for tg_messages_wait.
func MessagesWaitTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_wait",
		Description: "Wait until a new message arrives in a Telegram chat, then return it. " +
			"To wait for a reply, pass the messageId of the message you sent as afterMessageId: " +
			"a quick reply can land before this call starts, and the default baseline would then skip it. " +
			"Pass the returned lastMessageId back to keep waiting. One call reads the newest 100 messages: " +
			"when more arrived since afterMessageId, truncated is set (even with count 0, when the filter rejected the whole page) " +
			"and the older ones must be paged with tg_messages_list",
		Annotations: readOnlyAnnotations(),
	}
}
