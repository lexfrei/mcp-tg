package tools

import (
	"context"
	"slices"

	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// getMessagesMaxIDs is the per-call id ceiling Telegram DOCUMENTS for
// messages.getMessages and channels.getMessages: "passing up to 200 IDs
// from the range that needs filling, re-invoking the method until the
// desired range is fetched" (core.telegram.org/api/updates). The method
// pages themselves state no limit, so that guidance is the only place
// Telegram names a number for these two methods.
//
// TDLib corroborates it independently and more strongly: it splits both
// methods' id lists at `MAX_SLICE_SIZE = 200`, annotated `// server-side
// limit`, on its general read path rather than only when recovering gaps
// — which is what rules out reading the quote above as advice narrow to
// gap-filling. Nothing found argues for a lower ceiling: Telethon chunks
// at 100, but that is its history-pagination constant reused, where 100
// is genuinely getHistory's page size, and it claims nothing about
// getMessages.
//
// Not observed here, though — confirming it firsthand would need a chat
// yielding more than 200 distinct out-of-batch parents in one call,
// reachable in principle (see below) but not producible on demand. Treat
// it as an upper bound to respect rather than a measured one: chunking
// smaller is always safe, larger is not.
//
// The unchunked send it replaces was reachable: tg_messages_list takes a
// limit of up to 1000, so one call can collect more than 200 distinct
// out-of-batch parents. Past that, an over-long request is rejected whole,
// fetchMissingParents swallows the error, and EVERY out-of-batch parent
// loses its replyToMessage while in-batch ones keep theirs — a partial,
// plausible-looking result rather than an error. That failure follows from
// the documented cap; it was never witnessed, for the same reason the cap
// itself was not. Chunking is what makes "every reply whose parent is
// reachable" true in the docs.
//
// The tests express the mock's cap in terms of this constant, so they pin
// the chunking, not the number: were the real ceiling lower, they would
// stay green while the silent failure returned. That is why the sourcing
// above is spelled out rather than left as a bare 200.
//
// A second ceiling governs the same wrapper method and disagrees:
// maxIDsPerRequest (helpers.go) rejects a user-supplied `ids` list above
// 100 in tg_messages_get. The two are not in conflict — that one is a
// blanket input guard applied to delete/forward/get in one sweep, one
// round number for three unrelated RPCs and no citation, so it is not
// evidence that this method's limit is 100. It is also why tg_messages_get
// can never reach the cap here, which the docs' round-trip argument rests
// on. Change one and check the other; a reader who finds only one of them
// will not know the other exists.
const getMessagesMaxIDs = 200

// replyParentTextLimit caps parent-message text copied into
// ReplyToMessage. Kept short to avoid bloating output when callers
// only need reply context.
const replyParentTextLimit = 200

// resolveReplyParents fills MessageItem.ReplyToMessage for every item
// whose parent is reachable. Parents already present in msgs cost no
// request; the ones missing from it are fetched in batched GetMessages
// calls of at most getMessagesMaxIDs ids each. Parents from another peer
// (cross-chat reply) are skipped. Errors are swallowed — the resolver is
// best-effort, and a failed chunk costs only its own parents.
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

// fetchMissingParents fetches parents in chunks of at most
// getMessagesMaxIDs, since Telegram rejects a longer id list outright.
// A failed chunk costs only its own parents rather than the whole batch:
// the resolver is best-effort either way, and a partial enrichment beats
// discarding parents that were fetched successfully.
func fetchMissingParents(
	ctx context.Context,
	client telegram.MessageClient,
	peer telegram.InputPeer,
	ids []int,
) map[int]*telegram.Message {
	if len(ids) == 0 {
		return nil
	}

	result := make(map[int]*telegram.Message, len(ids))

	for chunk := range slices.Chunk(ids, getMessagesMaxIDs) {
		// Chunking multiplies the cost of a dead context: without this
		// check a cancelled call would pay for every remaining chunk
		// before returning what it already has.
		if ctx.Err() != nil {
			break
		}

		parents, err := client.GetMessages(ctx, peer, chunk)
		if err != nil {
			// FLOOD_WAIT is singled out on COST, not category. Plenty of
			// errors here are equally certain to repeat on every later
			// chunk (CHANNEL_INVALID, AUTH_KEY_UNREGISTERED) and still
			// continue — they fail fast, so at most five wasted chunks at
			// tg_messages_list's top limit. A throttle is the one that
			// gets expensive: newFloodWaitMiddleware sleeps a
			// server-chosen delay through each of its retries before
			// passing the raw error up, so continuing pays that per chunk
			// to collect nothing. ctx.Err() above cannot catch it —
			// FLOOD_WAIT never cancels the context, and no caller here
			// sets a deadline. Chunking is what made the multiplication
			// possible, so it carries the bound.
			if _, isFlood := tgerr.AsFloodWait(err); isFlood {
				break
			}

			// Anything else costs only its own parents.
			continue
		}

		for idx := range parents {
			result[parents[idx].ID] = &parents[idx]
		}
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

func buildReplyToMessage(parent *telegram.Message, refByID map[senderKey]senderRef) *ReplyToMessage {
	name, username := parent.FromName, parent.FromUsername

	if parent.FromID != 0 {
		ref := refByID[senderKey{Type: parent.FromType, ID: parent.FromID}]
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

type senderKey struct {
	Type telegram.PeerType
	ID   int64
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
//
// The key is keyed on {FromType, FromID} rather than bare FromID — same
// reason participantsFromMessages does: a user with ID 500 and a channel
// posting under its own identity with ID 500 must not stomp each other.
func buildSenderLookup(msgs []telegram.Message, fetched map[int]*telegram.Message) map[senderKey]senderRef {
	lookup := make(map[senderKey]senderRef, len(msgs)+len(fetched))

	// last-wins-among-non-empty: a later non-empty value overrides an
	// earlier one (mirrors how the previous buildNameLookup gated on
	// `FromName != ""` and then assigned). This way a renamed user
	// reflects the most recent display name seen in the batch instead
	// of being frozen to whichever message landed first in iteration
	// order.
	mergeRef := func(peerType telegram.PeerType, peerID int64, name, username string) {
		if peerID == 0 {
			return
		}

		key := senderKey{Type: peerType, ID: peerID}
		ref := lookup[key]

		if name != "" {
			ref.name = name
		}

		if username != "" {
			ref.username = username
		}

		lookup[key] = ref
	}

	for idx := range msgs {
		mergeRef(msgs[idx].FromType, msgs[idx].FromID, msgs[idx].FromName, msgs[idx].FromUsername)
	}

	// Iterate fetched in deterministic key order so two parents
	// sharing the same {FromType, FromID} but disagreeing on
	// FromName/FromUsername always tie-break the same way across runs
	// (Go map iteration order is randomized).
	ids := make([]int, 0, len(fetched))
	for parentID := range fetched {
		ids = append(ids, parentID)
	}

	slices.Sort(ids)

	for _, parentID := range ids {
		parent := fetched[parentID]
		if parent != nil {
			mergeRef(parent.FromType, parent.FromID, parent.FromName, parent.FromUsername)
		}
	}

	return lookup
}
