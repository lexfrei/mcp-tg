package tools

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testAliceName      = "Alice"
	testBobName        = "Bob"
	testPunchline      = "punchline"
	testChatPeer       = "@chat"
	testSetupFromEarly = "setup from earlier"
	testSetupWord      = "setup"
)

func messagesWithReply() []telegram.Message {
	return []telegram.Message{
		{ID: 26150, Date: 1700000000, Text: testSetupWord, FromName: testAliceName},
		{
			ID:       26154,
			Date:     1700000001,
			Text:     testPunchline,
			FromName: testBobName,
			ReplyTo:  &telegram.ReplyToInfo{MessageID: 26150},
		},
	}
}

func TestMessagesListHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
		total:    2,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{Peer: testChatPeer})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("expected ReplyTo on second message")
	}

	if res.Messages[1].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo.MessageID = %d, want 26150", res.Messages[1].ReplyTo.MessageID)
	}

	if !strings.Contains(res.Output, "reply to: 26150") {
		t.Errorf("output missing reply marker reply to: 26150: %q", res.Output)
	}

	// Without ResolveReplies, parent text should not be fetched again
	// (it's already in the batch), but ReplyToMessage should also not
	// be populated since the flag is off.
	if res.Messages[1].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil when resolveReplies is off", res.Messages[1].ReplyToMessage)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0", mock.getMessagesCalls)
	}
}

func TestMessagesListHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    testPunchline,
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
		total: 1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           testChatPeer,
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}

	if res.Messages[0].ReplyToMessage.FromName != testAliceName {
		t.Errorf("ReplyToMessage.FromName = %q, want %q",
			res.Messages[0].ReplyToMessage.FromName, testAliceName)
	}
}

// In text format the structured messages are dropped, so the parent
// enrichment they would carry is worthless — the extra GetMessages RPC
// must be skipped even when resolveReplies is on.
func TestMessagesListHandler_ResolveReplies_SkippedInTextFormat(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{ID: 26154, Text: testPunchline, ReplyTo: &telegram.ReplyToInfo{MessageID: 26150}},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
		total: 1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           testChatPeer,
		ResolveReplies: &resolveOn,
		Format:         formatText,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 (enrichment is wasted in text format)", mock.getMessagesCalls)
	}

	if res.Messages != nil {
		t.Errorf("text format must drop the structured messages, got %+v", res.Messages)
	}

	if res.Output == "" {
		t.Error("text format must keep the human-readable output")
	}
}

func TestMessagesListHandler_ResolveReplies_ParentInBatchNoFetch(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
		total:    2,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           testChatPeer,
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 when parent already in batch",
			mock.getMessagesCalls)
	}

	if res.Messages[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated from batch when resolveReplies is on")
	}

	if res.Messages[1].ReplyToMessage.Text != testSetupWord {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[1].ReplyToMessage.Text, testSetupWord)
	}
}

func TestMessagesContextHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    testPunchline,
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
	}
	handler := NewMessagesContextHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesContextParams{
		Peer:           testChatPeer,
		MessageID:      26154,
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}
}

func TestMessagesGetHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    testPunchline,
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
	}
	handler := NewMessagesGetHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesGetParams{
		Peer:           testChatPeer,
		IDs:            []int{26154},
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// messages_get fetches the requested IDs (26154) via GetMessages,
	// then the resolver makes a second GetMessages call for the
	// missing parent (26150). Any change that folds the two into a
	// single call must update this assertion deliberately.
	const wantGetMessagesCalls = 2
	if mock.getMessagesCalls != wantGetMessagesCalls {
		t.Errorf("GetMessages calls = %d, want %d (primary fetch + resolver lookup)",
			mock.getMessagesCalls, wantGetMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}
}

func TestMessagesContextHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesContextHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesContextParams{
		Peer:      testChatPeer,
		MessageID: 26154,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil || res.Messages[1].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo not propagated: %+v", res.Messages[1].ReplyTo)
	}

	if !strings.Contains(res.Output, "reply to: 26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesGetHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesGetHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesGetParams{
		Peer: testChatPeer,
		IDs:  []int{26150, 26154},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want populated")
	}

	if !strings.Contains(res.Output, "reply to: 26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesSearchHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{Peer: testChatPeer, Query: testPunchline},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want populated")
	}

	if !strings.Contains(res.Output, "reply to: 26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesSearchHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	primary := []telegram.Message{
		{
			ID:      26154,
			Text:    testPunchline,
			ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
		},
	}
	fetched := []telegram.Message{
		{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
	}
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: primary,
		getMessagesFn: func(_ []int) []telegram.Message {
			return fetched
		},
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(
		context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{
			Peer:           testChatPeer,
			Query:          testPunchline,
			ResolveReplies: &resolveOn,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}
}

func TestMessagesSearchHandler_ResolveReplies_ParentInBatchNoFetch(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(
		context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{
			Peer:           testChatPeer,
			Query:          testPunchline,
			ResolveReplies: &resolveOn,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 when parent already in batch",
			mock.getMessagesCalls)
	}

	if res.Messages[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated from batch when resolveReplies is on")
	}

	if res.Messages[1].ReplyToMessage.Text != testSetupWord {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[1].ReplyToMessage.Text, testSetupWord)
	}
}

