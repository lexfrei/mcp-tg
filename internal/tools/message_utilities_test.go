package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func channelPeer() telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerChannel, ID: 10, AccessHash: 1}
}

func TestMessagesGetLink(t *testing.T) {
	mock := &mockClient{peer: channelPeer(), messageLink: "https://t.me/team/42"}

	_, result, err := NewMessagesGetLinkHandler(mock)(context.Background(), nil,
		MessagesGetLinkParams{Peer: "@team", MessageID: 42, Thread: new(true)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Link != "https://t.me/team/42" || !mock.lastThread {
		t.Errorf("result = %+v, thread = %v", result, mock.lastThread)
	}
}

func TestMessagesGetReadBy(t *testing.T) {
	mock := &mockClient{peer: channelPeer(), readReceipts: []telegram.ReadReceipt{{UserID: 5, Date: 1700000000}}}

	_, result, err := NewMessagesGetReadByHandler(mock)(context.Background(), nil,
		MessagesGetReadByParams{Peer: "@team", MessageID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || !strings.Contains(result.Output, "[user:5]") {
		t.Errorf("result = %+v", result)
	}
}

func TestMessagesUnpinAll_PassesTopic(t *testing.T) {
	mock := &mockClient{peer: channelPeer(), group: &telegram.GroupInfo{IsForum: true}}

	_, result, err := NewMessagesUnpinAllHandler(mock)(context.Background(), nil,
		MessagesUnpinAllParams{Peer: "@team", TopicID: new(3)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.unpinAllCalls != 1 || mock.unpinAllTopic != 3 || !strings.Contains(result.Output, "topic") {
		t.Errorf("calls = %d, topic = %d, output = %q", mock.unpinAllCalls, mock.unpinAllTopic, result.Output)
	}
}

func TestDraftsList(t *testing.T) {
	mock := &mockClient{drafts: []telegram.Draft{
		{Peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 5}, Name: "Ann", Username: "ann", Text: "see you", ReplyTo: 7},
	}}

	_, result, err := NewDraftsListHandler(mock)(context.Background(), nil, DraftsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || !strings.Contains(result.Output, "Ann [@ann]") || !strings.Contains(result.Output, "reply to: 7") {
		t.Errorf("result = %+v", result)
	}
}

func TestMessagesSendContact(t *testing.T) {
	mock := &mockClient{peer: channelPeer(), message: &telegram.Message{ID: 8}}

	_, result, err := NewMessagesSendContactHandler(mock)(context.Background(), nil, MessagesSendContactParams{
		Peer: "@team", Phone: "+15550100", FirstName: "Ann", LastName: "Lee", Silent: new(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MessageID != 8 || mock.lastContact.Phone != "+15550100" || mock.lastContact.LastName != "Lee" ||
		!mock.lastSendOpts.Silent {
		t.Errorf("result = %+v, contact = %+v, opts = %+v", result, mock.lastContact, mock.lastSendOpts)
	}
}

func TestMessagesSendContact_Validation(t *testing.T) {
	for want, params := range map[error]MessagesSendContactParams{
		ErrPeerRequired:         {Phone: "1", FirstName: "a"},
		ErrContactPhoneRequired: {Peer: "@x", FirstName: "a"},
		ErrContactNameRequired:  {Peer: "@x", Phone: "1"},
	} {
		_, _, err := NewMessagesSendContactHandler(&mockClient{})(context.Background(), nil, params)
		if !errors.Is(err, want) {
			t.Errorf("params %+v: err = %v, want %v", params, err, want)
		}
	}
}

// A negative topicId must fail, not widen a topic-scoped unpin to the
// whole chat.
func TestMessagesUnpinAll_RejectsANegativeTopic(t *testing.T) {
	mock := &mockClient{peer: channelPeer(), group: &telegram.GroupInfo{IsForum: true}}

	_, _, err := NewMessagesUnpinAllHandler(mock)(context.Background(), nil,
		MessagesUnpinAllParams{Peer: "@team", TopicID: new(-3)})
	if !errors.Is(err, ErrNegativeTopicID) || !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrNegativeTopicID as a validation error", err)
	}

	if mock.unpinAllCalls != 0 {
		t.Error("a rejected topic reached UnpinAllMessages")
	}
}

func TestMessageUtilities_RejectBadTargets(t *testing.T) {
	user := telegram.InputPeer{Type: telegram.PeerUser, ID: 5, AccessHash: 1}

	for name, call := range map[string]func() error{
		"get_link empty peer": func() error {
			_, _, err := NewMessagesGetLinkHandler(&mockClient{})(context.Background(), nil, MessagesGetLinkParams{MessageID: 1})

			return err
		},
		"get_link no message": func() error {
			_, _, err := NewMessagesGetLinkHandler(&mockClient{peer: channelPeer()})(context.Background(), nil,
				MessagesGetLinkParams{Peer: "@team"})

			return err
		},
		"read_by empty peer": func() error {
			_, _, err := NewMessagesGetReadByHandler(&mockClient{})(context.Background(), nil, MessagesGetReadByParams{MessageID: 1})

			return err
		},
		"read_by no message": func() error {
			_, _, err := NewMessagesGetReadByHandler(&mockClient{peer: channelPeer()})(context.Background(), nil,
				MessagesGetReadByParams{Peer: "@team"})

			return err
		},
		"unpin_all empty peer": func() error {
			_, _, err := NewMessagesUnpinAllHandler(&mockClient{})(context.Background(), nil, MessagesUnpinAllParams{})

			return err
		},
		"unpin_all topic on a non-forum": func() error {
			_, _, err := NewMessagesUnpinAllHandler(&mockClient{peer: user})(context.Background(), nil,
				MessagesUnpinAllParams{Peer: "@ann", TopicID: new(3)})

			return err
		},
	} {
		err := call()
		if err == nil {
			t.Errorf("%s: no error", name)

			continue
		}

		if !errors.Is(err, ErrPeerRequired) && !errors.Is(err, ErrMessageIDRequired) && !errors.Is(err, ErrTopicIDOnNonForum) {
			t.Errorf("%s: err = %v, want a rejection before any request", name, err)
		}
	}
}

// Every new tool reports a client failure as a tool error.
func TestMessageUtilities_PropagateClientErrors(t *testing.T) {
	errBoom := errors.New("boom")

	failing := func() *mockClient {
		return &mockClient{
			err: errBoom,
			resolvePeerFn: func(string) (telegram.InputPeer, error) {
				return channelPeer(), nil
			},
		}
	}

	for name, call := range map[string]func() error{
		"get_link": func() error {
			_, _, err := NewMessagesGetLinkHandler(failing())(context.Background(), nil,
				MessagesGetLinkParams{Peer: "@team", MessageID: 1})

			return err
		},
		"read_by": func() error {
			_, _, err := NewMessagesGetReadByHandler(failing())(context.Background(), nil,
				MessagesGetReadByParams{Peer: "@team", MessageID: 1})

			return err
		},
		"unpin_all": func() error {
			_, _, err := NewMessagesUnpinAllHandler(failing())(context.Background(), nil, MessagesUnpinAllParams{Peer: "@team"})

			return err
		},
		"drafts_list": func() error {
			_, _, err := NewDraftsListHandler(failing())(context.Background(), nil, DraftsListParams{})

			return err
		},
		"send_contact": func() error {
			_, _, err := NewMessagesSendContactHandler(failing())(context.Background(), nil,
				MessagesSendContactParams{Peer: "@team", Phone: "+1", FirstName: "a"})

			return err
		},
	} {
		err := call()
		if !errors.Is(err, errBoom) {
			t.Errorf("%s: err = %v, want the client error", name, err)
		}
	}
}
