package telegram

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// registerVideoMIME makes video extension detection deterministic regardless of
// the host's system MIME database (CI containers often lack /etc/mime.types).
func registerVideoMIME(t *testing.T) {
	t.Helper()

	_ = mime.AddExtensionType(".mp4", "video/mp4")
	_ = mime.AddExtensionType(".mov", "video/quicktime")
}

func TestAlbumIsVisual(t *testing.T) {
	registerVideoMIME(t)

	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"all images", []string{"a.png", "b.jpg"}, true},
		{"all videos", []string{"a.mp4", "b.mov"}, true},
		{"mixed image and video", []string{"a.png", "b.mp4"}, true},
		{"image and document", []string{"a.png", "b.pdf"}, false},
		{"single document", []string{"a.pdf"}, false},
		{"extensionless", []string{"noext"}, false},
		{"empty", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := albumIsVisual(tt.paths); got != tt.want {
				t.Errorf("albumIsVisual(%v) = %v, want %v", tt.paths, got, tt.want)
			}
		})
	}
}

func TestUploadedAlbumMedia(t *testing.T) {
	registerVideoMIME(t)

	file := &tg.InputFile{ID: 1, Parts: 1, Name: "x"}

	t.Run("visual image is a photo", func(t *testing.T) {
		got := uploadedAlbumMedia(file, "a.png", true)
		if _, ok := got.(*tg.InputMediaUploadedPhoto); !ok {
			t.Fatalf("visual image: got %T, want *tg.InputMediaUploadedPhoto", got)
		}
	})

	t.Run("visual video is a document with video attribute", func(t *testing.T) {
		got := uploadedAlbumMedia(file, "a.mp4", true)

		doc, ok := got.(*tg.InputMediaUploadedDocument)
		if !ok {
			t.Fatalf("visual video: got %T, want *tg.InputMediaUploadedDocument", got)
		}

		hasVideoAttr := false

		for _, attr := range doc.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
				hasVideoAttr = true
			}
		}

		if !hasVideoAttr {
			t.Errorf("visual video document missing *tg.DocumentAttributeVideo, attrs=%v", doc.Attributes)
		}
	})

	t.Run("non-visual image is a plain document", func(t *testing.T) {
		got := uploadedAlbumMedia(file, "a.png", false)

		doc, ok := got.(*tg.InputMediaUploadedDocument)
		if !ok {
			t.Fatalf("non-visual image: got %T, want *tg.InputMediaUploadedDocument", got)
		}

		for _, attr := range doc.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
				t.Errorf("non-visual document must not carry a video attribute")
			}
		}
	})
}

func TestInputMediaFromUploaded(t *testing.T) {
	photo := &tg.Photo{ID: 10, AccessHash: 20, FileReference: []byte{1, 2, 3}}
	mediaPhoto := &tg.MessageMediaPhoto{}
	mediaPhoto.SetPhoto(photo)

	doc := &tg.Document{ID: 30, AccessHash: 40, FileReference: []byte{4, 5, 6}}
	mediaDoc := &tg.MessageMediaDocument{}
	mediaDoc.SetDocument(doc)

	emptyPhoto := &tg.MessageMediaPhoto{}
	emptyPhoto.SetPhoto(&tg.PhotoEmpty{ID: 1})

	emptyDoc := &tg.MessageMediaDocument{}
	emptyDoc.SetDocument(&tg.DocumentEmpty{ID: 1})

	t.Run("photo converts to input photo", func(t *testing.T) {
		got, err := inputMediaFromUploaded(mediaPhoto)
		if err != nil {
			t.Fatalf("inputMediaFromUploaded(photo) error: %v", err)
		}

		in, ok := got.(*tg.InputMediaPhoto)
		if !ok {
			t.Fatalf("got %T, want *tg.InputMediaPhoto", got)
		}

		id, ok := in.ID.(*tg.InputPhoto)
		if !ok {
			t.Fatalf("ID is %T, want *tg.InputPhoto", in.ID)
		}

		if id.ID != 10 || id.AccessHash != 20 || string(id.FileReference) != string([]byte{1, 2, 3}) {
			t.Errorf("input photo = %+v, want id=10 hash=20 ref=[1 2 3]", id)
		}
	})

	t.Run("document converts to input document", func(t *testing.T) {
		got, err := inputMediaFromUploaded(mediaDoc)
		if err != nil {
			t.Fatalf("inputMediaFromUploaded(document) error: %v", err)
		}

		in, ok := got.(*tg.InputMediaDocument)
		if !ok {
			t.Fatalf("got %T, want *tg.InputMediaDocument", got)
		}

		id, ok := in.ID.(*tg.InputDocument)
		if !ok {
			t.Fatalf("ID is %T, want *tg.InputDocument", in.ID)
		}

		if id.ID != 30 || id.AccessHash != 40 || string(id.FileReference) != string([]byte{4, 5, 6}) {
			t.Errorf("input document = %+v, want id=30 hash=40 ref=[4 5 6]", id)
		}
	})

	errorCases := map[string]tg.MessageMediaClass{
		"empty photo":      emptyPhoto,
		"empty document":   emptyDoc,
		"photo flag unset": &tg.MessageMediaPhoto{},
		"unexpected media": &tg.MessageMediaEmpty{},
	}

	for name, media := range errorCases {
		t.Run(name+" errors", func(t *testing.T) {
			_, err := inputMediaFromUploaded(media)
			if err == nil {
				t.Errorf("inputMediaFromUploaded(%s) = nil error, want error", name)
			}
		})
	}
}