func TestMessagesListHandler_ResolveReplies_OutputUnchanged(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    testPunchline,
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
		total: 1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           testChatPeer,
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Output only carries the "reply to: <parentId>" marker; resolved
	// parent text lives in the JSON replyToMessage field. Keep both
	// behaviours pinned so future changes touch them deliberately.
	if strings.Contains(res.Output, testSetupFromEarly) {
		t.Errorf("Output must not embed resolved parent text, got %q", res.Output)
	}

	if !strings.Contains(res.Output, "reply to: 26150") {
		t.Errorf("Output missing reply marker reply to: 26150: %q", res.Output)
	}
}

func TestMessagesSearchGlobalHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    testPunchline,
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSearchGlobalParams{
		Query: testPunchline,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[0].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want propagated in global search")
	}

	if res.Messages[0].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo.MessageID = %d, want 26150", res.Messages[0].ReplyTo.MessageID)
	}

	// Global search intentionally returns only a summary line as
	// output; per-message reply lines are not emitted. Pin that, so
	// a future change to output format has to touch this test.
	if strings.Contains(res.Output, "reply to:") {
		t.Errorf("Output must not contain per-message reply lines for global search, got %q", res.Output)
	}
}

// TestResolveReplyParents_ChunksAtTelegramsIDCap pins the resolver
// against Telegram's per-call ceiling on messages.getMessages: "passing
// up to 200 IDs from the range that needs filling, re-invoking the
// method until the desired range is fetched".
//
// The cap is reachable and its failure is silent. tg_messages_list takes
// a limit of up to 1000, so one call can collect more than 200 distinct
// out-of-batch parents; an unchunked request is rejected wholesale, and
// because the resolver swallows fetch errors EVERY out-of-batch parent
// loses its replyToMessage while in-batch ones keep theirs — a partial
// result that looks like a chat where nobody quoted anything.
//
// The mock models the server rather than the client: it rejects an
// over-long id list, exactly as Telegram does. Without chunking this
// test reports zero enriched parents.
func TestResolveReplyParents_ChunksAtTelegramsIDCap(t *testing.T) {
	const replyCount = 250 // > getMessagesMaxIDs, reachable via tg_messages_list

	msgs, fetch := replyBatch(replyCount)

	client := &mockClient{
		// Expressed in terms of the constant, so this pins the chunking
		// rather than the number: Telegram documents 200 but the method
		// page states no limit, and the boundary is not observable here.
		getMessagesErrFn: func(ids []int) error {
			if len(ids) > getMessagesMaxIDs {
				return errors.New("MSG_ID_INVALID: too many ids in one request")
			}

			return nil
		},
		getMessagesFn: fetch,
	}

	items := messagesToItems(msgs)
	peer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	resolveReplyParents(t.Context(), client, peer, items, msgs)

	enriched := 0

	for idx := range items {
		if items[idx].ReplyToMessage != nil {
			enriched++
		}
	}

	if enriched != replyCount {
		t.Errorf("enriched %d of %d replies — an over-long id list is rejected wholesale, "+
			"so every out-of-batch parent is silently lost", enriched, replyCount)
	}

	for _, size := range client.getMessagesSizes {
		if size > getMessagesMaxIDs {
			t.Errorf("GetMessages called with %d ids, above Telegram's cap of %d",
				size, getMessagesMaxIDs)
		}
	}

	// ceil(250/200): the flag's cost is one round-trip per 200 missing
	// parents, not the "at most one" a single-call resolver implied.
	if client.getMessagesCalls != 2 {
		t.Errorf("getMessagesCalls = %d, want 2 chunks for %d parents",
			client.getMessagesCalls, replyCount)
	}
}

