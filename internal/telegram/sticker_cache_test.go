package telegram

import (
	"bytes"
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

const (
	stickerDocID   = 5181593617004757506
	stickerDocHash = 4242
	stickerSetName = "AnimatedEmojies"
)

func stickerFileRef() []byte {
	return []byte{1, 2, 3}
}

// stickerInvoker answers messages.getStickerSet with one document and
// captures the sendMedia request that a sticker send produces.
type stickerInvoker struct {
	sent *tg.MessagesSendMediaRequest
}

func (s *stickerInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.MessagesGetStickerSetRequest:
		return encodeResp(cannedStickerSet(), output)
	case *tg.MessagesSendMediaRequest:
		s.sent = req

		return encodeResp(&tg.Updates{}, output)
	default:
		return errUnexpectedRequest
	}
}

func cannedStickerSet() *tg.MessagesStickerSet {
	return &tg.MessagesStickerSet{
		Set: tg.StickerSet{ID: 1, Title: "Animated Emoji", ShortName: stickerSetName, Count: 1},
		Documents: []tg.DocumentClass{
			&tg.Document{
				ID:            stickerDocID,
				AccessHash:    stickerDocHash,
				FileReference: stickerFileRef(),
				MimeType:      "application/x-tgsticker",
				// Stickerset is a required field on
				// documentAttributeSticker#, not a conditional one.
				Attributes: []tg.DocumentAttributeClass{
					&tg.DocumentAttributeSticker{Alt: "👍", Stickerset: &tg.InputStickerSetEmpty{}},
				},
			},
		},
	}
}

func newStickerWrapper(inv *stickerInvoker) *Wrapper {
	return &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache(), stickers: NewStickerCache()}
}

// inputDocument#1abfb575 carries id, access_hash AND file_reference.
// Sending a bare id answers MEDIA_EMPTY, so the send must reuse the
// document the sticker set already handed us.
func TestSendSticker_CarriesAccessHashAndFileReference(t *testing.T) {
	inv := &stickerInvoker{}
	wrap := newStickerWrapper(inv)

	_, err := wrap.GetStickerSet(t.Context(), stickerSetName)
	if err != nil {
		t.Fatalf("GetStickerSet: %v", err)
	}

	_, err = wrap.SendSticker(t.Context(), targetPeer(), stickerDocID, nil)
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}

	if inv.sent == nil {
		t.Fatal("messages.sendMedia was never invoked")
	}

	media, ok := inv.sent.Media.(*tg.InputMediaDocument)
	if !ok {
		t.Fatalf("media is %T, want *tg.InputMediaDocument", inv.sent.Media)
	}

	doc, ok := media.ID.(*tg.InputDocument)
	if !ok {
		t.Fatalf("document is %T, want *tg.InputDocument", media.ID)
	}

	if doc.ID != stickerDocID {
		t.Errorf("document id = %d, want %d", doc.ID, stickerDocID)
	}

	if doc.AccessHash != stickerDocHash {
		t.Errorf("access hash = %d, want %d", doc.AccessHash, stickerDocHash)
	}

	if !bytes.Equal(doc.FileReference, stickerFileRef()) {
		t.Errorf("file reference = %v, want %v", doc.FileReference, stickerFileRef())
	}
}

// Without a cached document there is no access hash to send, and guessing
// one produces MEDIA_EMPTY from the server — a code that names neither
// the sticker nor the remedy.
func TestSendSticker_RejectsUnknownStickerBeforeRPC(t *testing.T) {
	inv := &stickerInvoker{}

	_, err := newStickerWrapper(inv).SendSticker(t.Context(), targetPeer(), stickerDocID, nil)
	if !errors.Is(err, ErrStickerNotCached) {
		t.Fatalf("error = %v, want ErrStickerNotCached", err)
	}

	if inv.sent != nil {
		t.Error("messages.sendMedia was invoked for an unknown sticker")
	}
}

func TestSendSticker_StillCarriesSendAs(t *testing.T) {
	inv := &stickerInvoker{}
	wrap := newStickerWrapper(inv)
	identity := sendAsIdentity()

	_, err := wrap.GetStickerSet(t.Context(), stickerSetName)
	if err != nil {
		t.Fatalf("GetStickerSet: %v", err)
	}

	_, err = wrap.SendSticker(t.Context(), targetPeer(), stickerDocID, &identity)
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}

	got, ok := inv.sent.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

func TestSendSticker_OmitsSendAsWhenNil(t *testing.T) {
	inv := &stickerInvoker{}
	wrap := newStickerWrapper(inv)

	_, err := wrap.GetStickerSet(t.Context(), stickerSetName)
	if err != nil {
		t.Fatalf("GetStickerSet: %v", err)
	}

	_, err = wrap.SendSticker(t.Context(), targetPeer(), stickerDocID, nil)
	if err != nil {
		t.Fatalf("SendSticker: %v", err)
	}

	if _, ok := inv.sent.GetSendAs(); ok {
		t.Error("send_as flag is set even though sendAs is nil")
	}
}

func TestStickerCache_StoresAndLooksUp(t *testing.T) {
	cache := NewStickerCache()

	if _, ok := cache.Lookup(stickerDocID); ok {
		t.Fatal("empty cache returned a document")
	}

	cache.StoreAll(cannedStickerSet().Documents)

	doc, ok := cache.Lookup(stickerDocID)
	if !ok {
		t.Fatal("stored document was not found")
	}

	if doc.AccessHash != stickerDocHash || !bytes.Equal(doc.FileReference, stickerFileRef()) {
		t.Errorf("cached document = %+v, want hash %d and ref %v", doc, stickerDocHash, stickerFileRef())
	}
}
