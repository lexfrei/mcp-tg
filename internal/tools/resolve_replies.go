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
	refByID := buildSenderLookup(msgs, fetched)

	for idx := range items {
		reply := items[idx].ReplyTo
		if reply == nil || reply.MessageID == 0 {
			continue
		}

		if isCrossChatReply(reply, peer) {
			continue
		}

		parent := lookupParent(reply.MessageID, inBatch, fetched)
		if parent == nil {
			continue
		}

		items[idx].ReplyToMessage = buildReplyToMessage(parent, refByID)
	}
}

func lookupParent(
	parentID int, inBatch, fetched map[int]*telegram.Message,
) *telegram.Message {
	if parent, ok := inBatch[parentID]; ok {
		return parent
	}

	if parent, ok := fetched[parentID]; ok {
		return parent
	}

	return nil
}

func buildReplyToMessage(parent *telegram.Message, refByID map[int64]senderRef) *ReplyToMessage {
	name, username := parent.FromName, parent.FromUsername

	if parent.FromID != 0 {
		ref := refByID[parent.FromID]
		if name == "" {
			name = ref.name
		}

		if username == "" {
			username = ref.username
		}
	}

	return &ReplyToMessage{
		FromName:     name,
		FromUsername: username,
		Text:         truncateText(parent.Text, replyParentTextLimit),
	}
}

type senderRef struct {
	name     string
	username string
}

// buildSenderLookup combines FromID→{name, username} from the primary
// batch and from any parents fetched by the resolver. Without fetched
// entries, a parent whose own fields are empty but whose FromID appears
// in the fetched payload would miss its display name.
//
// FromID==0 entries are excluded: zero means "no identifiable sender"
// (channel posts without signature, anonymous admins), and bucketing
// them together would cross-attribute names between unrelated senders.
func buildSenderLookup(msgs []telegram.Message, fetched map[int]*telegram.Message) map[int64]senderRef {
	lookup := make(map[int64]senderRef, len(msgs)+len(fetched))

	// last-wins-among-non-empty: a later non-empty value overrides an
	// earlier one (mirrors how the previous buildNameLookup gated on
	// `FromName != ""` and then assigned). This way a renamed user
	// reflects the most recent display name seen in the batch instead
	// of being frozen to whichever message landed first in iteration
	// order.
	mergeRef := func(peerID int64, name, username string) {
		if peerID == 0 {
			return
		}

		ref := lookup[peerID]
		if name != "" {
			ref.name = name
		}

		if username != "" {
			ref.username = username
		}

		lookup[peerID] = ref
	}

	for idx := range msgs {
		mergeRef(msgs[idx].FromID, msgs[idx].FromName, msgs[idx].FromUsername)
	}

	for _, parent := range fetched {
		if parent != nil {
			mergeRef(parent.FromID, parent.FromName, parent.FromUsername)
		}
	}

	return lookup
}
