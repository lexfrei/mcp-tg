package tools

import (
	"context"
	"strings"
	"testing"
)

func TestMessagesDeleteTool_Definition(t *testing.T) {
	tool := MessagesDeleteTool()
	if tool.Name == "" {
		t.Error("tool name must not be empty")
	}
}

func TestMessagesDeleteHandler_DeletesFromHistoryByDefault(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesDeleteHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer: "@channel",
		IDs:  []int{22, 23},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := mock.deleteIDs; len(got) != 2 {
		t.Errorf("deleteIDs = %v, want the history RPC to receive both IDs", got)
	}

	if mock.deleteScheduledIDs != nil {
		t.Errorf("deleteScheduledIDs = %v, want the scheduled RPC untouched", mock.deleteScheduledIDs)
	}

	if strings.Contains(structured.Output, "scheduled") {
		t.Errorf("Output = %q, want it not to claim a scheduled deletion", structured.Output)
	}
}

// A scheduled message and a published one can wear the same ID, so the flag
// must pick the RPC rather than merely annotate the output: routing it to
// the history RPC would delete a live message instead.
func TestMessagesDeleteHandler_ScheduledRoutesToTheScheduledQueue(t *testing.T) {
	scheduled := true
	mock := &mockClient{}
	handler := NewMessagesDeleteHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer:      "@channel",
		IDs:       []int{22, 23},
		Scheduled: &scheduled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := mock.deleteScheduledIDs; len(got) != 2 || got[0] != 22 || got[1] != 23 {
		t.Errorf("deleteScheduledIDs = %v, want [22 23]", got)
	}

	if mock.deleteIDs != nil {
		t.Errorf("deleteIDs = %v, want the history RPC untouched", mock.deleteIDs)
	}

	if !strings.Contains(structured.Output, "scheduled") {
		t.Errorf("Output = %q, want it to name the scheduled queue", structured.Output)
	}
}

func TestMessagesDeleteHandler_RequiresPeerAndIDs(t *testing.T) {
	handler := NewMessagesDeleteHandler(&mockClient{})

	cases := map[string]MessagesDeleteParams{
		"no peer": {IDs: []int{1}},
		"no ids":  {Peer: "@channel"},
	}

	for name, params := range cases {
		t.Run(name, func(t *testing.T) {
			result, _, err := handler(context.Background(), nil, params)
			if err == nil {
				t.Fatal("expected a validation error")
			}

			if result == nil || !result.IsError {
				t.Error("expected the result to be flagged as an error")
			}
		})
	}
}
