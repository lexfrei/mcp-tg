package telegram

import (
	"bytes"
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

const (
	// viewMaxSide is the longest side of the photo size ViewMedia picks: big
	// enough to read text in a screenshot, small enough to stay a modest
	// payload once base64-encoded into a tool result.
	viewMaxSide = 1280

	// videoNoteSide is the side a round video note is declared with. The
	// official clients record 384×384 (older ones 240); the server needs a
	// size and does not check it against the file.
	videoNoteSide = 384

	mimeJPEG = "image/jpeg"

	mediaTypeVideo = "video"
)

// ErrNothingToView is returned when a message has no photo, image or
// thumbnail to show.
var ErrNothingToView = errors.New("message has no photo, image or thumbnail to view")

// ErrViewTooLarge is returned when the image to view exceeds the size limit.
var ErrViewTooLarge = errors.New("image is larger than the viewing limit; download it with tg_media_download instead")

// ErrNoMedia is returned when a message carries no media at all.
var ErrNoMedia = errors.New("message has no media")

// MediaInfo describes a message's media without downloading it.
type MediaInfo struct {
	Type      string  `json:"type"`
	MimeType  string  `json:"mimeType,omitempty"`
	Size      int64   `json:"size,omitempty"`
	FileName  string  `json:"fileName,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Voice     bool    `json:"voice,omitempty"`
	VideoNote bool    `json:"videoNote,omitempty"`
	Sticker   bool    `json:"sticker,omitempty"`
	Animated  bool    `json:"animated,omitempty"`
	Title     string  `json:"title,omitempty"`
	Performer string  `json:"performer,omitempty"`
}

// ViewedImage is an image read into memory for the model to look at.
type ViewedImage struct {
	Data     []byte
	MimeType string
}

// VoiceOpts configures SendVoice. VideoNote sends a round video message
// instead of a voice message; Duration is in seconds and may be 0.
type VoiceOpts struct {
	VideoNote bool
	Duration  int
	Send      SendOpts
}

// InlineResult is one answer of an inline bot query.
type InlineResult struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url,omitempty"`
}

// InlineResults is an inline bot's answer. QueryID is a decimal string: it is
// a 64-bit value that would lose precision as a JSON number (see the
// stickerFileId note in CLAUDE.md).
type InlineResults struct {
	QueryID    string         `json:"queryId"`
	NextOffset string         `json:"nextOffset,omitempty"`
	Results    []InlineResult `json:"results"`
}

// SendVoice sends an audio file as a voice message, or a video file as a
// round video note.
func (w *Wrapper) SendVoice(ctx context.Context, peer InputPeer, path string, opts *VoiceOpts) (*Message, error) {
	file, err := w.uploaderFor(opts.Send.Progress).FromPath(ctx, path)
	if err != nil {
		return nil, errors.Wrap(err, "uploading file")
	}

	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	req := &tg.MessagesSendMediaRequest{
		Peer: InputPeerToTG(peer),
		Media: &tg.InputMediaUploadedDocument{
			File:       file,
			MimeType:   mimeByPath(path),
			Attributes: []tg.DocumentAttributeClass{voiceAttribute(opts)},
		},
		RandomID: randID,
		Silent:   opts.Send.Silent,
	}

	if reply := buildReplyTo(opts.Send.TopicID, opts.Send.ReplyTo); reply != nil {
		req.ReplyTo = reply
	}

	applySendAs(opts.Send.SendAs, req.SetSendAs)

	result, err := w.api.MessagesSendMedia(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending voice message")
	}

	return messageFromUpdate(result, peer, randID, nil), nil
}

func voiceAttribute(opts *VoiceOpts) tg.DocumentAttributeClass {
	if opts.VideoNote {
		return &tg.DocumentAttributeVideo{
			RoundMessage: true,
			Duration:     float64(opts.Duration),
			W:            videoNoteSide,
			H:            videoNoteSide,
		}
	}

	return &tg.DocumentAttributeAudio{Voice: true, Duration: opts.Duration}
}

// GetMediaInfo describes a message's media without downloading it.
func (w *Wrapper) GetMediaInfo(ctx context.Context, peer InputPeer, msgID int) (*MediaInfo, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, err
	}

	switch media := raw.Media.(type) {
	case *tg.MessageMediaPhoto:
		return photoInfo(media), nil
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.(*tg.Document)
		if !ok {
			return nil, ErrNoMedia
		}

		return documentInfo(doc), nil
	case nil:
		return nil, ErrNoMedia
	default:
		return &MediaInfo{Type: strings.TrimPrefix(media.TypeName(), "messageMedia")}, nil
	}
}

