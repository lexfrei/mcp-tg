package main

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

const testAppID = 17349

type recordingInvoker struct {
	inputs []bin.Encoder
	errs   []error
}

func (r *recordingInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	r.inputs = append(r.inputs, input)
	if len(r.errs) == 0 {
		return nil
	}

	err := r.errs[0]
	r.errs = r.errs[1:]

	return err
}

func invokeWithReinit(t *testing.T, next *recordingInvoker) error {
	t.Helper()

	mw := newConnReinitMiddleware(testAppID)
	handler := mw(next)

	return handler(context.Background(), &tg.ContactsResolveUsernameRequest{Username: "example"}, nil)
}

func TestConnReinit_PassthroughSuccess(t *testing.T) {
	next := &recordingInvoker{}

	err := invokeWithReinit(t, next)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Fatalf("expected 1 invoke, got %d", len(next.inputs))
	}

	if _, ok := next.inputs[0].(*tg.ContactsResolveUsernameRequest); !ok {
		t.Errorf("expected original request to pass through unwrapped, got %T", next.inputs[0])
	}
}

func TestConnReinit_PassthroughOtherError(t *testing.T) {
	testErr := errors.New("some other failure")
	next := &recordingInvoker{errs: []error{testErr}}

	err := invokeWithReinit(t, next)
	if !errors.Is(err, testErr) {
		t.Fatalf("expected original error, got: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Fatalf("expected no retry for non-layer errors, got %d invokes", len(next.inputs))
	}
}

func TestConnReinit_RetriesWrappedInInitConnection(t *testing.T) {
	next := &recordingInvoker{errs: []error{tgerr.New(400, "CONNECTION_LAYER_INVALID")}}

	err := invokeWithReinit(t, next)
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}

	if len(next.inputs) != 2 {
		t.Fatalf("expected exactly 2 invokes (original + retry), got %d", len(next.inputs))
	}

	initReq, ok := next.inputs[1].(*tg.InitConnectionRequest)
	if !ok {
		t.Fatalf("expected retry wrapped in InitConnectionRequest, got %T", next.inputs[1])
	}

	if initReq.APIID != testAppID {
		t.Errorf("expected APIID %d, got %d", testAppID, initReq.APIID)
	}

	if initReq.DeviceModel == "" || initReq.SystemVersion == "" || initReq.AppVersion == "" {
		t.Errorf("expected device defaults to be populated, got %+v", initReq)
	}

	if _, ok := initReq.Query.(*tg.ContactsResolveUsernameRequest); !ok {
		t.Errorf("expected original query inside initConnection, got %T", initReq.Query)
	}
}

func TestConnReinit_HandlesConnectionNotInited(t *testing.T) {
	next := &recordingInvoker{errs: []error{tgerr.New(400, "CONNECTION_NOT_INITED")}}

	err := invokeWithReinit(t, next)
	if err != nil {
		t.Fatalf("expected retry to succeed, got: %v", err)
	}

	if len(next.inputs) != 2 {
		t.Fatalf("expected exactly 2 invokes (original + retry), got %d", len(next.inputs))
	}
}

// encoderOnly implements bin.Encoder but not bin.Object (no Decode), so it
// cannot be wrapped into initConnection.
type encoderOnly struct{}

func (encoderOnly) Encode(_ *bin.Buffer) error { return nil }

func TestConnReinit_NonObjectInputReturnsOriginalError(t *testing.T) {
	next := &recordingInvoker{errs: []error{tgerr.New(400, "CONNECTION_LAYER_INVALID")}}

	mw := newConnReinitMiddleware(testAppID)
	handler := mw(next)

	err := handler(context.Background(), encoderOnly{}, nil)
	if !tgerr.Is(err, "CONNECTION_LAYER_INVALID") {
		t.Fatalf("expected original error passthrough, got: %v", err)
	}

	if len(next.inputs) != 1 {
		t.Fatalf("expected no retry for non-Object input, got %d invokes", len(next.inputs))
	}
}

func TestConnReinit_RetryFailureReturnsError(t *testing.T) {
	next := &recordingInvoker{errs: []error{
		tgerr.New(400, "CONNECTION_LAYER_INVALID"),
		tgerr.New(400, "CONNECTION_LAYER_INVALID"),
	}}

	err := invokeWithReinit(t, next)
	if !tgerr.Is(err, "CONNECTION_LAYER_INVALID") {
		t.Fatalf("expected CONNECTION_LAYER_INVALID from failed retry, got: %v", err)
	}

	if len(next.inputs) != 2 {
		t.Fatalf("expected exactly 2 invokes (no second retry), got %d", len(next.inputs))
	}
}
