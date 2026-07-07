package telegram

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

var errUnexpectedTranscriptionRequest = errors.New("unexpected transcription request")

type transcriptionInvoker struct {
	message                *tg.Message
	response               *tg.MessagesTranscribedAudio
	beforeTranscribeReturn func(context.Context) error
	transcribeCalls        atomic.Int32
}

func (i *transcriptionInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetMessagesRequest:
		return encodeTranscriptionTestResponse(&tg.MessagesMessages{
			Messages: []tg.MessageClass{i.message},
		}, output)
	case *tg.MessagesTranscribeAudioRequest:
		i.transcribeCalls.Add(1)

		if i.beforeTranscribeReturn != nil {
			err := i.beforeTranscribeReturn(ctx)
			if err != nil {
				return err
			}
		}

		return encodeTranscriptionTestResponse(i.response, output)
	default:
		return errors.Wrapf(errUnexpectedTranscriptionRequest, "%T", input)
	}
}

func encodeTranscriptionTestResponse(response bin.Encoder, output bin.Decoder) error {
	var buf bin.Buffer

	err := response.Encode(&buf)
	if err != nil {
		return errors.Wrap(err, "encode")
	}

	err = output.Decode(&buf)
	if err != nil {
		return errors.Wrap(err, "decode")
	}

	return nil
}

func TestTranscriptionBrokerDeliversMatchingUpdate(t *testing.T) {
	broker := NewTranscriptionBroker()
	subscription := broker.Subscribe(InputPeer{Type: PeerChannel, ID: 500, AccessHash: 99}, 42, 77)
	defer subscription.Cancel()

	err := broker.HandleUpdate(context.Background(), tg.Entities{}, &tg.UpdateTranscribedAudio{
		Peer:            &tg.PeerChannel{ChannelID: 500},
		MsgID:           42,
		TranscriptionID: 77,
		Text:            "decoded voice",
	})
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}

	select {
	case got := <-subscription.Updates:
		if got.Status != TranscriptionStatusCompleted {
			t.Errorf("Status = %q, want completed", got.Status)
		}
		if got.Text != "decoded voice" {
			t.Errorf("Text = %q, want decoded voice", got.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("matching update was not delivered")
	}
}

func TestTranscriptionBrokerIgnoresDifferentMessage(t *testing.T) {
	broker := NewTranscriptionBroker()
	subscription := broker.Subscribe(InputPeer{Type: PeerChannel, ID: 500}, 42, 77)
	defer subscription.Cancel()

	err := broker.HandleUpdate(context.Background(), tg.Entities{}, &tg.UpdateTranscribedAudio{
		Peer:            &tg.PeerChannel{ChannelID: 500},
		MsgID:           43,
		TranscriptionID: 77,
		Text:            "wrong message",
	})
	if err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}

	select {
	case got := <-subscription.Updates:
		t.Fatalf("unexpected update delivered: %+v", got)
	case <-time.After(10 * time.Millisecond):
	}
}

