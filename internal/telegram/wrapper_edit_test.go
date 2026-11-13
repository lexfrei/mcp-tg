package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// refusingInvoker answers every query with one RPC error.
type refusingInvoker struct {
	err error
}

func (r refusingInvoker) Invoke(context.Context, bin.Encoder, bin.Decoder) error {
	return r.err
}

// MESSAGE_NOT_MODIFIED says the stored message already equals what was
// submitted, entities included, so the submitted set is what it carries.
func TestEditMessage_NotModifiedReportsTheSubmittedEntities(t *testing.T) {
	t.Parallel()

	wrap := NewWrapper(tg.NewClient(refusingInvoker{err: tgerr.New(400, "MESSAGE_NOT_MODIFIED")}))
	peer := InputPeer{Type: PeerUser, ID: 1, AccessHash: 2}

	msg, err := wrap.EditMessage(context.Background(), peer, 42, "**bold** text", ParseModeCommonMark)
	if err != nil {
		t.Fatalf("EditMessage: %v, want success", err)
	}

	if msg == nil || msg.ID != 42 {
		t.Fatalf("msg = %+v, want ID 42", msg)
	}

	if len(msg.Entities) != 1 || msg.Entities[0].Type != "bold" {
		t.Errorf("entities = %+v, want the one submitted bold span", msg.Entities)
	}
}

func TestEditMessage_OtherRefusalStillFails(t *testing.T) {
	t.Parallel()

	wrap := NewWrapper(tg.NewClient(refusingInvoker{err: tgerr.New(400, "MESSAGE_ID_INVALID")}))
	peer := InputPeer{Type: PeerUser, ID: 1, AccessHash: 2}

	_, err := wrap.EditMessage(context.Background(), peer, 42, "text", ParseModePlain)
	if !tgerr.Is(err, "MESSAGE_ID_INVALID") {
		t.Fatalf("err = %v, want MESSAGE_ID_INVALID", err)
	}
}