// ViewMedia reads a message's photo — or an image document, or failing that
// a document's thumbnail — into memory, refusing anything over maxBytes.
func (w *Wrapper) ViewMedia(ctx context.Context, peer InputPeer, msgID, maxBytes int) (*ViewedImage, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, err
	}

	location, mimeType, size := viewLocation(raw.Media, maxBytes)
	if location == nil {
		return nil, ErrNothingToView
	}

	if size > maxBytes {
		return nil, ErrViewTooLarge
	}

	buf := &cappedBuffer{limit: maxBytes}

	_, err = w.down.Download(w.api, location).Stream(ctx, buf)
	if err != nil {
		if errors.Is(err, ErrViewTooLarge) {
			return nil, ErrViewTooLarge
		}

		return nil, errors.Wrap(err, "downloading image")
	}

	return &ViewedImage{Data: buf.Bytes(), MimeType: mimeType}, nil
}

// InlineQuery asks an inline bot (@gif, @vid, @wiki, ...) for results. peer
// is the chat the results are meant for; a result must be sent to that same
// chat.
func (w *Wrapper) InlineQuery(
	ctx context.Context, bot, peer InputPeer, query, offset string,
) (*InlineResults, error) {
	answer, err := w.api.MessagesGetInlineBotResults(ctx, &tg.MessagesGetInlineBotResultsRequest{
		Bot:    InputUserFromPeer(bot),
		Peer:   InputPeerToTG(peer),
		Query:  query,
		Offset: offset,
	})
	if err != nil {
		return nil, errors.Wrap(err, "querying inline bot")
	}

	results := make([]InlineResult, 0, len(answer.Results))
	for _, result := range answer.Results {
		results = append(results, convertInlineResult(result))
	}

	return &InlineResults{
		QueryID:    strconv.FormatInt(answer.QueryID, 10),
		NextOffset: answer.NextOffset,
		Results:    results,
	}, nil
}

// SendInlineResult sends one result of an earlier InlineQuery.
func (w *Wrapper) SendInlineResult(
	ctx context.Context, peer InputPeer, queryID int64, resultID string, opts SendOpts,
) (*Message, error) {
	randID, err := cryptoRandID()
	if err != nil {
		return nil, err
	}

	req := &tg.MessagesSendInlineBotResultRequest{
		Peer:     InputPeerToTG(peer),
		RandomID: randID,
		QueryID:  queryID,
		ID:       resultID,
		Silent:   opts.Silent,
	}

	if reply := buildReplyTo(opts.TopicID, opts.ReplyTo); reply != nil {
		req.ReplyTo = reply
	}

	applySendAs(opts.SendAs, req.SetSendAs)

	result, err := w.api.MessagesSendInlineBotResult(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "sending inline result")
	}

	return messageFromUpdate(result, peer, randID, nil), nil
}

func convertInlineResult(result tg.BotInlineResultClass) InlineResult {
	switch typed := result.(type) {
	case *tg.BotInlineResult:
		return InlineResult{
			ID: typed.ID, Type: typed.Type, Title: typed.Title, Description: typed.Description, URL: typed.URL,
		}
	case *tg.BotInlineMediaResult:
		return InlineResult{ID: typed.ID, Type: typed.Type, Title: typed.Title, Description: typed.Description}
	default:
		return InlineResult{}
	}
}

func photoInfo(media *tg.MessageMediaPhoto) *MediaInfo {
	info := &MediaInfo{Type: "photo", MimeType: mimeJPEG}

	photo, ok := media.Photo.(*tg.Photo)
	if !ok {
		return info
	}

	for _, size := range photo.Sizes {
		width, height, byteSize := photoSizeDims(size)
		if width*height > info.Width*info.Height {
			info.Width, info.Height, info.Size = width, height, int64(byteSize)
		}
	}

	return info
}

func documentInfo(doc *tg.Document) *MediaInfo {
	info := &MediaInfo{Type: "document", MimeType: doc.MimeType, Size: doc.Size}

	for _, attr := range doc.Attributes {
		applyDocumentAttribute(info, attr)
	}

	return info
}

