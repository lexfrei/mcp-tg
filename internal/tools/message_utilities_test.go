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
