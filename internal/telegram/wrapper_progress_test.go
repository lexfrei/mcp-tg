package telegram

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// progressCapture records every progress callback invocation thread-safely
// (gotd may call Chunk from multiple uploader workers).
type progressCapture struct {
	mu    sync.Mutex
	calls [][2]int64
}

func (p *progressCapture) cb(_ context.Context, uploaded, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = append(p.calls, [2]int64{uploaded, total})
}

func (p *progressCapture) snapshot() [][2]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([][2]int64, len(p.calls))
	copy(out, p.calls)

	return out
}

// uploadInvoker answers every RPC the upload paths issue: saveFilePart,
// uploadMedia, sendMedia, sendMultiMedia.
type uploadInvoker struct{}

func (uploadInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.UploadSaveFilePartRequest:
		return encodeResp(&tg.BoolTrue{}, output)
	case *tg.MessagesUploadMediaRequest:
		return encodeResp(uploadMediaResponse(req.Media), output)
	case *tg.MessagesSendMediaRequest:
		return encodeResp(&tg.Updates{}, output)
	case *tg.MessagesSendMultiMediaRequest:
		return encodeResp(&tg.Updates{}, output)
	default:
		return errUnexpectedRequest
	}
}

func writeTempFile(t *testing.T, name string, size int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)

	err := os.WriteFile(path, make([]byte, size), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	return path
}

func newUploadWrapper() *Wrapper {
	api := tg.NewClient(uploadInvoker{})

	return &Wrapper{api: api, up: uploader.NewUploader(api), cache: NewPeerCache()}
}

func TestUploadProgressAdapter_ForwardsChunk(t *testing.T) {
	var gotUploaded, gotTotal int64

	adapter := uploadProgress{cb: func(_ context.Context, uploaded, total int64) {
		gotUploaded = uploaded
		gotTotal = total
	}}

	err := adapter.Chunk(t.Context(), uploader.ProgressState{Uploaded: 42, Total: 100})
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}

	if gotUploaded != 42 || gotTotal != 100 {
		t.Errorf("forwarded (%d,%d), want (42,100)", gotUploaded, gotTotal)
	}
}

func TestUploaderFor(t *testing.T) {
	wrap := newUploadWrapper()

	if wrap.uploaderFor(nil) != wrap.up {
		t.Error("uploaderFor(nil) must return the shared uploader unchanged")
	}

	withCB := wrap.uploaderFor(func(context.Context, int64, int64) {})
	if withCB == nil || withCB == wrap.up {
		t.Error("uploaderFor(cb) must return a fresh uploader, not the shared one")
	}
}

func TestSendFile_ReportsByteProgress(t *testing.T) {
	const size = 1000

	path := writeTempFile(t, "a.bin", size)
	pc := &progressCapture{}

	_, err := newUploadWrapper().SendFile(
		t.Context(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		path, "",
		SendOpts{Progress: pc.cb},
	)
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	assertFinalProgress(t, pc.snapshot(), size)
}

func TestUploadFile_ReportsByteProgress(t *testing.T) {
	const size = 1500

	path := writeTempFile(t, "b.bin", size)
	pc := &progressCapture{}

	got, err := newUploadWrapper().UploadFile(t.Context(), path, UploadOpts{Progress: pc.cb})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}

	if got.Size != size {
		t.Errorf("UploadedFile.Size = %d, want %d", got.Size, size)
	}

	assertFinalProgress(t, pc.snapshot(), size)
}

func TestUploadFile_EmptyFile_NoChunk(t *testing.T) {
	path := writeTempFile(t, "empty.bin", 0)
	pc := &progressCapture{}

	got, err := newUploadWrapper().UploadFile(t.Context(), path, UploadOpts{Progress: pc.cb})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}

	if got.Size != 0 {
		t.Errorf("Size = %d, want 0", got.Size)
	}

	// gotd returns on EOF before invoking Chunk for a 0-byte file, so the
	// callback never fires — this is exactly why done() needs its fallback.
	if calls := pc.snapshot(); len(calls) != 0 {
		t.Errorf("empty file produced %d progress callbacks, want 0", len(calls))
	}
}

func TestSendAlbum_AggregatesProgress(t *testing.T) {
	const (
		size1 = 1000
		size2 = 2000
		grand = size1 + size2
	)

	paths := []string{writeTempFile(t, "a.png", size1), writeTempFile(t, "b.png", size2)}
	pc := &progressCapture{}

	_, err := newUploadWrapper().SendAlbum(
		t.Context(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		paths, "",
		SendOpts{Progress: pc.cb},
	)
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}

	calls := pc.snapshot()
	assertFinalProgress(t, calls, grand)

	// Every report uses the grand total and is monotonic in uploaded bytes.
	var prev int64

	for i, c := range calls {
		if c[1] != grand {
			t.Errorf("call %d total = %d, want grand total %d", i, c[1], grand)
		}

		if c[0] < prev {
			t.Errorf("call %d uploaded = %d went backwards from %d", i, c[0], prev)
		}

		prev = c[0]
	}
}

func TestAlbumItemProgress_ClampsToGrandTotal(t *testing.T) {
	var got [2]int64

	wrapped := albumItemProgress(func(_ context.Context, uploaded, total int64) {
		got = [2]int64{uploaded, total}
	}, 0, 1000)

	// An un-stat'd item is recorded as 0 bytes in the grand total yet still
	// uploads real bytes; the reported value must never exceed the total.
	wrapped(t.Context(), 1500, 1500)

	if got[0] > got[1] {
		t.Errorf("reported (%d,%d) exceeds total; want current <= total", got[0], got[1])
	}

	if got[0] != 1000 || got[1] != 1000 {
		t.Errorf("reported (%d,%d), want clamped to (1000,1000)", got[0], got[1])
	}
}

func TestAlbumItemProgress_NilCallback(t *testing.T) {
	if albumItemProgress(nil, 0, 100) != nil {
		t.Error("albumItemProgress(nil, ...) must return nil")
	}
}

func assertFinalProgress(t *testing.T, calls [][2]int64, wantTotal int64) {
	t.Helper()

	if len(calls) == 0 {
		t.Fatal("progress callback was never invoked")
	}

	last := calls[len(calls)-1]
	if last[0] != wantTotal || last[1] != wantTotal {
		t.Errorf("final progress = (%d,%d), want (%d,%d)", last[0], last[1], wantTotal, wantTotal)
	}
}
