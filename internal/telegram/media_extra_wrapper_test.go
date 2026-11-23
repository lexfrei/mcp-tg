package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// mediaMessageInvoker serves one message for every message read.
type mediaMessageInvoker struct{ message *tg.Message }

func (m *mediaMessageInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetMessagesRequest); !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	return encodeAndDecode(&tg.MessagesMessages{Messages: []tg.MessageClass{m.message}}, output)
}

func mediaWrapper(media tg.MessageMediaClass) *Wrapper {
	msg := &tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 5}}
	if media != nil {
		msg.SetMedia(media)
	}

	return newWrapperWithInvoker(&mediaMessageInvoker{message: msg})
}

var mediaPeer = InputPeer{Type: PeerUser, ID: 5, AccessHash: 50}

func documentMedia(mime string, attrs ...tg.DocumentAttributeClass) *tg.MessageMediaDocument {
	return &tg.MessageMediaDocument{Document: &tg.Document{
		ID: 1, MimeType: mime, Attributes: attrs, FileReference: []byte{1}, Thumbs: nil,
	}}
}

// The type label is MessagePayloadType's, the one tg_messages_list reports.
func TestGetMediaInfo_UsesTheMessageTypeLabels(t *testing.T) {
	for want, media := range map[string]tg.MessageMediaClass{
		MessageTypeVoice: documentMedia("audio/ogg", &tg.DocumentAttributeAudio{Voice: true, Duration: 3}),
		MessageTypeVideoNote: documentMedia("video/mp4",
			&tg.DocumentAttributeVideo{RoundMessage: true, Duration: 3, W: 384, H: 384},
			&tg.DocumentAttributeAudio{Duration: 3}),
		MessagePayloadType(&tg.MessageMediaGeo{Geo: &tg.GeoPointEmpty{}}): &tg.MessageMediaGeo{Geo: &tg.GeoPointEmpty{}},
	} {
		info, err := mediaWrapper(media).GetMediaInfo(t.Context(), mediaPeer, 10)
		if err != nil || info.Type != want {
			t.Errorf("%T: info = %+v, err = %v; want type %q", media, info, err, want)
		}
	}
}

func TestGetMediaInfo_NoMedia(t *testing.T) {
	for name, media := range map[string]tg.MessageMediaClass{
		"text message":   nil,
		"empty document": &tg.MessageMediaDocument{Document: &tg.DocumentEmpty{ID: 1}},
	} {
		_, err := mediaWrapper(media).GetMediaInfo(t.Context(), mediaPeer, 10)
		if !errors.Is(err, ErrNoMedia) {
			t.Errorf("%s: err = %v, want ErrNoMedia", name, err)
		}
	}
}

// A photo whose viewing size is over the budget is refused before any
// download.
func TestViewMedia_RefusesWhatIsTooLarge(t *testing.T) {
	photo := &tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 4, FileReference: []byte{1}, Sizes: []tg.PhotoSizeClass{
		&tg.PhotoSize{Type: "y", W: 1280, H: 960, Size: 9000},
	}}}

	_, err := mediaWrapper(photo).ViewMedia(t.Context(), mediaPeer, 10, 5000)
	if !errors.Is(err, ErrViewTooLarge) {
		t.Errorf("err = %v, want ErrViewTooLarge", err)
	}
}

func TestViewMedia_NothingToView(t *testing.T) {
	_, err := mediaWrapper(nil).ViewMedia(t.Context(), mediaPeer, 10, 5000)
	if !errors.Is(err, ErrNothingToView) {
		t.Errorf("err = %v, want ErrNothingToView", err)
	}
}
