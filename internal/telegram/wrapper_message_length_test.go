package telegram

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// failingResolver returns a resolver that fails the test if invoked.
// Used to assert the fast-path skips the server lookup.
func failingResolver(t *testing.T) func(context.Context) (int, error) {
	t.Helper()

	return func(_ context.Context) (int, error) {
		t.Fatalf("server-side resolver must not be called on fast-path")

		return 0, nil
	}
}

func TestValidateMessageLength_Empty(t *testing.T) {
	err := validateMessageLength(t.Context(), "", failingResolver(t))
	if err == nil {
		t.Fatal("validateMessageLength(\"\") = nil, want error")
	}
}

func TestValidateMessageLength_FastPathExactBoundary(t *testing.T) {
	text := strings.Repeat("a", messageLengthFastPath)

	err := validateMessageLength(t.Context(), text, failingResolver(t))
	if err != nil {
		t.Fatalf("validateMessageLength(4096 chars) = %v, want nil", err)
	}
}

func TestValidateMessageLength_FastPathBelowBoundary(t *testing.T) {
	text := strings.Repeat("a", messageLengthFastPath-1)

	err := validateMessageLength(t.Context(), text, failingResolver(t))
	if err != nil {
		t.Fatalf("validateMessageLength(4095 chars) = %v, want nil", err)
	}
}

func TestValidateMessageLength_OverFastPathWithinServerLimit(t *testing.T) {
	text := strings.Repeat("a", messageLengthFastPath+500)

	var calls int32

	resolver := func(_ context.Context) (int, error) {
		atomic.AddInt32(&calls, 1)

		return messageLengthFastPath + 1000, nil
	}

	err := validateMessageLength(t.Context(), text, resolver)
	if err != nil {
		t.Fatalf("validateMessageLength(within server limit) = %v, want nil", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("resolver calls = %d, want 1", got)
	}
}

func TestValidateMessageLength_OverServerLimit(t *testing.T) {
	const serverMax = 5000

	textLen := serverMax + 500
	text := strings.Repeat("a", textLen)

	resolver := func(_ context.Context) (int, error) {
		return serverMax, nil
	}

	err := validateMessageLength(t.Context(), text, resolver)
	if err == nil {
		t.Fatalf("validateMessageLength(over server limit) = nil, want error")
	}

	if !strings.Contains(err.Error(), "5500") {
		t.Errorf("error %q must mention text length 5500", err)
	}

	if !strings.Contains(err.Error(), "5000") {
		t.Errorf("error %q must mention server limit 5000", err)
	}
}

func TestValidateMessageLength_ResolverErrorAllowsSend(t *testing.T) {
	text := strings.Repeat("a", messageLengthFastPath+500)
	resolverErr := errors.New("network down")

	resolver := func(_ context.Context) (int, error) {
		return 0, resolverErr
	}

	err := validateMessageLength(t.Context(), text, resolver)
	if err != nil {
		t.Fatalf("validateMessageLength with resolver error = %v, want nil (let server decide)", err)
	}
}

// Surrogate-pair emoji: 1 codepoint, 2 UTF-16 code units, 4 UTF-8 bytes.
// 4500 such emoji = 4500 codepoints (over fast-path) but fits within 5000-codepoint server limit.
// Counting UTF-16 would yield 9000 and incorrectly fail.
func TestValidateMessageLength_CountsCodepointsNotUTF16(t *testing.T) {
	text := strings.Repeat("🎉", messageLengthFastPath+500)

	resolver := func(_ context.Context) (int, error) {
		return messageLengthFastPath + 1000, nil
	}

	err := validateMessageLength(t.Context(), text, resolver)
	if err != nil {
		t.Fatalf("emoji-only text within server limit = %v, want nil (codepoint counting)", err)
	}
}

// configInvoker fakes help.getConfig responses to test caching of length limits.
type configInvoker struct {
	calls      atomic.Int32
	maxLen     int
	captionMax int
	err        error
}

func (c *configInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.HelpGetConfigRequest); !ok {
		return errUnexpectedRequest
	}

	c.calls.Add(1)

	if c.err != nil {
		return c.err
	}

	cfg := &tg.Config{
		MessageLengthMax: c.maxLen,
		CaptionLengthMax: c.captionMax,
	}

	return encodeAndDecodeConfig(cfg, output)
}

func encodeAndDecodeConfig(cfg *tg.Config, output bin.Decoder) error {
	var buf bin.Buffer

	err := cfg.Encode(&buf)
	if err != nil {
		return errors.Wrap(err, "encode config")
	}

	err = output.Decode(&buf)
	if err != nil {
		return errors.Wrap(err, "decode config")
	}

	return nil
}

