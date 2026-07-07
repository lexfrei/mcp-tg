package telegram

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

type transcriptionKey struct {
	peerType        PeerType
	peerID          int64
	msgID           int
	transcriptionID int64
}

// TranscriptionBroker routes Telegram updateTranscribedAudio updates to
// callers waiting for a specific transcription request.
type TranscriptionBroker struct {
	mu      sync.Mutex
	waiters map[transcriptionKey]map[chan Transcription]struct{}
}

// TranscriptionSubscription contains a transcription update stream and its cancel function.
type TranscriptionSubscription struct {
	// Updates receives matching transcription updates.
	Updates <-chan Transcription
	// Cancel unregisters the subscription.
	Cancel func()
}

// NewTranscriptionBroker creates a transcription update broker.
func NewTranscriptionBroker() *TranscriptionBroker {
	return &TranscriptionBroker{
		waiters: make(map[transcriptionKey]map[chan Transcription]struct{}),
	}
}

// Subscribe registers interest in one transcription update stream.
func (b *TranscriptionBroker) Subscribe(
	peer InputPeer, msgID int, transcriptionID int64,
) TranscriptionSubscription {
	updateCh := make(chan Transcription, 1)
	key := makeTranscriptionKey(peer, msgID, transcriptionID)

	b.mu.Lock()
	if b.waiters[key] == nil {
		b.waiters[key] = make(map[chan Transcription]struct{})
	}

	b.waiters[key][updateCh] = struct{}{}
	b.mu.Unlock()

	var once sync.Once

	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			delete(b.waiters[key], updateCh)

			if len(b.waiters[key]) == 0 {
				delete(b.waiters, key)
			}
		})
	}

	return TranscriptionSubscription{
		Updates: updateCh,
		Cancel:  cancel,
	}
}

// HandleUpdate satisfies tg.TranscribedAudioHandler.
func (b *TranscriptionBroker) HandleUpdate(
	_ context.Context, _ tg.Entities, update *tg.UpdateTranscribedAudio,
) error {
	if b == nil || update == nil {
		return nil
	}

	peer := extractPeerID(update.Peer)
	if peer.ID == 0 {
		return nil
	}

	transcription := transcriptionFromUpdate(update)
	keys := matchingTranscriptionKeys(peer, transcription.MessageID, transcription.TranscriptionID)

	b.mu.Lock()
	waiters := b.waitersForKeys(keys)

	if !transcription.Pending {
		for _, key := range keys {
			delete(b.waiters, key)
		}
	}

	b.mu.Unlock()

	for _, updateCh := range waiters {
		deliverTranscriptionUpdate(updateCh, &transcription)
	}

	return nil
}

func matchingTranscriptionKeys(peer InputPeer, msgID int, transcriptionID int64) []transcriptionKey {
	keys := []transcriptionKey{makeTranscriptionKey(peer, msgID, transcriptionID)}
	if transcriptionID != 0 {
		keys = append(keys, makeTranscriptionKey(peer, msgID, 0))
	}

	return keys
}

func (b *TranscriptionBroker) waitersForKeys(keys []transcriptionKey) []chan Transcription {
	seen := make(map[chan Transcription]struct{})
	waiters := make([]chan Transcription, 0)

	for _, key := range keys {
		for updateCh := range b.waiters[key] {
			if _, ok := seen[updateCh]; ok {
				continue
			}

			seen[updateCh] = struct{}{}
			waiters = append(waiters, updateCh)
		}
	}

	return waiters
}

func deliverTranscriptionUpdate(updateCh chan Transcription, transcription *Transcription) {
	select {
	case updateCh <- *transcription:
		return
	default:
	}

	if transcription.Pending {
		return
	}

	select {
	case <-updateCh:
	default:
	}

	select {
	case updateCh <- *transcription:
	default:
	}
}

