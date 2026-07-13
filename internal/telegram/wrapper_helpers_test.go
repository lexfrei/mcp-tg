package telegram

import (
	"fmt"
	"testing"

	"github.com/gotd/td/tg"
)

const (
	wantChannel    = "channel"
	wantSupergroup = "supergroup"
)

func TestMimeByPath_JPEG(t *testing.T) {
	got := mimeByPath("photo.jpg")
	want := "image/jpeg"

	if got != want {
		t.Errorf("mimeByPath(photo.jpg) = %q, want %q", got, want)
	}
}

func TestMimeByPath_PDF(t *testing.T) {
	got := mimeByPath("document.pdf")
	want := "application/pdf"

	if got != want {
		t.Errorf("mimeByPath(document.pdf) = %q, want %q", got, want)
	}
}

func TestMimeByPath_UnknownExtension(t *testing.T) {
	got := mimeByPath("file.unknown_ext_xyz")

	if got != fallbackMIME {
		t.Errorf("mimeByPath(file.unknown_ext_xyz) = %q, want %q", got, fallbackMIME)
	}
}

func TestMimeByPath_NoExtension(t *testing.T) {
	got := mimeByPath("noextfile")

	if got != fallbackMIME {
		t.Errorf("mimeByPath(noextfile) = %q, want %q", got, fallbackMIME)
	}
}

func TestMimeByPath_HTMLFile(t *testing.T) {
	got := mimeByPath("index.html")

	if got != "text/html; charset=utf-8" && got != "text/html" {
		t.Errorf("mimeByPath(index.html) = %q, want text/html", got)
	}
}

func TestUploadedFileID_InputFile(t *testing.T) {
	file := &tg.InputFile{ID: 42}
	got := uploadedFileID(file)

	if got != 42 {
		t.Errorf("uploadedFileID(InputFile) = %d, want 42", got)
	}
}

func TestUploadedFileID_InputFileBig(t *testing.T) {
	file := &tg.InputFileBig{ID: 99}
	got := uploadedFileID(file)

	if got != 99 {
		t.Errorf("uploadedFileID(InputFileBig) = %d, want 99", got)
	}
}

func TestUploadedFileID_Nil(t *testing.T) {
	got := uploadedFileID(nil)

	if got != 0 {
		t.Errorf("uploadedFileID(nil) = %d, want 0", got)
	}
}

func TestLargestPhotoSize_Empty(t *testing.T) {
	got := largestPhotoSize(nil)

	if got != "x" {
		t.Errorf("largestPhotoSize(nil) = %q, want %q", got, "x")
	}
}

func TestLargestPhotoSize_Single(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "m"},
	}

	got := largestPhotoSize(sizes)

	if got != "m" {
		t.Errorf("largestPhotoSize(single) = %q, want %q", got, "m")
	}
}

func TestLargestPhotoSize_Multiple(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "s"},
		&tg.PhotoSize{Type: "m"},
		&tg.PhotoSizeProgressive{Type: "y"},
	}

	got := largestPhotoSize(sizes)

	if got != "y" {
		t.Errorf("largestPhotoSize(multiple) = %q, want %q", got, "y")
	}
}

func TestLargestPhotoSize_UnknownTypesOnly(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoStrippedSize{Type: "i"},
	}

	got := largestPhotoSize(sizes)

	if got != "x" {
		t.Errorf("largestPhotoSize(stripped only) = %q, want %q", got, "x")
	}
}

func TestDocumentFileName_WithAttr(t *testing.T) {
	doc := &tg.Document{
		ID: 100,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: "test.pdf"},
		},
	}

	got := documentFileName(doc)

	if got != "test.pdf" {
		t.Errorf("documentFileName() = %q, want %q", got, "test.pdf")
	}
}

func TestDocumentFileName_WithoutAttr(t *testing.T) {
	doc := &tg.Document{ID: 123}
	got := documentFileName(doc)
	want := fmt.Sprintf("document_%d", doc.ID)

	if got != want {
		t.Errorf("documentFileName() = %q, want %q", got, want)
	}
}

func TestDocumentFileName_OtherAttrs(t *testing.T) {
	doc := &tg.Document{
		ID: 456,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeAnimated{},
		},
	}
	got := documentFileName(doc)
	want := fmt.Sprintf("document_%d", doc.ID)

	if got != want {
		t.Errorf("documentFileName() = %q, want %q", got, want)
	}
}

