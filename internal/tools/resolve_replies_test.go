package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

const testSetupLine = "setup line"

func replyToInfo(id int) *telegram.ReplyToInfo {
	return &telegram.ReplyToInfo{MessageID: id}
}

func TestResolveReplyParents_ParentInBatch(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 10, Text: testSetupLine, FromName: "Alice"},
		{ID: 11, Text: "punchline", FromName: "Bob", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{}
	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 (parent already in batch)", mock.getMessagesCalls)
	}

	if items[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated from batch")
	}

	if items[1].ReplyToMessage.Text != testSetupLine {
		t.Errorf("ReplyToMessage.Text = %q, want %q", items[1].ReplyToMessage.Text, testSetupLine)
	}

	if items[1].ReplyToMessage.FromName != "Alice" {
		t.Errorf("ReplyToMessage.FromName = %q, want %q", items[1].ReplyToMessage.FromName, "Alice")
	}
}

func TestResolveReplyParents_ParentOutsideBatch(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 11, Text: "punchline", FromName: "Bob", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)
	peer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	mock := &mockClient{
		parentMessages: []telegram.Message{{ID: 10, Text: testSetupLine, FromName: "Alice"}},
	}

	resolveReplyParents(context.Background(), mock, peer, items, msgs)

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if mock.lastPeer != peer {
		t.Errorf("lastPeer = %+v, want %+v", mock.lastPeer, peer)
	}

	if len(mock.getMessagesIDs) != 1 || mock.getMessagesIDs[0] != 10 {
		t.Errorf("requested IDs = %v, want [10]", mock.getMessagesIDs)
	}

	if items[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if items[0].ReplyToMessage.Text != testSetupLine {
		t.Errorf("ReplyToMessage.Text = %q, want %q", items[0].ReplyToMessage.Text, testSetupLine)
	}
}

func TestResolveReplyParents_SamePeerIgnoresAccessHash(t *testing.T) {
	// FromPeerID produced by extractPeerID lacks AccessHash (not
	// carried on tg.PeerClass). The current peer carries AccessHash
	// from the resolver cache. Comparison must ignore AccessHash,
	// else a same-peer reply is mis-classified as cross-chat.
	peerWithHash := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555, AccessHash: 12345}
	samePeerNoHash := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	msgs := []telegram.Message{
		{
			ID:   11,
			Text: "reply",
			ReplyTo: &telegram.ReplyToInfo{
				MessageID:  10,
				FromPeerID: &samePeerNoHash,
			},
		},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{
		parentMessages: []telegram.Message{
			{ID: 10, Text: testSetupLine, FromName: testAliceName},
		},
	}

	resolveReplyParents(context.Background(), mock, peerWithHash, items, msgs)

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1 (same peer, AccessHash must be ignored)",
			mock.getMessagesCalls)
	}

	if items[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated for same-peer reply")
	}
}

func TestResolveReplyParents_CrossChatSkipped(t *testing.T) {
	otherPeer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 999}
	msgs := []telegram.Message{
		{
			ID:   11,
			Text: "cross-chat",
			ReplyTo: &telegram.ReplyToInfo{
				MessageID:  10,
				FromPeerID: &otherPeer,
			},
		},
	}
	items := messagesToItems(msgs)
	currentPeer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	mock := &mockClient{
		parentMessages: []telegram.Message{{ID: 10, Text: "whatever"}},
	}

	resolveReplyParents(context.Background(), mock, currentPeer, items, msgs)

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 (cross-chat must not trigger fetch)", mock.getMessagesCalls)
	}

	if items[0].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil for cross-chat reply", items[0].ReplyToMessage)
	}
}