// albumInvoker is a routing fake that answers the three RPCs SendAlbum issues:
// upload.saveFilePart, messages.uploadMedia, messages.sendMultiMedia.
type albumInvoker struct {
	mu          sync.Mutex
	uploadMedia int
	sentReq     *tg.MessagesSendMultiMediaRequest
}

func (a *albumInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch req := input.(type) {
	case *tg.UploadSaveFilePartRequest:
		return encodeResp(&tg.BoolTrue{}, output)
	case *tg.MessagesUploadMediaRequest:
		a.uploadMedia++

		return encodeResp(uploadMediaResponse(req.Media), output)
	case *tg.MessagesSendMultiMediaRequest:
		a.sentReq = req

		return encodeResp(&tg.Updates{}, output)
	default:
		return errUnexpectedRequest
	}
}

func uploadMediaResponse(media tg.InputMediaClass) tg.MessageMediaClass {
	if _, ok := media.(*tg.InputMediaUploadedPhoto); ok {
		out := &tg.MessageMediaPhoto{}
		out.SetPhoto(&tg.Photo{ID: 100, AccessHash: 200, FileReference: []byte{9}})

		return out
	}

	out := &tg.MessageMediaDocument{}
	out.SetDocument(&tg.Document{ID: 300, AccessHash: 400, FileReference: []byte{8}})

	return out
}

func encodeResp(enc bin.Encoder, output bin.Decoder) error {
	var buf bin.Buffer

	err := enc.Encode(&buf)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	err = output.Decode(&buf)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}

func TestSendAlbum_FinalizesUploadedMedia(t *testing.T) {
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")}

	for _, p := range paths {
		err := os.WriteFile(p, []byte("payload"), 0o600)
		if err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	inv := &albumInvoker{}
	api := tg.NewClient(inv)
	wrap := &Wrapper{api: api, up: uploader.NewUploader(api), cache: NewPeerCache()}

	_, err := wrap.SendAlbum(
		t.Context(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		paths,
		"caption text",
		SendOpts{},
	)
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}

	if inv.uploadMedia != len(paths) {
		t.Errorf("uploadMedia calls = %d, want %d (one per item)", inv.uploadMedia, len(paths))
	}

	if inv.sentReq == nil {
		t.Fatal("sendMultiMedia was never called")
	}

	if len(inv.sentReq.MultiMedia) != len(paths) {
		t.Fatalf("MultiMedia len = %d, want %d", len(inv.sentReq.MultiMedia), len(paths))
	}

	for i, item := range inv.sentReq.MultiMedia {
		// Must be finalized referenced media, never freshly-uploaded media —
		// the latter is exactly what triggers MEDIA_INVALID.
		if _, ok := item.Media.(*tg.InputMediaPhoto); !ok {
			t.Errorf("item %d media is %T, want *tg.InputMediaPhoto (finalized)", i, item.Media)
		}
	}

	if inv.sentReq.MultiMedia[0].Message != "caption text" {
		t.Errorf("first item caption = %q, want %q", inv.sentReq.MultiMedia[0].Message, "caption text")
	}

	if inv.sentReq.MultiMedia[1].Message != "" {
		t.Errorf("second item caption = %q, want empty (caption is first-item only)", inv.sentReq.MultiMedia[1].Message)
	}
}
