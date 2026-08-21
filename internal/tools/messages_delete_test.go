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

// Deleted must report the server's affected count, not the request size:
// an id already gone contributes nothing to Telegram's own count, and a
// caller relying on Deleted to confirm a real deletion needs that gap
// visible instead of papered over with len(ids).
func TestMessagesDeleteHandler_ScheduledReportsTheServersAffectedCount(t *testing.T) {
	scheduled := true
	mock := &mockClient{deleteAffected: 1}
	handler := NewMessagesDeleteHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer:      "@channel",
		IDs:       []int{22, 23, 24},
		Scheduled: &scheduled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Deleted == nil || *structured.Deleted != 1 {
		t.Errorf("Deleted = %v, want a pointer to the server's count (1), not len(ids) (3)", structured.Deleted)
	}

	if !strings.Contains(structured.Output, "Deleted 1 ") {
		t.Errorf("Output = %q, want it to name the server's count", structured.Output)
	}
}

func TestMessagesDeleteHandler_ScheduledDeletedZeroWhenNothingMatched(t *testing.T) {
	scheduled := true
	mock := &mockClient{deleteAffected: 0}
	handler := NewMessagesDeleteHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer:      "@channel",
		IDs:       []int{22, 23},
		Scheduled: &scheduled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Deleted == nil || *structured.Deleted != 0 {
		t.Errorf("Deleted = %v, want a pointer to 0 when nothing in the queue matched the ids", structured.Deleted)
	}
}

// The ordinary history path has no server-verified affected-count to
// report (see Wrapper.DeleteMessages), so Deleted must be nil rather than
// a number dressed up as server truth — the exact defect this tool used
// to have before this fix.
func TestMessagesDeleteHandler_OrdinaryDeleteOmitsTheUnverifiableCount(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesDeleteHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer: "@channel",
		IDs:  []int{22, 23, 24},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Deleted != nil {
		t.Errorf("Deleted = %v, want nil for an ordinary delete", structured.Deleted)
	}

	if !strings.Contains(structured.Output, "Requested deletion of 3 ") {
		t.Errorf("Output = %q, want it to name the request size without claiming server verification", structured.Output)
	}

	if strings.Contains(structured.Output, "Deleted") {
		t.Errorf("Output = %q, want it not to say \"Deleted\" without a verified count", structured.Output)
	}
}

func TestMessagesDeleteHandler_RequiresPeerAndIDs(t *testing.T) {
	handler := NewMessagesDeleteHandler(&mockClient{})

	tooMany := make([]int, maxIDsPerRequest+1)
	for i := range tooMany {
		tooMany[i] = i + 1
	}

	cases := map[string]MessagesDeleteParams{
		"no peer":      {IDs: []int{1}},
		"no ids":       {Peer: "@channel"},
		"too many ids": {Peer: "@channel", IDs: tooMany},
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

// The mock must record the revoke argument: the schema documents default
// true, the sibling delete-history tool defaults the same-named parameter to
// false via plain deref, and nothing but these assertions keeps a cleanup
// from crossing the two.
func TestMessagesDeleteHandler_RevokeDefaultsToTrue(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesDeleteHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer: "@channel",
		IDs:  []int{1},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.deleteRevoke {
		t.Error("deleteRevoke = false, want the omitted revoke to default to true")
	}
}

func TestMessagesDeleteHandler_ExplicitRevokeFalseIsPassedThrough(t *testing.T) {
	revoke := false
	mock := &mockClient{}
	handler := NewMessagesDeleteHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesDeleteParams{
		Peer:   "@channel",
		IDs:    []int{1},
		Revoke: &revoke,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.deleteRevoke {
		t.Error("deleteRevoke = true, want an explicit false to reach the client")
	}
}