func TestTypingAction_Default(t *testing.T) {
	got := typingAction("typing")

	if _, ok := got.(*tg.SendMessageTypingAction); !ok {
		t.Errorf("typingAction(typing) = %T, want *SendMessageTypingAction", got)
	}
}

func TestTypingAction_UploadPhoto(t *testing.T) {
	got := typingAction("uploading_photo")

	if _, ok := got.(*tg.SendMessageUploadPhotoAction); !ok {
		t.Errorf("typingAction(uploading_photo) = %T, want *SendMessageUploadPhotoAction", got)
	}
}

func TestTypingAction_RecordVoice(t *testing.T) {
	got := typingAction("recording_voice")

	if _, ok := got.(*tg.SendMessageRecordAudioAction); !ok {
		t.Errorf("typingAction(recording_voice) = %T, want *SendMessageRecordAudioAction", got)
	}
}

func TestTypingAction_UploadDocument(t *testing.T) {
	got := typingAction("uploading_document")

	if _, ok := got.(*tg.SendMessageUploadDocumentAction); !ok {
		t.Errorf("typingAction(uploading_document) = %T, want *SendMessageUploadDocumentAction", got)
	}
}

func TestTypingAction_ChooseSticker(t *testing.T) {
	got := typingAction("choosing_sticker")

	if _, ok := got.(*tg.SendMessageChooseStickerAction); !ok {
		t.Errorf("typingAction(choosing_sticker) = %T, want *SendMessageChooseStickerAction", got)
	}
}

func TestTypingAction_Cancel(t *testing.T) {
	got := typingAction("cancel")

	if _, ok := got.(*tg.SendMessageCancelAction); !ok {
		t.Errorf("typingAction(cancel) = %T, want *SendMessageCancelAction", got)
	}
}

func TestTypingAction_Unknown(t *testing.T) {
	got := typingAction("unknown_action")

	if _, ok := got.(*tg.SendMessageTypingAction); !ok {
		t.Errorf("typingAction(unknown) = %T, want *SendMessageTypingAction", got)
	}
}

func TestBuildReplyTo_BothZero(t *testing.T) {
	got := buildReplyTo(0, 0)
	if got != nil {
		t.Errorf("buildReplyTo(0, 0) = %+v, want nil", got)
	}
}

func TestBuildReplyTo_OnlyTopicID(t *testing.T) {
	got := buildReplyTo(42, 0)
	if got == nil {
		t.Fatal("buildReplyTo(42, 0) = nil, want non-nil")
	}

	topMsgID, hasTop := got.GetTopMsgID()
	if !hasTop || topMsgID != 42 {
		t.Errorf("TopMsgID = %d (set=%v), want 42", topMsgID, hasTop)
	}

	if got.ReplyToMsgID != 42 {
		t.Errorf("ReplyToMsgID = %d, want 42 (topic root)", got.ReplyToMsgID)
	}
}

func TestBuildReplyTo_OnlyReplyTo(t *testing.T) {
	got := buildReplyTo(0, 77)
	if got == nil {
		t.Fatal("buildReplyTo(0, 77) = nil, want non-nil")
	}

	if got.ReplyToMsgID != 77 {
		t.Errorf("ReplyToMsgID = %d, want 77", got.ReplyToMsgID)
	}

	topMsgID, hasTop := got.GetTopMsgID()
	if hasTop {
		t.Errorf("TopMsgID unexpectedly set to %d", topMsgID)
	}
}

func TestBuildReplyTo_BothSet(t *testing.T) {
	got := buildReplyTo(42, 77)
	if got == nil {
		t.Fatal("buildReplyTo(42, 77) = nil, want non-nil")
	}

	if got.ReplyToMsgID != 77 {
		t.Errorf("ReplyToMsgID = %d, want 77", got.ReplyToMsgID)
	}

	topMsgID, hasTop := got.GetTopMsgID()
	if !hasTop || topMsgID != 42 {
		t.Errorf("TopMsgID = %d (set=%v), want 42", topMsgID, hasTop)
	}
}

func TestChannelType_Channel(t *testing.T) {
	channel := &tg.Channel{Megagroup: false}
	got := channelType(channel)

	if got != wantChannel {
		t.Errorf("channelType(channel) = %q, want %q", got, wantChannel)
	}
}

func TestChannelType_Supergroup(t *testing.T) {
	channel := &tg.Channel{Megagroup: true}
	got := channelType(channel)

	if got != wantSupergroup {
		t.Errorf("channelType(supergroup) = %q, want %q", got, wantSupergroup)
	}
}