// replyBatch builds `count` replies, each pointing at a distinct parent
// outside the returned batch, plus a fetch stub that answers with the
// parent the caller asked for.
func replyBatch(count int) ([]telegram.Message, func(ids []int) []telegram.Message) {
	const firstReplyID = 1000

	msgs := make([]telegram.Message, 0, count)
	for idx := range count {
		msgs = append(msgs, telegram.Message{
			ID:      firstReplyID + idx,
			Text:    "a reply",
			ReplyTo: &telegram.ReplyToInfo{MessageID: idx + 1},
		})
	}

	fetch := func(ids []int) []telegram.Message {
		out := make([]telegram.Message, 0, len(ids))
		for _, id := range ids {
			out = append(out, telegram.Message{
				ID: id, FromID: 7, FromName: "Parent Author", Text: "parent text",
			})
		}

		return out
	}

	return msgs, fetch
}

// TestResolveReplyParents_FailedChunkKeepsTheOthers pins the partial
// failure the chunked resolver promises: one chunk erroring must cost
// only its own parents, not every parent in the call.
//
// The distinction is invisible with a single chunk, which is why the
// pre-chunking best-effort test could not catch it: "return what we have"
// and "discard everything" look identical when there is one request. With
// several, discarding is a silent, total loss of enrichment because one
// arbitrary chunk failed — so the branch needs its own test.
func TestResolveReplyParents_FailedChunkKeepsTheOthers(t *testing.T) {
	const replyCount = 250 // two chunks: parents 1..200 and 201..250

	msgs, fetch := replyBatch(replyCount)

	client := &mockClient{
		// Fail only the chunk carrying parent 1, i.e. the first.
		getMessagesErrFn: func(ids []int) error {
			if slices.Contains(ids, 1) {
				return errors.New("MSG_ID_INVALID")
			}

			return nil
		},
		getMessagesFn: fetch,
	}

	items := messagesToItems(msgs)
	peer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	resolveReplyParents(t.Context(), client, peer, items, msgs)

	for idx := range items {
		parentID := idx + 1

		got := items[idx].ReplyToMessage
		if parentID <= getMessagesMaxIDs && got != nil {
			t.Fatalf("parent %d came from the failed chunk, want no ReplyToMessage, got %+v",
				parentID, got)
		}

		if parentID > getMessagesMaxIDs && got == nil {
			t.Fatalf("parent %d is in a chunk that succeeded, but its enrichment was "+
				"discarded along with the failed chunk's", parentID)
		}
	}
}

// TestResolveReplyParents_StopsOnDeadContext pins the early exit. Once
// the lookup is chunked, a cancelled call would otherwise pay for every
// remaining chunk — five doomed round-trips at tg_messages_list's top
// limit, none of which can be delivered. Throttling is the other way to
// waste the same chunks and is bounded separately, since FLOOD_WAIT never
// cancels the context: see TestResolveReplyParents_StopsOnFloodWait.
func TestResolveReplyParents_StopsOnDeadContext(t *testing.T) {
	msgs, fetch := replyBatch(250)
	client := &mockClient{getMessagesFn: fetch}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	resolveReplyParents(ctx, client, telegram.InputPeer{Type: telegram.PeerChannel, ID: 555},
		messagesToItems(msgs), msgs)

	if client.getMessagesCalls != 0 {
		t.Errorf("getMessagesCalls = %d on a cancelled context, want 0 — the resolver "+
			"paid for chunks it could never deliver", client.getMessagesCalls)
	}
}