func makeTranscriptionKey(peer InputPeer, msgID int, transcriptionID int64) transcriptionKey {
	return transcriptionKey{
		peerType:        peer.Type,
		peerID:          peer.ID,
		msgID:           msgID,
		transcriptionID: transcriptionID,
	}
}

func transcriptionFromUpdate(update *tg.UpdateTranscribedAudio) Transcription {
	status := TranscriptionStatusCompleted
	if update.GetPending() {
		status = TranscriptionStatusPending
	}

	return Transcription{
		Status:          status,
		MessageID:       update.MsgID,
		Pending:         update.GetPending(),
		TranscriptionID: update.TranscriptionID,
		Text:            update.Text,
	}
}

func transcriptionFromResponse(
	msgID int, messageType string, response *tg.MessagesTranscribedAudio,
) *Transcription {
	status := TranscriptionStatusCompleted
	if response.GetPending() {
		status = TranscriptionStatusPending
	}

	transcription := &Transcription{
		Status:          status,
		MessageID:       msgID,
		Type:            messageType,
		Pending:         response.GetPending(),
		TranscriptionID: response.TranscriptionID,
		Text:            response.Text,
	}

	if num, ok := response.GetTrialRemainsNum(); ok {
		transcription.TrialRemainsNum = num
	}

	if until, ok := response.GetTrialRemainsUntilDate(); ok {
		transcription.TrialRemainsUntilDate = until
	}

	return transcription
}

func isTranscribableAudioType(messageType string) bool {
	return messageType == MessageTypeVoice || messageType == MessageTypeVideoNote
}

// TranscribeAudio requests Telegram-side transcription for a voice message
// or video note and, when Telegram returns pending=true, waits for a matching
// updateTranscribedAudio until wait expires.
func (w *Wrapper) TranscribeAudio(
	ctx context.Context, peer InputPeer, msgID int, wait time.Duration,
) (*Transcription, error) {
	raw, err := w.getRawMessage(ctx, peer, msgID)
	if err != nil {
		return nil, errors.Wrap(err, "getting message before transcription")
	}

	messageType := MessageType(raw)
	if !isTranscribableAudioType(messageType) {
		return &Transcription{
			Status:    TranscriptionStatusNotTranscribable,
			MessageID: msgID,
			Type:      messageType,
		}, nil
	}

	subscription := w.subscribeTranscriptionUpdates(peer, msgID, wait)
	if subscription.Cancel != nil {
		defer subscription.Cancel()
	}

	response, err := w.api.MessagesTranscribeAudio(ctx, &tg.MessagesTranscribeAudioRequest{
		Peer:  InputPeerToTG(peer),
		MsgID: msgID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "transcribing audio")
	}

	if response == nil {
		return nil, errors.New("empty transcription response")
	}

	transcription := transcriptionFromResponse(msgID, messageType, response)
	if !transcription.Pending || subscription.Updates == nil {
		return transcription, nil
	}

	return w.waitForTranscription(ctx, transcription, subscription.Updates, wait)
}

func (w *Wrapper) subscribeTranscriptionUpdates(
	peer InputPeer, msgID int, wait time.Duration,
) TranscriptionSubscription {
	if wait <= 0 || w.transcriptions == nil {
		return TranscriptionSubscription{}
	}

	return w.transcriptions.Subscribe(peer, msgID, 0)
}

func (w *Wrapper) waitForTranscription(
	ctx context.Context,
	initial *Transcription,
	updates <-chan Transcription,
	wait time.Duration,
) (*Transcription, error) {
	timer := time.NewTimer(wait)
	defer timer.Stop()

	current := *initial

	for {
		select {
		case update := <-updates:
			current.Status = update.Status
			current.Pending = update.Pending
			current.Text = update.Text

			if update.TranscriptionID != 0 {
				current.TranscriptionID = update.TranscriptionID
			}

			if !update.Pending {
				return &current, nil
			}
		case <-timer.C:
			return &current, nil
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "waiting for transcription update")
		}
	}
}
