package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// replyParentTextLimit caps parent-message text copied into
// ReplyToMessage. Kept short to avoid bloating output when callers
// only need reply context.
const replyParentTextLimit = 200

// resolveReplyParents fills MessageItem.ReplyToMessage for items
// whose parent message is not already present in msgs. Missing
// parents from the same peer are fetched in a single batched
// GetMessages call; parents from another peer (cross-chat reply)
// are skipped. Errors are swallowed — the resolver is best-effort.
func resolveReplyParents(
	ctx context.Context,
	client telegram.MessageClient,
	peer telegram.InputPeer,
	items []MessageItem,
	msgs []telegram.Message,
) {
	if len(items) == 0 {
		return
	}

	inBatch := indexMessagesByID(msgs)
	missingIDs := collectMissingParentIDs(items, inBatch, peer)
	fetched := fetchMissingParents(ctx, client, peer, missingIDs)

	attachReplyParents(items, msgs, peer, inBatch, fetched)
}

func indexMessagesByID(msgs []telegram.Message) map[int]*telegram.Message {
	index := make(map[int]*telegram.Message, len(msgs))
	for idx := range msgs {
		index[msgs[idx].ID] = &msgs[idx]
	}

	return index
}

func collectMissingParentIDs(
	items []MessageItem,
	inBatch map[int]*telegram.Message,
	peer telegram.InputPeer,
) []int {
	seen := make(map[int]bool)

	var ids []int

	for idx := range items {
		reply := items[idx].ReplyTo
		if reply == nil || reply.MessageID == 0 {
			continue
		}

		if isCrossChatReply(reply, peer) {
			continue
		}

		if _, ok := inBatch[reply.MessageID]; ok {
			continue
		}

		if seen[reply.MessageID] {
			continue
		}

		seen[reply.MessageID] = true
		ids = append(ids, reply.MessageID)
	}

	return ids
}

// isCrossChatReply reports whether reply points to another peer than
// the one whose history we are currently paging through. AccessHash is
// ignored because it is not carried on the MTProto PeerClass inside
// MessageReplyHeader — identity is Type+ID only.
func isCrossChatReply(reply *telegram.ReplyToInfo, peer telegram.InputPeer) bool {
	if reply.FromPeerID == nil {
		return false
	}

	other := *reply.FromPeerID

	return other.Type != peer.Type || other.ID != peer.ID
}

func fetchMissingParents(
	ctx context.Context,
	client telegram.MessageClient,
	peer telegram.InputPeer,
	ids []int,
) map[int]*telegram.Message {
	if len(ids) == 0 {
		return nil
	}

	parents, err := client.GetMessages(ctx, peer, ids)
	if err != nil {
		return nil
	}

	result := make(map[int]*telegram.Message, len(parents))
	for idx := range parents {
		result[parents[idx].ID] = &parents[idx]
	}

	return result
}

func attachReplyParents(
	items []MessageItem,
	msgs []telegram.Message,
	peer telegram.InputPeer,
	inBatch map[int]*telegram.Message,
	fetched map[int]*telegram.Message,
) {
	nameByID := buildNameLookup(msgs, fetched)

	for idx := range items {
		reply := items[idx].ReplyTo
		if reply == nil || reply.MessageID == 0 {
			continue
		}

		if isCrossChatReply(reply, peer) {
			continue
		}

		parent, ok := inBatch[reply.MessageID]
		if !ok {
			parent, ok = fetched[reply.MessageID]
		}

		if !ok || parent == nil {
			continue
		}

		name := parent.FromName
		if name == "" && parent.FromID != 0 {
			name = nameByID[parent.FromID]
		}

		items[idx].ReplyToMessage = &ReplyToMessage{
			FromName: name,
			Text:     truncateText(parent.Text, replyParentTextLimit),
		}
	}
}

// buildNameLookup combines FromID→FromName from the primary batch and
// from any parents fetched by the resolver. Without fetched entries,
// a parent whose own FromName is empty but whose FromID appears in
// the fetched payload would miss its display name.
//
// FromID==0 entries are excluded: zero means "no identifiable sender"
// (channel posts without signature, anonymous admins), and bucketing
// them together would cross-attribute names between unrelated senders.
func buildNameLookup(msgs []telegram.Message, fetched map[int]*telegram.Message) map[int64]string {
	lookup := make(map[int64]string, len(msgs)+len(fetched))

	for idx := range msgs {
		if msgs[idx].FromID != 0 && msgs[idx].FromName != "" {
			lookup[msgs[idx].FromID] = msgs[idx].FromName
		}
	}

	for _, parent := range fetched {
		if parent != nil && parent.FromID != 0 && parent.FromName != "" {
			lookup[parent.FromID] = parent.FromName
		}
	}

	return lookup
}