func TestBuildUserRefs_PopulatesNameAndUsername(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 10, FirstName: "Alice", LastName: "A", Username: "alice"},
		&tg.User{ID: 20, FirstName: "Bob"},
		&tg.UserEmpty{ID: 30},
	}

	refs := buildUserRefs(users)

	if refs[10].Name != "Alice A" {
		t.Errorf("refs[10].Name = %q, want %q", refs[10].Name, "Alice A")
	}

	if refs[10].Username != "alice" {
		t.Errorf("refs[10].Username = %q, want %q", refs[10].Username, "alice")
	}

	if refs[20].Username != "" {
		t.Errorf("refs[20].Username = %q, want empty", refs[20].Username)
	}

	if _, ok := refs[30]; ok {
		t.Errorf("refs[30] populated for UserEmpty, want absent")
	}
}

func TestBuildChatRefs_ChannelAndChat(t *testing.T) {
	chats := []tg.ChatClass{
		&tg.Channel{ID: 100, Title: "Public Channel", Username: "pub"},
		&tg.Channel{ID: 200, Title: "Private Channel"},
		&tg.Chat{ID: 300, Title: "Basic Group"},
	}

	refs := buildChatRefs(chats)

	if refs[100].Name != "Public Channel" || refs[100].Username != "pub" {
		t.Errorf("refs[100] = %+v, want {Public Channel, pub}", refs[100])
	}

	if refs[200].Username != "" {
		t.Errorf("refs[200].Username = %q, want empty for private channel", refs[200].Username)
	}

	if refs[300].Name != "Basic Group" {
		t.Errorf("refs[300].Name = %q, want %q", refs[300].Name, "Basic Group")
	}
}

func TestMessageFromUpdate_HandlesUpdatesCombined(t *testing.T) {
	// The server returns *tg.UpdatesCombined when the client's update
	// sequence diverges enough to need both Seq and SeqStart. Skipping
	// it would silently drop the just-sent message data — no ID, no
	// echo of the text — for SendMessage / EditMessage / Forward
	// callers reading the response.
	raw := &tg.Message{ID: 42, Date: 100, FromID: &tg.PeerUser{UserID: 10}}
	combined := &tg.UpdatesCombined{
		Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: raw}},
		Users:   []tg.UserClass{&tg.User{ID: 10, FirstName: "Bob", Username: "bob"}},
	}

	got := messageFromUpdate(combined, nil)
	if got == nil {
		t.Fatal("messageFromUpdate returned nil for *tg.UpdatesCombined")
	}

	if got.ID != 42 {
		t.Errorf("got ID = %d, want 42 — message data lost from UpdatesCombined", got.ID)
	}

	if got.FromName != "Bob" || got.FromUsername != "bob" {
		t.Errorf("got FromName=%q FromUsername=%q, want Bob/bob — UpdatesCombined was not enriched",
			got.FromName, got.FromUsername)
	}
}

func TestMessageFromUpdate_ShortSentMessageKeepsEntities(t *testing.T) {
	// The common echo for a plain text send is UpdateShortSentMessage,
	// and it carries the entities the server accepted. Dropping them
	// would blind any caller that wants to verify formatting parsed —
	// entitiesParsed would read 0 after a perfectly rendered send.
	short := &tg.UpdateShortSentMessage{ID: 42, Date: 100}
	short.SetEntities([]tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 4},
		&tg.MessageEntityCode{Offset: 5, Length: 3},
	})

	got := messageFromUpdate(short, nil)
	if got == nil {
		t.Fatal("messageFromUpdate returned nil for *tg.UpdateShortSentMessage")
	}

	if len(got.Entities) != 2 {
		t.Errorf("got %d entities, want 2 — the server echo was discarded", len(got.Entities))
	}
}

func TestMessageFromUpdate_EditMessageEcho(t *testing.T) {
	// messages.editMessage answers with UpdateEditMessage (or the
	// channel variant), not UpdateNewMessage. Without these cases the
	// whole EditMessage return value is nil and callers cannot see the
	// edited message's entities.
	raw := &tg.Message{ID: 7, Date: 100, Message: "edited"}
	raw.SetEntities([]tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 6}})

	updates := &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateEditMessage{Message: raw}},
	}

	got := editedMessageFromUpdate(updates, nil)
	if got == nil {
		t.Fatal("editedMessageFromUpdate returned nil for UpdateEditMessage")
	}

	if got.ID != 7 || len(got.Entities) != 1 {
		t.Errorf("got ID=%d entities=%d, want 7/1", got.ID, len(got.Entities))
	}
}

