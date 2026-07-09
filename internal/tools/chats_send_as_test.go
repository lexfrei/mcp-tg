package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func sendAsOptions() []telegram.SendAsOption {
	return []telegram.SendAsOption{
		{
			Peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 10, AccessHash: 11},
			Name:     "Alice",
			Username: "alice",
		},
		{
			Peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 20, AccessHash: 21},
			Name:     "My Channel",
			Username: "mychan",
		},
		{
			Peer:            telegram.InputPeer{Type: telegram.PeerChannel, ID: 30, AccessHash: 31},
			Name:            "Paid Channel",
			PremiumRequired: true,
		},
	}
}

func TestChatsGetSendAsTool_Definition(t *testing.T) {
	tool := ChatsGetSendAsTool()
	if tool.Name != "tg_chats_get_send_as" {
		t.Errorf("name = %q, want tg_chats_get_send_as", tool.Name)
	}

	if !tool.Annotations.ReadOnlyHint {
		t.Error("listing identities must be annotated read-only")
	}
}

func TestChatsGetSendAsHandler_Success(t *testing.T) {
	mock := &mockClient{peer: destPeer(), sendAsOptions: sendAsOptions()}

	_, structured, err := NewChatsGetSendAsHandler(mock)(
		context.Background(), nil, ChatsGetSendAsParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 3 {
		t.Fatalf("Count = %d, want 3", structured.Count)
	}

	channel := structured.Identities[1]
	if channel.Type != peerChannel || channel.Username != "mychan" || channel.Name != "My Channel" {
		t.Errorf("channel identity = %+v, want a channel named My Channel @mychan", channel)
	}

	// The peer string must round-trip straight back into a sendAs argument.
	if channel.Peer != "-1000000000020" {
		t.Errorf("peer = %q, want the bot-API form -1000000000020", channel.Peer)
	}

	if !structured.Identities[2].PremiumRequired {
		t.Error("the premium-gated identity lost its premiumRequired flag")
	}

	if !strings.Contains(structured.Output, "My Channel [@mychan]") {
		t.Errorf("output %q must render identities through formatPeerRef", structured.Output)
	}

	if !strings.Contains(structured.Output, "Premium") {
		t.Errorf("output %q must flag the premium-gated identity", structured.Output)
	}
}

// channels.getSendAs is meaningless for direct messages and legacy basic
// groups; failing before the round trip keeps the reason legible.
func TestChatsGetSendAsHandler_RejectsNonChannelPeer(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 5}}

	result, _, err := NewChatsGetSendAsHandler(mock)(
		context.Background(), nil, ChatsGetSendAsParams{Peer: "@user"},
	)
	if !errors.Is(err, telegram.ErrSendAsUnsupportedPeer) {
		t.Fatalf("error = %v, want ErrSendAsUnsupportedPeer", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}

	if mock.getSendAsCalls != 0 {
		t.Error("GetSendAs was called for a user peer")
	}
}

