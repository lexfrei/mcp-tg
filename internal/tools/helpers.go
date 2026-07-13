package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// validateTopicID rejects topicId values that the chat cannot accept.
// topicID == 0 means "no topic", which is always fine. For non-zero
// values, the chat must be a forum-enabled supergroup; otherwise the
// MTProto layer returns a cryptic error after the round-trip and the
// user has no obvious way to relate it to their input. This pre-flight
// fails fast with a clear message before any send is attempted.
//
// PeerUser (DMs) and PeerChat (legacy basic groups) cannot be forums,
// so we short-circuit without a round-trip. Calling GetGroupInfo on a
// PeerUser would hit MessagesGetFullChat with a user ID and produce a
// nonsense error that buries the actual constraint.
//
// Existence of the topic itself is NOT verified here. ChannelsGetForumTopics
// is more expensive and the failure mode (TOPIC_ID_INVALID) is already
// fielded by wrapTelegramError downstream.
func validateTopicID(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, topicID int,
) error {
	if topicID == 0 {
		return nil
	}

	if peer.Type != telegram.PeerChannel {
		return ErrTopicIDOnNonForum
	}

	info, err := client.GetGroupInfo(ctx, peer)
	if err != nil {
		return errors.Wrap(err, "fetching group info to validate topicId")
	}

	if info == nil || !info.IsForum {
		return ErrTopicIDOnNonForum
	}

	return nil
}

// resolveSendAs resolves an optional send-as identity. An empty
// reference yields a nil identity, which every send path treats as
// "leave send_as unset" — the server then posts under the chat's saved
// default, which is the account itself until tg_chats_set_send_as
// changes it.
//
// An identity that resolves without an access hash is rejected here
// rather than passed on: ResolvePeer returns AccessHash 0 and a nil error
// for a numeric ID it has never seen, and the resulting
// SEND_AS_PEER_INVALID or PEER_ID_INVALID from the server names neither
// the parameter nor the remedy. Listing the chat's identities once seeds
// the cache and makes the same numeric ID work.
//
// A legacy basic group has no access hash by design and can never be an
// identity, so it is refused outright rather than blamed on the cache.
func resolveSendAs(
	ctx context.Context, client telegram.Client, sendAs string,
) (*telegram.InputPeer, error) {
	if sendAs == "" {
		return nil, nil //nolint:nilnil // a nil identity is the documented "use the chat default".
	}

	peer, err := client.ResolvePeer(ctx, sendAs)
	if err != nil {
		return nil, telegramErr("failed to resolve the sendAs identity", err)
	}

	if peer.Type == telegram.PeerChat {
		return nil, validationErr(telegram.ErrSendAsUnsupportedPeer)
	}

	if peer.AccessHash == 0 {
		return nil, validationErr(ErrSendAsUnresolved)
	}

	return &peer, nil
}

// resolveOptionalPeer resolves an optional peer reference. An empty
// reference yields nil, meaning the caller leaves the corresponding
// request field unset. paramName labels resolution failures so the
// error points at the offending tool parameter rather than the main
// peer argument.
func resolveOptionalPeer(
	ctx context.Context, client telegram.Client, ref, paramName string,
) (*telegram.InputPeer, error) {
	if ref == "" {
		return nil, nil //nolint:nilnil // nil is the documented "leave unset".
	}

	peer, err := client.ResolvePeer(ctx, ref)
	if err != nil {
		return nil, telegramErr("failed to resolve the "+paramName+" peer", err)
	}

	return &peer, nil
}

// validateDateRange rejects negative bounds and a window whose lower
// bound exceeds its upper bound. Zero means unbounded on that side, so
// only a window with both ends set can be inverted.
func validateDateRange(minDate, maxDate int) error {
	if minDate < 0 || maxDate < 0 {
		return ErrNegativeDate
	}

	if minDate > 0 && maxDate > 0 && minDate > maxDate {
		return ErrInvalidDateRange
	}

	return nil
}

