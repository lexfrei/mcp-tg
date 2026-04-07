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

	if got.ReplyToMsgID != 0 {
		t.Errorf("ReplyToMsgID = %d, want 0", got.ReplyToMsgID)
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