func TestChatsGetSendAsHandler_Error(t *testing.T) {
	mock := &mockClient{peer: destPeer(), err: errors.New("fail")}

	result, _, err := NewChatsGetSendAsHandler(mock)(
		context.Background(), nil, ChatsGetSendAsParams{Peer: "@group"},
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsInfoHandler_ExposesDefaultSendAs(t *testing.T) {
	identity := sendAsOptions()[1]
	mock := &mockClient{
		peer:  destPeer(),
		group: &telegram.GroupInfo{Title: "Group", DefaultSendAs: &identity},
	}

	_, structured, err := NewGroupsInfoHandler(mock)(
		context.Background(), nil, GroupsInfoParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.DefaultSendAs == nil {
		t.Fatal("defaultSendAs is nil even though the group has one")
	}

	if structured.DefaultSendAs.Username != "mychan" || structured.DefaultSendAs.Peer != "-1000000000020" {
		t.Errorf("defaultSendAs = %+v, want @mychan as -1000000000020", *structured.DefaultSendAs)
	}

	if !strings.Contains(structured.Output, "posts as: My Channel [@mychan]") {
		t.Errorf("output %q must name the identity the group posts as", structured.Output)
	}
}

func TestGroupsInfoHandler_OmitsAbsentDefaultSendAs(t *testing.T) {
	mock := &mockClient{peer: destPeer(), group: &telegram.GroupInfo{Title: "Group"}}

	_, structured, err := NewGroupsInfoHandler(mock)(
		context.Background(), nil, GroupsInfoParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.DefaultSendAs != nil {
		t.Errorf("defaultSendAs = %+v, want nil", *structured.DefaultSendAs)
	}

	if strings.Contains(structured.Output, "posts as") {
		t.Errorf("output %q must not mention an identity that is not set", structured.Output)
	}
}

func TestChatsSetSendAsTool_Definition(t *testing.T) {
	tool := ChatsSetSendAsTool()
	if tool.Name != "tg_chats_set_send_as" {
		t.Errorf("name = %q, want tg_chats_set_send_as", tool.Name)
	}

	if !tool.Annotations.IdempotentHint {
		t.Error("setting the same default twice is safe; annotate it idempotent")
	}

	if !strings.Contains(tool.Description, "reaction") {
		t.Error("the description must say the default also governs reactions")
	}
}

func TestChatsSetSendAsHandler_SetsIdentity(t *testing.T) {
	mock := sendAsMock()

	_, structured, err := NewChatsSetSendAsHandler(mock)(
		context.Background(), nil, ChatsSetSendAsParams{Peer: "@group", SendAs: new(sendAsRef)},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSetSendAs)

	if !strings.Contains(structured.Output, "@mychan") {
		t.Errorf("output %q must name the identity that was set", structured.Output)
	}
}

// An omitted sendAs is not a validation error here — it is the documented
// way to hand the chat back to your own account.
func TestChatsSetSendAsHandler_OmittedIdentityResetsToSelf(t *testing.T) {
	mock := sendAsMock()

	_, structured, err := NewChatsSetSendAsHandler(mock)(
		context.Background(), nil, ChatsSetSendAsParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.setSendAsCalls != 1 {
		t.Fatalf("SetDefaultSendAs calls = %d, want 1", mock.setSendAsCalls)
	}

	if mock.lastSetSendAs != nil {
		t.Errorf("identity = %+v, want nil to reset to the account", *mock.lastSetSendAs)
	}

	if !strings.Contains(strings.ToLower(structured.Output), "yourself") {
		t.Errorf("output %q must say the chat now posts as yourself", structured.Output)
	}
}

func TestChatsSetSendAsHandler_RejectsNonChannelPeer(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChat, ID: 5}}

	_, _, err := NewChatsSetSendAsHandler(mock)(
		context.Background(), nil, ChatsSetSendAsParams{Peer: "-5"},
	)
	if !errors.Is(err, telegram.ErrSendAsUnsupportedPeer) {
		t.Fatalf("error = %v, want ErrSendAsUnsupportedPeer", err)
	}

	if mock.setSendAsCalls != 0 {
		t.Error("SetDefaultSendAs was called for a basic group")
	}
}

// saveDefaultSendAs rejects a disallowed identity with the same codes the
// send paths see, so the setter must point at the parameter too rather
// than leaking a raw rpc error.
func TestChatsSetSendAsHandler_BlamesTheIdentityOnRejection(t *testing.T) {
	mock := sendAsMock()
	mock.err = errors.New("rpc error code 400: SEND_AS_PEER_INVALID")
	mock.resolvePeerFn = func(identifier string) (telegram.InputPeer, error) {
		if identifier == sendAsRef {
			return sendAsChannel(), nil
		}

		return destPeer(), nil
	}

	_, _, err := NewChatsSetSendAsHandler(mock)(
		context.Background(), nil, ChatsSetSendAsParams{Peer: "@group", SendAs: new(sendAsRef)},
	)
	if err == nil {
		t.Fatal("expected the call to fail")
	}

	if !strings.Contains(err.Error(), "sendAs") {
		t.Errorf("error %q must name sendAs as a suspect", err)
	}
}