// TestMessagesFromUpdates_ScheduledMessages pins the scheduled path:
// a scheduled album (or forward) echoes updateNewScheduledMessage, and
// dropping those left the caller with count 0 and entitiesParsed 0 —
// which the documented contract reads as "nothing parsed".
func TestMessagesFromUpdates_ScheduledMessages(t *testing.T) {
	raw := &tg.Message{ID: 5, Date: 100, Message: "scheduled"}
	raw.SetEntities([]tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 9}})

	updates := &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateNewScheduledMessage{Message: raw}},
	}

	msgs := messagesFromUpdates(updates)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 — the scheduled echo was dropped", len(msgs))
	}

	if len(msgs[0].Entities) != 1 {
		t.Errorf("got %d entities, want 1", len(msgs[0].Entities))
	}
}

// TestShortSentEntities_FallsBackToSubmitted pins the repair of the one
// echo shape whose entity flag is unobservable: when the short echo
// carries no entities, the submitted set is reported instead.
func TestShortSentEntities_FallsBackToSubmitted(t *testing.T) {
	submitted := []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 4}}

	silent := &tg.UpdateShortSentMessage{ID: 1, Date: 100}

	got := messageFromUpdate(silent, submitted)
	if got == nil || len(got.Entities) != 1 {
		t.Errorf("a silent short echo must report the submitted entities, got %+v", got)
	}

	// An echo that DID carry entities is authoritative.
	echoed := &tg.UpdateShortSentMessage{ID: 1, Date: 100}
	echoed.SetEntities([]tg.MessageEntityClass{
		&tg.MessageEntityCode{Offset: 0, Length: 2},
		&tg.MessageEntityItalic{Offset: 3, Length: 2},
	})

	got = messageFromUpdate(echoed, submitted)
	if got == nil || len(got.Entities) != 2 {
		t.Errorf("a non-empty short echo must win, got %+v", got)
	}
}

// TestMessageFromUpdate_FullEchoZeroIsAuthoritative pins the scope of
// that fallback: on the full-updates path the server sends its own
// message, so a zero there means the server applied no entities — the
// exact signal entitiesParsed exists to carry. Filling it in from the
// submitted set would erase the signal.
func TestMessageFromUpdate_FullEchoZeroIsAuthoritative(t *testing.T) {
	submitted := []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 4}}

	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{Message: &tg.Message{ID: 3, Date: 100, Message: "no entities"}},
		},
	}

	got := messageFromUpdate(updates, submitted)
	if got == nil {
		t.Fatal("messageFromUpdate returned nil")
	}

	if len(got.Entities) != 0 {
		t.Errorf("a server echo reporting no entities must stay at 0, got %+v", got.Entities)
	}
}

// TestMessageFromUpdate_IgnoresEditUpdates pins that the send echo path
// is blind to edit updates: a send response can carry an edit update for
// the parent message (the topic root's reply-counter bump), and the send
// result must never report that parent's ID as the message it just sent.
// TestEditedMessageFromUpdate_IgnoresNewMessages is the mirror of
// TestMessageFromUpdate_IgnoresEditUpdates: an edit echo must report the
// edited message, never some other new message the envelope bundled.
func TestEditedMessageFromUpdate_IgnoresNewMessages(t *testing.T) {
	bundled := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 99, Date: 100, Message: "someone else"}},
		},
	}

	if got := editedMessageFromUpdate(bundled, nil); got != nil {
		t.Errorf("an edit echo without an edit update must yield nil, got ID=%d", got.ID)
	}
}

func TestMessageFromUpdate_IgnoresEditUpdates(t *testing.T) {
	parentOnly := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateEditChannelMessage{Message: &tg.Message{ID: 1, Date: 90, Message: "parent"}},
		},
	}

	if got := messageFromUpdate(parentOnly, nil); got != nil {
		t.Errorf("a send echo carrying only an edit update must yield nil, got ID=%d", got.ID)
	}
}

// TestMessageFromUpdate_PrefersNewOverEdit pins the ordering: an edit
// update for the parent may arrive BEFORE the new-message update, and
// the send result must still report the sent message.
func TestMessageFromUpdate_PrefersNewOverEdit(t *testing.T) {
	parent := &tg.Message{ID: 1, Date: 90, Message: "parent bumped"}
	sent := &tg.Message{ID: 2, Date: 100, Message: "the actual send"}

	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateEditChannelMessage{Message: parent},
			&tg.UpdateNewChannelMessage{Message: sent},
		},
	}

	got := messageFromUpdate(updates, nil)
	if got == nil {
		t.Fatal("messageFromUpdate returned nil")
	}

	if got.ID != 2 {
		t.Errorf("got ID=%d, want the new message 2, not the edited parent", got.ID)
	}
}