func TestMessageLengthMax_FetchesAndCaches(t *testing.T) {
	const serverMax = 8192

	invoker := &configInvoker{maxLen: serverMax}
	wrap := newWrapperWithInvoker(invoker)

	got, err := wrap.MessageLengthMax(t.Context())
	if err != nil {
		t.Fatalf("first MessageLengthMax: %v", err)
	}

	if got != serverMax {
		t.Errorf("first call = %d, want %d", got, serverMax)
	}

	got2, err := wrap.MessageLengthMax(t.Context())
	if err != nil {
		t.Fatalf("second MessageLengthMax: %v", err)
	}

	if got2 != serverMax {
		t.Errorf("second call = %d, want %d", got2, serverMax)
	}

	if calls := invoker.calls.Load(); calls != 1 {
		t.Errorf("HelpGetConfig calls = %d, want 1 (cached)", calls)
	}
}

func TestMessageLengthMax_ConcurrentCallsSingleFetch(t *testing.T) {
	const (
		serverMax  = 8192
		goroutines = 16
	)

	invoker := &configInvoker{maxLen: serverMax}
	wrap := newWrapperWithInvoker(invoker)

	var wg sync.WaitGroup

	wg.Add(goroutines)

	for range goroutines {
		go func() {
			defer wg.Done()

			_, _ = wrap.MessageLengthMax(t.Context())
		}()
	}

	wg.Wait()

	if calls := invoker.calls.Load(); calls != 1 {
		t.Errorf("HelpGetConfig calls = %d, want 1 (mutex-guarded)", calls)
	}
}

func TestMessageLengthMax_ErrorNotCached(t *testing.T) {
	invoker := &configInvoker{err: errTestBoom}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.MessageLengthMax(t.Context())
	if err == nil {
		t.Fatal("first MessageLengthMax: expected error")
	}

	_, err = wrap.MessageLengthMax(t.Context())
	if err == nil {
		t.Fatal("second MessageLengthMax: expected error")
	}

	if calls := invoker.calls.Load(); calls != 2 {
		t.Errorf("HelpGetConfig calls = %d, want 2 (errors must not be cached)", calls)
	}
}

// Defensive guard: if the server returns 0 for the limit, the result must
// still be cached so we don't keep hammering help.getConfig forever.
func TestMessageLengthMax_ZeroValueCached(t *testing.T) {
	invoker := &configInvoker{maxLen: 0}
	wrap := newWrapperWithInvoker(invoker)

	for range 3 {
		_, err := wrap.MessageLengthMax(t.Context())
		if err != nil {
			t.Fatalf("MessageLengthMax: %v", err)
		}
	}

	if calls := invoker.calls.Load(); calls != 1 {
		t.Errorf("HelpGetConfig calls = %d, want 1 (zero value still cached)", calls)
	}
}

// Caption and message limits share a single help.getConfig fetch.
func TestServerLimits_SharedAcrossMethods(t *testing.T) {
	invoker := &configInvoker{maxLen: 8192, captionMax: 2048}
	wrap := newWrapperWithInvoker(invoker)

	got, err := wrap.MessageLengthMax(t.Context())
	if err != nil {
		t.Fatalf("MessageLengthMax: %v", err)
	}

	if got != 8192 {
		t.Errorf("MessageLengthMax = %d, want 8192", got)
	}

	gotCaption, err := wrap.CaptionLengthMax(t.Context())
	if err != nil {
		t.Fatalf("CaptionLengthMax: %v", err)
	}

	if gotCaption != 2048 {
		t.Errorf("CaptionLengthMax = %d, want 2048", gotCaption)
	}

	if calls := invoker.calls.Load(); calls != 1 {
		t.Errorf("HelpGetConfig calls = %d, want 1 (config shared between MessageLengthMax and CaptionLengthMax)", calls)
	}
}

func TestCaptionLengthMax_FetchesAndCaches(t *testing.T) {
	const captionMax = 4096

	invoker := &configInvoker{captionMax: captionMax}
	wrap := newWrapperWithInvoker(invoker)

	got, err := wrap.CaptionLengthMax(t.Context())
	if err != nil {
		t.Fatalf("first CaptionLengthMax: %v", err)
	}

	if got != captionMax {
		t.Errorf("first call = %d, want %d", got, captionMax)
	}

	got2, err := wrap.CaptionLengthMax(t.Context())
	if err != nil {
		t.Fatalf("second CaptionLengthMax: %v", err)
	}

	if got2 != captionMax {
		t.Errorf("second call = %d, want %d", got2, captionMax)
	}

	if calls := invoker.calls.Load(); calls != 1 {
		t.Errorf("HelpGetConfig calls = %d, want 1 (cached)", calls)
	}
}