// TestResolveReplyParents_CancelledMidLoopKeepsWhatItHas pins the other
// half of the early exit: breaking must return the parents already
// fetched, not discard them. StopsOnDeadContext cancels before the first
// chunk, so its result map is empty either way — only a cancellation
// BETWEEN chunks can tell "return what we have" from "return nothing".
func TestResolveReplyParents_CancelledMidLoopKeepsWhatItHas(t *testing.T) {
	const replyCount = 250 // two chunks: parents 1..200, then 201..250

	msgs, fetch := replyBatch(replyCount)
	ctx, cancel := context.WithCancel(t.Context())

	client := &mockClient{
		getMessagesFn: func(ids []int) []telegram.Message {
			cancel() // the first chunk lands, then the caller goes away

			return fetch(ids)
		},
	}

	items := messagesToItems(msgs)
	resolveReplyParents(ctx, client, telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}, items, msgs)

	if client.getMessagesCalls != 1 {
		t.Fatalf("getMessagesCalls = %d, want 1 — the loop must stop once the context dies",
			client.getMessagesCalls)
	}

	if items[0].ReplyToMessage == nil {
		t.Error("parent 1 was fetched before the cancellation, but its enrichment was thrown away")
	}

	if last := items[replyCount-1].ReplyToMessage; last != nil {
		t.Errorf("parent %d belongs to the chunk that never ran, got %+v", replyCount, last)
	}
}

// TestResolveReplyParents_StopsOnFloodWait pins the other half of the
// bounded loop. FLOOD_WAIT is a statement about the connection, not the
// chunk: the remaining chunks would hit the same throttle, and the flood
// middleware sleeps through its retries before handing the raw error
// here — so a `continue` spends a server-chosen delay per chunk to
// collect nothing. Cancellation does not cover this; FLOOD_WAIT never
// cancels the context, and no caller sets a deadline.
//
// It takes THREE chunks with the throttle in the MIDDLE to pin the branch
// from both sides, and each detail earns its place:
//
//   - Flooding every chunk leaves nothing collected either way, so `break`
//     and `return nil` look identical — the same one-chunk blindness
//     FailedChunkKeepsTheOthers exists to avoid.
//   - Flooding the LAST chunk makes `break` and `continue` identical,
//     since there is nothing left to skip.
//
// With a middle throttle, `continue` shows up as a third call, `return
// nil` as the first chunk's parents going missing.
func TestResolveReplyParents_StopsOnFloodWait(t *testing.T) {
	// Three chunks: parents 1..200, 201..400, 401..450.
	const (
		replyCount    = 450
		throttledFrom = getMessagesMaxIDs + 1 // first id of the middle chunk
	)

	msgs, fetch := replyBatch(replyCount)

	client := &mockClient{
		getMessagesErrFn: func(ids []int) error {
			if slices.Contains(ids, throttledFrom) {
				// Wrapped exactly as wrapper.GetMessages wraps it. A bare
				// tgerr would test a shape the resolver never sees: the
				// break depends on tgerr.AsFloodWait unwrapping through
				// cockroachdb's Wrap, so an opaque wrap would degrade it
				// to a continue with this test still green.
				return errors.Wrap(tgerr.New(420, "FLOOD_WAIT_30"), "getting messages")
			}

			return nil
		},
		getMessagesFn: fetch,
	}

	items := messagesToItems(msgs)

	resolveReplyParents(t.Context(), client, telegram.InputPeer{Type: telegram.PeerChannel, ID: 555},
		items, msgs)

	if client.getMessagesCalls != 2 {
		t.Fatalf("getMessagesCalls = %d, want 2 — the loop must stop at the throttled chunk "+
			"instead of sleeping through the retries again on the third",
			client.getMessagesCalls)
	}

	if items[0].ReplyToMessage == nil {
		t.Error("parent 1 was fetched before the throttle, but stopping threw it away")
	}

	if got := items[throttledFrom-1].ReplyToMessage; got != nil {
		t.Errorf("parent %d belongs to the throttled chunk, got %+v", throttledFrom, got)
	}

	if last := items[replyCount-1].ReplyToMessage; last != nil {
		t.Errorf("parent %d is past the throttle and must never be fetched, got %+v",
			replyCount, last)
	}
}
