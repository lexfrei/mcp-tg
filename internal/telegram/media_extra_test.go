package telegram

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// The viewing size is the largest that fits 1280 px, not the largest there
// is: a 2560 px original would make a needlessly heavy tool result.
func TestViewPhotoSize_PicksLargestThatFits(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoStrippedSize{Type: "i"},
		&tg.PhotoSize{Type: "m", W: 320, H: 240, Size: 10},
		&tg.PhotoSize{Type: "y", W: 1280, H: 960, Size: 200},
		&tg.PhotoSizeProgressive{Type: "w", W: 2560, H: 1920, Sizes: []int{100, 900}},
	}

	if thumb, size := viewPhotoSize(sizes); thumb != "y" || size != 200 {
		t.Errorf("viewPhotoSize = %q, %d; want y, 200", thumb, size)
	}
}

func TestViewPhotoSize_FallsBackToSmallest(t *testing.T) {
	sizes := []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "w", W: 2560, H: 1920, Size: 900},
		&tg.PhotoSize{Type: "z", W: 2000, H: 1500, Size: 600},
	}

	if thumb, _ := viewPhotoSize(sizes); thumb != "z" {
		t.Errorf("viewPhotoSize = %q, want the smallest, z", thumb)
	}
}

func TestDocumentViewLocation(t *testing.T) {
	image := &tg.Document{ID: 1, MimeType: "image/png", Size: 1000}

	location, mime, size := documentViewLocation(image, 5000)
	if location == nil || mime != "image/png" || size != 1000 {
		t.Errorf("small image: %v, %q, %d; want the whole document", location, mime, size)
	}

	video := &tg.Document{ID: 2, MimeType: "video/mp4", Size: 1 << 30, Thumbs: []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "m", W: 320, H: 180, Size: 20},
	}}

	location, mime, _ = documentViewLocation(video, 5000)

	docLocation, ok := location.(*tg.InputDocumentFileLocation)
	if !ok || docLocation.ThumbSize != "m" || mime != mimeJPEG {
		t.Errorf("video: %+v, %q; want its m thumbnail as JPEG", location, mime)
	}

	if location, _, _ := documentViewLocation(&tg.Document{MimeType: "application/zip"}, 5000); location != nil {
		t.Error("a document with no thumbnail must have nothing to view")
	}
}

func TestDocumentInfo(t *testing.T) {
	info := documentInfo(&tg.Document{MimeType: "audio/ogg", Size: 4096, Attributes: []tg.DocumentAttributeClass{
		&tg.DocumentAttributeAudio{Voice: true, Duration: 12},
		&tg.DocumentAttributeFilename{FileName: "../../voice.ogg"},
	}})

	if info.Type != "audio" || !info.Voice || info.Duration != 12 || info.FileName != "voice.ogg" || info.Size != 4096 {
		t.Errorf("info = %+v", info)
	}
}

// Verified live: Telegram stores a video note with an audio attribute AFTER
// the video one, and it must not turn the note into "audio".
func TestDocumentInfo_VideoNoteStaysVideo(t *testing.T) {
	info := documentInfo(&tg.Document{MimeType: "video/mp4", Attributes: []tg.DocumentAttributeClass{
		&tg.DocumentAttributeVideo{RoundMessage: true, Duration: 3, W: 384, H: 384},
		&tg.DocumentAttributeAudio{Duration: 3},
	}})

	if info.Type != "video" || !info.VideoNote || info.Duration != 3 {
		t.Errorf("info = %+v, want a video note", info)
	}
}

func TestVoiceAttribute(t *testing.T) {
	audio, ok := voiceAttribute(&VoiceOpts{Duration: 5}).(*tg.DocumentAttributeAudio)
	if !ok || !audio.Voice || audio.Duration != 5 {
		t.Errorf("voice attribute = %+v", audio)
	}

	video, ok := voiceAttribute(&VoiceOpts{VideoNote: true, Duration: 7}).(*tg.DocumentAttributeVideo)
	if !ok || !video.RoundMessage || video.W == 0 || video.Duration != 7 {
		t.Errorf("video note attribute = %+v", video)
	}
}

func TestCappedBuffer_StopsAtTheLimit(t *testing.T) {
	buf := &cappedBuffer{limit: 4}

	_, err := buf.Write([]byte("abc"))
	if err != nil {
		t.Fatalf("write within limit: %v", err)
	}

	_, err = buf.Write([]byte("de"))
	if !errors.Is(err, ErrViewTooLarge) {
		t.Errorf("write past limit: err = %v, want ErrViewTooLarge", err)
	}
}

func TestConvertInlineResult(t *testing.T) {
	got := convertInlineResult(&tg.BotInlineResult{ID: "7", Type: "gif", Title: "cat", URL: "https://example.org/cat"})
	if got.ID != "7" || got.Type != "gif" || got.URL != "https://example.org/cat" {
		t.Errorf("result = %+v", got)
	}

	media := convertInlineResult(&tg.BotInlineMediaResult{ID: "8", Type: "photo", Title: "dog"})
	if media.ID != "8" || media.Title != "dog" {
		t.Errorf("media result = %+v", media)
	}
}