func TestValidateCaptionLength_EmptyAllowed(t *testing.T) {
	err := validateCaptionLength(t.Context(), "", failingResolver(t))
	if err != nil {
		t.Fatalf("validateCaptionLength(\"\") = %v, want nil (caption is optional)", err)
	}
}

func TestValidateCaptionLength_FastPathBoundary(t *testing.T) {
	text := strings.Repeat("a", captionLengthFastPath)

	err := validateCaptionLength(t.Context(), text, failingResolver(t))
	if err != nil {
		t.Fatalf("validateCaptionLength(1024 chars) = %v, want nil", err)
	}
}

func TestValidateCaptionLength_OverFastPathWithinServerLimit(t *testing.T) {
	text := strings.Repeat("a", captionLengthFastPath+500)

	var calls int32

	resolver := func(_ context.Context) (int, error) {
		atomic.AddInt32(&calls, 1)

		return captionLengthFastPath + 1000, nil
	}

	err := validateCaptionLength(t.Context(), text, resolver)
	if err != nil {
		t.Fatalf("validateCaptionLength(within server limit) = %v, want nil", err)
	}

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("resolver calls = %d, want 1", got)
	}
}

func TestValidateCaptionLength_OverServerLimit(t *testing.T) {
	const serverMax = 1024

	textLen := serverMax + 500
	text := strings.Repeat("a", textLen)

	resolver := func(_ context.Context) (int, error) {
		return serverMax, nil
	}

	err := validateCaptionLength(t.Context(), text, resolver)
	if err == nil {
		t.Fatalf("validateCaptionLength(over server limit) = nil, want error")
	}

	if !strings.Contains(err.Error(), "caption") {
		t.Errorf("error %q must mention caption", err)
	}

	if !strings.Contains(err.Error(), "1524") {
		t.Errorf("error %q must mention text length 1524", err)
	}

	if !strings.Contains(err.Error(), "1024") {
		t.Errorf("error %q must mention server limit 1024", err)
	}
}

func TestValidateCaptionLength_ResolverErrorAllowsSend(t *testing.T) {
	text := strings.Repeat("a", captionLengthFastPath+500)

	resolver := func(_ context.Context) (int, error) {
		return 0, errors.New("network down")
	}

	err := validateCaptionLength(t.Context(), text, resolver)
	if err != nil {
		t.Fatalf("validateCaptionLength with resolver error = %v, want nil", err)
	}
}

// renderCaption must use the post-CommonMark plaintext for length checks,
// because that is what the server measures.
func TestRenderCaption_StripsCommonMarkMarkers(t *testing.T) {
	source := "**bold** _italic_"
	rendered := renderCaption(source, "commonmark")

	if rendered == source {
		t.Errorf("rendered = %q, expected markdown markers stripped", rendered)
	}

	if !strings.Contains(rendered, "bold") || !strings.Contains(rendered, "italic") {
		t.Errorf("rendered = %q lost text content", rendered)
	}
}

func TestRenderCaption_PlainPassthrough(t *testing.T) {
	source := "**not parsed when parseMode is empty**"
	rendered := renderCaption(source, "")

	if rendered != source {
		t.Errorf("rendered = %q, want passthrough %q", rendered, source)
	}
}

// SendFile must reject over-limit captions before doing anything else.
// The wrapper used here has no uploader, so anything past the validation
// gate would panic; reaching the expected validation error proves order.
func TestSendFile_RejectsOverLimitCaptionBeforeUpload(t *testing.T) {
	invoker := &configInvoker{maxLen: 4096, captionMax: 1024}
	wrap := newWrapperWithInvoker(invoker)

	caption := strings.Repeat("a", 2000)

	_, err := wrap.SendFile(
		t.Context(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 1},
		"/nonexistent/path.bin",
		caption,
		SendOpts{},
	)
	if err == nil {
		t.Fatal("SendFile with over-limit caption: expected error")
	}

	if !strings.Contains(err.Error(), "caption length") {
		t.Errorf("error %q must mention caption length", err)
	}
}

// SendAlbum has the same fail-fast contract.
func TestSendAlbum_RejectsOverLimitCaptionBeforeUpload(t *testing.T) {
	invoker := &configInvoker{maxLen: 4096, captionMax: 1024}
	wrap := newWrapperWithInvoker(invoker)

	caption := strings.Repeat("a", 2000)

	_, err := wrap.SendAlbum(
		t.Context(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 1},
		[]string{"/nonexistent/a.bin", "/nonexistent/b.bin"},
		caption,
		SendOpts{},
	)
	if err == nil {
		t.Fatal("SendAlbum with over-limit caption: expected error")
	}

	if !strings.Contains(err.Error(), "caption length") {
		t.Errorf("error %q must mention caption length", err)
	}
}