func TestResolveReplyParents_CrossChatNotAttachedEvenIfIDCollides(t *testing.T) {
	// A cross-chat reply whose parent-id happens to match a message ID
	// present in the current batch must NOT be resolved from the batch
	// — the batch belongs to a different peer.
	otherPeer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 999}
	currentPeer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}

	msgs := []telegram.Message{
		// Coincidentally shares ID=10 with the cross-chat reply's parent.
		{ID: 10, Text: "unrelated message from current chat", FromName: testAliceName},
		{
			ID:   11,
			Text: "cross-chat",
			ReplyTo: &telegram.ReplyToInfo{
				MessageID:  10,
				FromPeerID: &otherPeer,
			},
		},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{}

	resolveReplyParents(context.Background(), mock, currentPeer, items, msgs)

	if items[1].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil (parent ID collision must not bypass cross-chat filter)",
			items[1].ReplyToMessage)
	}
}

func TestResolveReplyParents_FetchError_BestEffort(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 11, Text: "orphan", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{err: errors.New("api down")}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if items[0].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil when fetch errors (best-effort)", items[0].ReplyToMessage)
	}
}

func TestResolveReplyParents_NoReplies(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 10, Text: "plain"},
		{ID: 11, Text: "also plain"},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 (no replies)", mock.getMessagesCalls)
	}
}

func TestResolveReplyParents_NameFromFetchedLookup(t *testing.T) {
	// A fetched parent with empty FromName must still get a name when
	// another fetched entry shares its FromID and carries the name.
	msgs := []telegram.Message{
		{ID: 11, Text: "reply", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{
		parentMessages: []telegram.Message{
			// The actual parent: known author ID, name missing.
			{ID: 10, Text: "parent text", FromID: 42, FromName: ""},
			// Another fetched entry naming FromID 42.
			{ID: 99, Text: "sibling", FromID: 42, FromName: testAliceName},
		},
	}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if items[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated")
	}

	if items[0].ReplyToMessage.FromName != testAliceName {
		t.Errorf("ReplyToMessage.FromName = %q, want %q (should fall back to fetched lookup)",
			items[0].ReplyToMessage.FromName, testAliceName)
	}
}

func TestResolveReplyParents_AnonymousSenderNotCrossAttributed(t *testing.T) {
	// Two unrelated anonymous posts share FromID==0. Looking up the
	// parent's name by FromID must NOT pick another sender's name
	// from the same zero bucket; it must stay empty.
	msgs := []telegram.Message{
		{ID: 11, Text: "reply", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{
		parentMessages: []telegram.Message{
			// Parent is anonymous: FromID 0, no own name.
			{ID: 10, Text: "parent text", FromID: 0, FromName: ""},
			// Another anonymous entry with a name — must NOT leak.
			{ID: 99, Text: "sibling", FromID: 0, FromName: testAliceName},
		},
	}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if items[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated")
	}

	if items[0].ReplyToMessage.FromName != "" {
		t.Errorf("ReplyToMessage.FromName = %q, want empty (FromID==0 must not share lookup)",
			items[0].ReplyToMessage.FromName)
	}
}

func TestResolveReplyParents_EmptyFetchResponse(t *testing.T) {
	// Telegram can legitimately return zero messages (deleted parent,
	// revoked, etc.). Resolver must not panic and must leave the item's
	// ReplyToMessage nil.
	msgs := []telegram.Message{
		{ID: 11, Text: "orphan", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{
		getMessagesFn: func(_ []int) []telegram.Message {
			return []telegram.Message{}
		},
	}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if items[0].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil on empty fetch response",
			items[0].ReplyToMessage)
	}
}

func TestResolveReplyParents_Truncates(t *testing.T) {
	long := make([]rune, 300)
	for i := range long {
		long[i] = 'x'
	}

	msgs := []telegram.Message{
		{ID: 10, Text: string(long), FromName: "Alice"},
		{ID: 11, Text: "reply", ReplyTo: replyToInfo(10)},
	}
	items := messagesToItems(msgs)

	mock := &mockClient{}

	resolveReplyParents(context.Background(), mock, telegram.InputPeer{}, items, msgs)

	if items[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil")
	}

	runes := []rune(items[1].ReplyToMessage.Text)
	// truncateText limits to 200 runes + ellipsis.
	if len(runes) != replyParentTextLimit+1 {
		t.Errorf("truncated length = %d runes, want %d", len(runes), replyParentTextLimit+1)
	}
}