func applyDocumentAttribute(info *MediaInfo, attr tg.DocumentAttributeClass) {
	switch typed := attr.(type) {
	case *tg.DocumentAttributeFilename:
		info.FileName = filepath.Base(typed.FileName)
	case *tg.DocumentAttributeAudio:
		// A video note carries an audio attribute next to its video one;
		// the video attribute names the kind, whichever comes first.
		if info.Type != mediaTypeVideo {
			info.Type, info.Duration = "audio", float64(typed.Duration)
		}

		info.Voice, info.Title, info.Performer = typed.Voice, typed.Title, typed.Performer
	case *tg.DocumentAttributeVideo:
		info.Type, info.Duration, info.VideoNote = mediaTypeVideo, typed.Duration, typed.RoundMessage
		info.Width, info.Height = typed.W, typed.H
	case *tg.DocumentAttributeImageSize:
		info.Width, info.Height = typed.W, typed.H
	case *tg.DocumentAttributeSticker:
		info.Type, info.Sticker = "sticker", true
	case *tg.DocumentAttributeAnimated:
		info.Animated = true
	}
}

// viewLocation picks what ViewMedia downloads: a photo at viewing size, an
// image document whole, or a document's thumbnail.
//
//nolint:gocritic // unnamedResult: location, MIME type and size read clearly by position.
func viewLocation(media tg.MessageMediaClass, maxBytes int) (tg.InputFileLocationClass, string, int) {
	switch typed := media.(type) {
	case *tg.MessageMediaPhoto:
		photo, ok := typed.Photo.(*tg.Photo)
		if !ok || len(photo.Sizes) == 0 {
			return nil, "", 0
		}

		thumb, size := viewPhotoSize(photo.Sizes)

		return &tg.InputPhotoFileLocation{
			ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: thumb,
		}, mimeJPEG, size
	case *tg.MessageMediaDocument:
		doc, ok := typed.Document.(*tg.Document)
		if !ok {
			return nil, "", 0
		}

		return documentViewLocation(doc, maxBytes)
	default:
		return nil, "", 0
	}
}

//nolint:gocritic // unnamedResult: see viewLocation.
func documentViewLocation(doc *tg.Document, maxBytes int) (tg.InputFileLocationClass, string, int) {
	location := &tg.InputDocumentFileLocation{ID: doc.ID, AccessHash: doc.AccessHash, FileReference: doc.FileReference}

	if strings.HasPrefix(doc.MimeType, "image/") && doc.Size <= int64(maxBytes) {
		return location, doc.MimeType, int(doc.Size)
	}

	if len(doc.Thumbs) == 0 {
		return nil, "", 0
	}

	thumb, size := viewPhotoSize(doc.Thumbs)
	location.ThumbSize = thumb

	return location, mimeJPEG, size
}

// viewPhotoSize returns the type letter and byte size of the largest photo
// size whose longer side fits viewMaxSide, or of the smallest size when none
// fits. Stripped and path sizes carry no real image and are skipped.
//
//nolint:gocritic // unnamedResult: type and size read clearly by position.
func viewPhotoSize(sizes []tg.PhotoSizeClass) (string, int) {
	var (
		bestType, smallestType string
		bestArea, bestSize     int
		smallestArea           = -1
		smallestSize           int
	)

	for _, size := range sizes {
		width, height, byteSize := photoSizeDims(size)
		if width == 0 || height == 0 {
			continue
		}

		area := width * height

		if smallestArea < 0 || area < smallestArea {
			smallestArea, smallestType, smallestSize = area, size.GetType(), byteSize
		}

		if max(width, height) <= viewMaxSide && area > bestArea {
			bestArea, bestType, bestSize = area, size.GetType(), byteSize
		}
	}

	if bestType != "" {
		return bestType, bestSize
	}

	return smallestType, smallestSize
}

//nolint:gocritic // unnamedResult: width, height and byte size read clearly by position.
func photoSizeDims(size tg.PhotoSizeClass) (int, int, int) {
	switch typed := size.(type) {
	case *tg.PhotoSize:
		return typed.W, typed.H, typed.Size
	case *tg.PhotoSizeProgressive:
		largest := 0
		if len(typed.Sizes) > 0 {
			largest = typed.Sizes[len(typed.Sizes)-1]
		}

		return typed.W, typed.H, largest
	case *tg.PhotoCachedSize:
		return typed.W, typed.H, len(typed.Bytes)
	default:
		return 0, 0, 0
	}
}

// cappedBuffer collects a download in memory and fails once it passes
// limit, so a lying size field cannot make ViewMedia buffer a huge file.
type cappedBuffer struct {
	bytes.Buffer

	limit int
}

func (buf *cappedBuffer) Write(chunk []byte) (int, error) {
	if buf.Len()+len(chunk) > buf.limit {
		return 0, ErrViewTooLarge
	}

	n, err := buf.Buffer.Write(chunk)

	return n, errors.Wrap(err, "buffering image")
}