func TestTranscribeAudio_AllowsVideoNote(t *testing.T) {
	msgID := 51555
	invoker := &transcriptionInvoker{
		message: videoNoteMessage(msgID),
		response: &tg.MessagesTranscribedAudio{
			TranscriptionID: 77,
			Text:            "video note text",
		},
	}
	wrap := NewWrapperWithTranscriptionBroker(tg.NewClient(invoker), nil)

	got, err := wrap.TranscribeAudio(t.Context(), InputPeer{Type: PeerUser, ID: 42}, msgID, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := invoker.transcribeCalls.Load(); got != 1 {
		t.Fatalf("transcribe calls = %d, want 1", got)
	}
	if got.Status != TranscriptionStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Type != MessageTypeVideoNote {
		t.Errorf("Type = %q, want video_note", got.Type)
	}
	if got.Text != "video note text" {
		t.Errorf("Text = %q, want video note text", got.Text)
	}
}

func TestTranscribeAudio_PreservesVideoNoteTypeAfterPendingUpdate(t *testing.T) {
	msgID := 51555
	transcriptionID := int64(88)
	broker := NewTranscriptionBroker()
	invoker := &transcriptionInvoker{
		message: videoNoteMessage(msgID),
		response: &tg.MessagesTranscribedAudio{
			Pending:         true,
			TranscriptionID: transcriptionID,
		},
	}
	wrap := NewWrapperWithTranscriptionBroker(tg.NewClient(invoker), broker)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	resultCh := make(chan *Transcription, 1)
	errCh := make(chan error, 1)
	go func() {
		got, err := wrap.TranscribeAudio(ctx, InputPeer{Type: PeerUser, ID: 42}, msgID, time.Second)
		if err != nil {
			errCh <- err

			return
		}
		resultCh <- got
	}()

	if waitForTranscribeCall(ctx, invoker) != nil {
		t.Fatal("transcribe request was not started")
	}
	err := broker.HandleUpdate(ctx, tg.Entities{}, &tg.UpdateTranscribedAudio{
		Peer:            &tg.PeerUser{UserID: 42},
		MsgID:           msgID,
		TranscriptionID: transcriptionID,
		Text:            "updated video note text",
	})
	if err != nil {
		t.Fatalf("handle update: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case got := <-resultCh:
		if got.Status != TranscriptionStatusCompleted {
			t.Errorf("Status = %q, want completed", got.Status)
		}
		if got.Type != MessageTypeVideoNote {
			t.Errorf("Type = %q, want video_note", got.Type)
		}
		if got.Text != "updated video note text" {
			t.Errorf("Text = %q, want updated video note text", got.Text)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for transcription")
	}
}

func TestTranscribeAudio_DeliversUpdateArrivingBeforeRPCResponse(t *testing.T) {
	msgID := 51555
	transcriptionID := int64(88)
	broker := NewTranscriptionBroker()
	invoker := &transcriptionInvoker{
		message: videoNoteMessage(msgID),
		response: &tg.MessagesTranscribedAudio{
			Pending:         true,
			TranscriptionID: transcriptionID,
		},
		beforeTranscribeReturn: func(ctx context.Context) error {
			return broker.HandleUpdate(ctx, tg.Entities{}, &tg.UpdateTranscribedAudio{
				Peer:            &tg.PeerUser{UserID: 42},
				MsgID:           msgID,
				TranscriptionID: transcriptionID,
				Text:            "arrived before rpc response",
			})
		},
	}
	wrap := NewWrapperWithTranscriptionBroker(tg.NewClient(invoker), broker)

	got, err := wrap.TranscribeAudio(t.Context(), InputPeer{Type: PeerUser, ID: 42}, msgID, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Status != TranscriptionStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Pending {
		t.Error("Pending should be false after completed update")
	}
	if got.Type != MessageTypeVideoNote {
		t.Errorf("Type = %q, want video_note", got.Type)
	}
	if got.Text != "arrived before rpc response" {
		t.Errorf("Text = %q, want update text", got.Text)
	}
}

func TestTranscribeAudio_DoesNotCallTelegramForPlainVideo(t *testing.T) {
	msgID := 51555
	invoker := &transcriptionInvoker{
		message: plainVideoMessage(msgID),
		response: &tg.MessagesTranscribedAudio{
			TranscriptionID: 77,
			Text:            "should not be used",
		},
	}
	wrap := NewWrapperWithTranscriptionBroker(tg.NewClient(invoker), nil)

	got, err := wrap.TranscribeAudio(t.Context(), InputPeer{Type: PeerUser, ID: 42}, msgID, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := invoker.transcribeCalls.Load(); got != 0 {
		t.Fatalf("transcribe calls = %d, want 0", got)
	}
	if got.Status != TranscriptionStatusNotTranscribable {
		t.Errorf("Status = %q, want not_transcribable", got.Status)
	}
	if got.Type != MessageTypeVideo {
		t.Errorf("Type = %q, want video", got.Type)
	}
}

func waitForTranscribeCall(ctx context.Context, invoker *transcriptionInvoker) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if invoker.transcribeCalls.Load() > 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "waiting for transcribe call")
		case <-ticker.C:
		}
	}
}

func videoNoteMessage(id int) *tg.Message {
	return documentMessage(id, func(attr *tg.DocumentAttributeVideo) {
		attr.SetRoundMessage(true)
	})
}

func plainVideoMessage(id int) *tg.Message {
	return documentMessage(id, func(attr *tg.DocumentAttributeVideo) {})
}

func documentMessage(id int, configure func(*tg.DocumentAttributeVideo)) *tg.Message {
	attr := &tg.DocumentAttributeVideo{
		Duration: 3,
		W:        320,
		H:        320,
	}
	configure(attr)

	return &tg.Message{
		ID:     id,
		PeerID: &tg.PeerUser{UserID: 42},
		Media: &tg.MessageMediaDocument{
			Document: &tg.Document{
				Attributes: []tg.DocumentAttributeClass{attr},
			},
		},
	}
}