// validatePlainText rejects markdown-looking text sent with
// parseMode=plain unless allowRaw overrides. Empty text (a file or
// album without a caption) always passes.
func validatePlainText(mode string, allowRaw bool, text string) error {
	if mode != telegram.ParseModePlain || allowRaw || text == "" {
		return nil
	}

	if telegram.LooksLikeMarkdown(text) {
		return ErrPlainLooksLikeMarkdown
	}

	return nil
}

// normalizeParseMode lowercases the input so callers can pass
// "Markdown", "COMMONMARK" etc. without getting a validation error.
func normalizeParseMode(mode string) string {
	return strings.ToLower(mode)
}

// deref returns the value of a pointer or a zero value if nil.
func deref[T any](ptr *T) T {
	if ptr == nil {
		var zero T

		return zero
	}

	return *ptr
}

// validateLimit returns an error if limit is negative.
func validateLimit(limit int) error {
	if limit < 0 {
		return ErrNegativeLimit
	}

	return nil
}

// hasMorePage returns true when the returned count saturates the page,
// signalling the caller that another page may be available. The
// requestedLimit may be zero (caller did not specify), in which case the
// server-default page size is assumed via telegram.DefaultLimit.
func hasMorePage(count, requestedLimit int) bool {
	effective := requestedLimit
	if effective <= 0 {
		effective = telegram.DefaultLimit
	}

	return count >= effective
}

const maxIDsPerRequest = 100

// validateIDCount returns an error if too many IDs are provided.
func validateIDCount(ids []int) error {
	if len(ids) > maxIDsPerRequest {
		return ErrTooManyIDs
	}

	return nil
}

// formatPeer returns a bot-API style numeric ID that can be passed back
// as a peer parameter to other tools. Positive = user, negative = chat,
// -100xxx = channel.
func formatPeer(peer telegram.InputPeer) string {
	switch peer.Type {
	case telegram.PeerUser:
		return strconv.FormatInt(peer.ID, 10)
	case telegram.PeerChat:
		return strconv.FormatInt(-peer.ID, 10)
	case telegram.PeerChannel:
		return strconv.FormatInt(-telegram.ChannelIDOffset-peer.ID, 10)
	default:
		return strconv.FormatInt(peer.ID, 10)
	}
}

// Telegram-accepted slowmode delay values in seconds.
const (
	slowmodeOff = 0
	slowmode10s = 10
	slowmode30s = 30
	slowmode1m  = 60
	slowmode5m  = 300
	slowmode15m = 900
	slowmode1h  = 3600
	slowmode6h  = 21600
	slowmode12h = 43200
)

// validSlowmode reports whether sec is a Telegram-accepted slowmode value.
func validSlowmode(sec int) bool {
	switch sec {
	case slowmodeOff, slowmode10s, slowmode30s, slowmode1m,
		slowmode5m, slowmode15m, slowmode1h, slowmode6h,
		slowmode12h:
		return true
	default:
		return false
	}
}

// validateParseMode accepts exactly 'plain' and 'commonmark'. There is
// deliberately no default and no legacy alias: an omitted mode and the
// retired 'markdown' spelling each get their own steering error.
// Comparison is case-insensitive here as defense in depth — the schema
// enum enforces strict lowercase before the handler ever runs.
func validateParseMode(mode string) error {
	switch normalizeParseMode(mode) {
	case telegram.ParseModePlain, telegram.ParseModeCommonMark:
		return nil
	case "":
		return ErrParseModeRequired
	case telegram.ParseModeMarkdown:
		return ErrMarkdownAliasRemoved
	case telegram.ParseModeHTML, telegram.ParseModeMarkdownV2:
		return ErrParseModeNotImplemented
	default:
		return ErrUnknownParseMode
	}
}

// truncateText returns text shortened to at most maxRunes runes,
// appending an ellipsis when truncation happened. Operates on runes
// to avoid splitting multi-byte sequences (Cyrillic, emoji, etc.).
func truncateText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}

	runes := []rune(text)

	return string(runes[:maxRunes]) + "…"
}