func TestMessageFromUpdate_EditChannelMessageEcho(t *testing.T) {
	raw := &tg.Message{ID: 9, Date: 100, Message: "edited"}

	updates := &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateEditChannelMessage{Message: raw}},
	}

	got := editedMessageFromUpdate(updates, nil)
	if got == nil {
		t.Fatal("editedMessageFromUpdate returned nil for UpdateEditChannelMessage")
	}

	if got.ID != 9 {
		t.Errorf("got ID=%d, want 9", got.ID)
	}
}

func TestMessageFromUpdate_EnrichesSenderFromUsersArray(t *testing.T) {
	// SendMessage/EditMessage/ForwardMessages return values flow through
	// messageFromUpdate; the single-message path must apply the same
	// name/username resolution as the history-read path so callers don't
	// see a half-populated Message just because the response shape was
	// tg.Updates instead of tg.MessagesMessages.
	raw := &tg.Message{ID: 7, Date: 100, FromID: &tg.PeerUser{UserID: 42}}
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: raw}},
		Users: []tg.UserClass{
			&tg.User{ID: 42, FirstName: "Alice", LastName: "A", Username: "alice"},
		},
	}

	got := messageFromUpdate(updates, nil)
	if got == nil {
		t.Fatal("messageFromUpdate returned nil")
	}

	if got.FromName != "Alice A" || got.FromUsername != "alice" {
		t.Errorf("got FromName=%q FromUsername=%q, want Alice A/alice — enrichment did not run on the single-message-update path",
			got.FromName, got.FromUsername)
	}
}

func TestEnrichUpdateMessage_FromIDZeroStaysZero(t *testing.T) {
	// enrichUpdateMessage passes InputPeer{} for the host peer because
	// UpdateNewMessage doesn't carry one. If fillSenderRef misread that
	// empty peer as 'present', a FromID==0 message would get promoted
	// to peer.ID==0 — a no-op in practice but a silent invariant bug.
	raw := &tg.Message{ID: 1, Date: 100} // FromID nil → 0 after ConvertMessage
	users := map[int64]peerRef{0: {Name: "should not match"}}

	got := enrichUpdateMessage(raw, users, nil)

	if got.FromID != 0 {
		t.Errorf("FromID = %d, want 0 — enrichUpdateMessage must not promote when host peer is empty", got.FromID)
	}

	if got.FromName != "" {
		t.Errorf("FromName = %q, want empty — must not pick up the {0: ...} sentinel entry", got.FromName)
	}
}

func TestFillSenderRef_FillsUsername(t *testing.T) {
	msg := &Message{FromID: 42}
	users := map[int64]peerRef{42: {Name: "Carol", Username: "carol"}}

	fillSenderRef(msg, users, nil, InputPeer{})

	if msg.FromName != "Carol" || msg.FromUsername != "carol" {
		t.Errorf("FromName=%q FromUsername=%q, want Carol/carol", msg.FromName, msg.FromUsername)
	}
}

func TestFillSenderRef_DMFallbackToPeer(t *testing.T) {
	msg := &Message{FromID: 0}
	users := map[int64]peerRef{55: {Name: "DM Partner"}}

	fillSenderRef(msg, users, nil, InputPeer{Type: PeerUser, ID: 55})

	if msg.FromID != 55 || msg.FromName != "DM Partner" {
		t.Errorf("FromID=%d FromName=%q, want 55/DM Partner", msg.FromID, msg.FromName)
	}

	if msg.FromType != PeerUser {
		t.Errorf("FromType = %d, want PeerUser", msg.FromType)
	}
}

func TestFillSenderRef_AnonymousChannelPostCarriesChannelType(t *testing.T) {
	msg := &Message{FromID: 0}
	chats := map[int64]peerRef{500: {Name: "Example Channel"}}

	fillSenderRef(msg, nil, chats, InputPeer{Type: PeerChannel, ID: 500})

	if msg.FromType != PeerChannel {
		t.Errorf("FromType = %d, want PeerChannel — anonymous channel post must keep host peer kind",
			msg.FromType)
	}

	if msg.FromID != 500 {
		t.Errorf("FromID = %d, want 500 (promoted from host peer)", msg.FromID)
	}
}