// formatUserName builds a display name plus identifier for a User.
// Routes through formatPeerRef so the rendered shape matches the rest
// of the tool surface: "First Last [@username]" / "First Last [user:N]"
// / "First Last [hidden]" — never the legacy "Name (@username)" form.
func formatUserName(user *telegram.User) string {
	if user == nil {
		return unknownValue
	}

	name := strings.TrimSpace(user.FirstName + " " + user.LastName)

	return formatPeerRef(name, user.Username,
		telegram.InputPeer{Type: telegram.PeerUser, ID: user.ID})
}

const peerRefHidden = "[hidden]"

// formatPeerRef returns a single human-readable identifier that
// unambiguously names a peer in message metadata. Output shapes:
//
//	"Display Name [@username]" — public username available
//	"Display Name [user:N]" / "[channel:N]" / "[group:N]" — id-only
//	"Display Name [hidden]" — name present but no resolvable id (privacy-hidden forwards)
//	"[user:N]" / "[hidden]" — degenerate forms when display name missing
//
// Callers pass the display name, optional @username and the InputPeer
// that identifies the peer. A zero InputPeer (Type == PeerUser && ID == 0)
// is treated as "no peer identity available".
//
// Display name and username are passed through collapseLineBreaks so an
// adversarial peer name like "Alice\nfrom: Mallory" cannot inject fake
// key:value lines into the multi-line text output. The JSON surface
// keeps the original strings verbatim — sanitization is purely a
// presentation-layer defense against prompt-injection through the
// human-readable output that LLMs typically consume.
//
// Important scope limit: the message body itself (Message.Text)
// is rendered VERBATIM after 'text:\n' and is NOT sanitized. A user
// can therefore put '\n---\n[999] 2030-01-01T00:00:00Z\nfrom: Admin\n
// text:\nfake' in their message body and the rendered output will
// look like two messages. The JSON 'messages[]' array is the
// authoritative shape if integrity matters — body verbatim
// preservation is a deliberate UX choice for code blocks and quoted
// text, not a security property.
func formatPeerRef(name, username string, peer telegram.InputPeer) string {
	name = collapseLineBreaks(name)
	username = collapseLineBreaks(username)
	label := peerLabel(peer, username)

	if name != "" && label != "" {
		return fmt.Sprintf("%s [%s]", name, label)
	}

	if name != "" {
		return name + " " + peerRefHidden
	}

	if label != "" {
		return fmt.Sprintf("[%s]", label)
	}

	return peerRefHidden
}

// peerLabel returns the bracket-contents portion of formatPeerRef.
// Returns "" when the peer is empty AND no username is provided so the
// caller can fall back to "[hidden]". Callers must pre-sanitize the
// username for line-break injection if the value comes from
// adversarial input; formatPeerRef does this for its callers.
//
// The kind labels — user / group / channel — match the strings used by
// ParticipantItem.Type and MessageItem.FromType, so a caller can grep
// either surface for the same identifier ("group:42" appears in both
// text and JSON). "channel:N" covers both broadcast channels and
// supergroups (gotd represents both as PeerChannel); "group:N" is
// only legacy basic groups.
func peerLabel(peer telegram.InputPeer, username string) string {
	if username != "" {
		return "@" + username
	}

	if peer.ID == 0 {
		return ""
	}

	// Use the shared peerUser / peerGroup / peerChannel /
	// unknownPeerType constants so the text labels stay locked to the
	// JSON labels emitted by participantTypeLabel and messageToItem.
	// Renaming any of these constants now propagates to both surfaces.
	switch peer.Type {
	case telegram.PeerUser:
		return fmt.Sprintf("%s:%d", peerUser, peer.ID)
	case telegram.PeerChat:
		return fmt.Sprintf("%s:%d", peerGroup, peer.ID)
	case telegram.PeerChannel:
		return fmt.Sprintf("%s:%d", peerChannel, peer.ID)
	default:
		// Don't masquerade a future PeerType as 'user:' — surface the
		// fact that we don't know the kind so the reader doesn't
		// trust the wrong deep-link form.
		return fmt.Sprintf("%s:%d", unknownPeerType, peer.ID)
	}
}