func TestFillSenderRef_EmptyLookupDoesNotOverwrite(t *testing.T) {
	msg := &Message{FromID: 42, FromName: "Preset", FromUsername: "preset"}
	users := map[int64]peerRef{42: {}} // entry exists but is empty

	fillSenderRef(msg, users, nil, InputPeer{})

	if msg.FromName != "Preset" || msg.FromUsername != "preset" {
		t.Errorf("empty lookup overwrote Preset/preset → %q/%q", msg.FromName, msg.FromUsername)
	}
}

func TestFillForwardRefs_CollidingIDsResolvesByType(t *testing.T) {
	// Same numeric ID 500 in both Users[] and Chats[]; the forward
	// targets a channel and must NOT pick up the user's name.
	msg := &Message{
		Forward: &ForwardInfo{From: &PeerRef{
			Peer: InputPeer{Type: PeerChannel, ID: 500},
		}},
	}
	users := map[int64]peerRef{500: {Name: "Wrong User", Username: "wrong"}}
	chats := map[int64]peerRef{500: {Name: "Correct Channel", Username: "correct"}}

	fillForwardRefs(msg, users, chats)

	if msg.Forward.From.Name != "Correct Channel" {
		t.Errorf("Forward.From.Name = %q, want %q — type-blind lookup let the user shadow the channel",
			msg.Forward.From.Name, "Correct Channel")
	}

	if msg.Forward.From.Username != "correct" {
		t.Errorf("Forward.From.Username = %q, want %q", msg.Forward.From.Username, "correct")
	}
}

func TestFillForwardRefs_ResolvesUser(t *testing.T) {
	msg := &Message{
		Forward: &ForwardInfo{From: &PeerRef{Peer: InputPeer{Type: PeerUser, ID: 777}}},
	}
	users := map[int64]peerRef{777: {Name: "Original Author", Username: "orig"}}

	fillForwardRefs(msg, users, nil)

	if msg.Forward.From.Name != "Original Author" {
		t.Errorf("Forward.From.Name = %q, want %q", msg.Forward.From.Name, "Original Author")
	}

	if msg.Forward.From.Username != "orig" {
		t.Errorf("Forward.From.Username = %q, want %q", msg.Forward.From.Username, "orig")
	}
}

func TestFillForwardRefs_ResolvesChannelTitle(t *testing.T) {
	msg := &Message{
		Forward: &ForwardInfo{From: &PeerRef{Peer: InputPeer{Type: PeerChannel, ID: 100}}},
	}
	chats := map[int64]peerRef{100: {Name: "Example Channel", Username: "examplechan"}}

	fillForwardRefs(msg, nil, chats)

	if msg.Forward.From.Name != "Example Channel" || msg.Forward.From.Username != "examplechan" {
		t.Errorf("Forward.From = %+v, want Example Channel/examplechan", msg.Forward.From)
	}
}

func TestFillForwardRefs_HiddenName_NoResolve(t *testing.T) {
	msg := &Message{Forward: &ForwardInfo{FromName: "Privacy Hidden Author"}}

	fillForwardRefs(msg, nil, nil)

	if msg.Forward.FromName != "Privacy Hidden Author" {
		t.Errorf("Forward.FromName = %q, want preserved", msg.Forward.FromName)
	}

	if msg.Forward.From != nil {
		t.Errorf("Forward.From = %+v, want nil for privacy-hidden", msg.Forward.From)
	}
}

func TestFillReplyToRef_CrossChat(t *testing.T) {
	other := InputPeer{Type: PeerUser, ID: 999}
	msg := &Message{ReplyTo: &ReplyToInfo{MessageID: 1, FromPeerID: &other}}
	users := map[int64]peerRef{999: {Name: "Other User", Username: "other"}}

	fillReplyToRef(msg, users, nil)

	if msg.ReplyTo.FromName != "Other User" || msg.ReplyTo.FromUsername != "other" {
		t.Errorf("ReplyTo = %+v, want Other User/other", msg.ReplyTo)
	}
}

func TestFillReplyToRef_SameChat_NoResolve(t *testing.T) {
	msg := &Message{ReplyTo: &ReplyToInfo{MessageID: 1}}

	fillReplyToRef(msg, nil, nil)

	if msg.ReplyTo.FromName != "" {
		t.Errorf("ReplyTo.FromName populated for same-chat reply, want empty")
	}
}
