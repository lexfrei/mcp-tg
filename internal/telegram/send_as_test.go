package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// IDs and access hashes of the canned channels.getSendAs response below.
const (
	selfUserID     = 10
	selfUserHash   = 11
	ownChannelID   = 20
	ownChannelHash = 21
	paidChannelID  = 30
	paidChanHash   = 31
)

// sendAsPeersInvoker answers channels.getSendAs with a canned response
// and captures messages.saveDefaultSendAs for assertions.
type sendAsPeersInvoker struct {
	getReq  *tg.ChannelsGetSendAsRequest
	saveReq *tg.MessagesSaveDefaultSendAsRequest
}

func (s *sendAsPeersInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch req := input.(type) {
	case *tg.ChannelsGetSendAsRequest:
		s.getReq = req

		return encodeResp(cannedSendAsPeers(), output)
	case *tg.MessagesSaveDefaultSendAsRequest:
		s.saveReq = req

		return encodeResp(&tg.BoolTrue{}, output)
	default:
		return errUnexpectedRequest
	}
}

// cannedSendAsPeers mirrors what the server sends: bare peers in Peers,
// their access hashes and display names only in Chats/Users.
func cannedSendAsPeers() *tg.ChannelsSendAsPeers {
	return &tg.ChannelsSendAsPeers{
		Peers: []tg.SendAsPeer{
			{Peer: &tg.PeerUser{UserID: selfUserID}},
			{Peer: &tg.PeerChannel{ChannelID: ownChannelID}},
			{Peer: &tg.PeerChannel{ChannelID: paidChannelID}, PremiumRequired: true},
		},
		Users: []tg.UserClass{
			&tg.User{ID: selfUserID, AccessHash: selfUserHash, FirstName: "Alice", Username: "alice"},
		},
		// Photo is a required field on channel#, not a conditional one,
		// so a nil there fails encoding rather than decoding as absent.
		Chats: []tg.ChatClass{
			&tg.Channel{
				ID: ownChannelID, AccessHash: ownChannelHash,
				Title: "My Channel", Username: "mychan", Photo: &tg.ChatPhotoEmpty{},
			},
			&tg.Channel{
				ID: paidChannelID, AccessHash: paidChanHash,
				Title: "Paid Channel", Photo: &tg.ChatPhotoEmpty{},
			},
		},
	}
}

func newSendAsPeersWrapper(inv *sendAsPeersInvoker) *Wrapper {
	return &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}
}

func TestGetSendAs_MapsPeersWithAccessHashesAndNames(t *testing.T) {
	inv := &sendAsPeersInvoker{}

	got, err := newSendAsPeersWrapper(inv).GetSendAs(t.Context(), targetPeer())
	if err != nil {
		t.Fatalf("GetSendAs: %v", err)
	}

	want := []SendAsOption{
		{
			Peer:     InputPeer{Type: PeerUser, ID: selfUserID, AccessHash: selfUserHash},
			Name:     "Alice",
			Username: "alice",
		},
		{
			Peer:     InputPeer{Type: PeerChannel, ID: ownChannelID, AccessHash: ownChannelHash},
			Name:     "My Channel",
			Username: "mychan",
		},
		{
			Peer:            InputPeer{Type: PeerChannel, ID: paidChannelID, AccessHash: paidChanHash},
			Name:            "Paid Channel",
			PremiumRequired: true,
		},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d options, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// The bare peers in the response carry no access hash. Without seeding
// the cache from Chats/Users, a later numeric-ID resolve of a private
// channel would build an InputPeerChannel with AccessHash 0.
func TestGetSendAs_SeedsPeerCache(t *testing.T) {
	wrap := newSendAsPeersWrapper(&sendAsPeersInvoker{})

	_, err := wrap.GetSendAs(t.Context(), targetPeer())
	if err != nil {
		t.Fatalf("GetSendAs: %v", err)
	}

	cached, ok := wrap.cache.Lookup(PeerChannel, ownChannelID)
	if !ok {
		t.Fatal("own channel was not seeded into the peer cache")
	}

	if cached.AccessHash != ownChannelHash {
		t.Errorf("cached access hash = %d, want %d", cached.AccessHash, ownChannelHash)
	}
}

func TestGetSendAs_RejectsNonChannelPeerWithoutRPC(t *testing.T) {
	inv := &sendAsPeersInvoker{}

	_, err := newSendAsPeersWrapper(inv).GetSendAs(
		t.Context(), InputPeer{Type: PeerUser, ID: 5, AccessHash: 6},
	)
	if !errors.Is(err, ErrSendAsUnsupportedPeer) {
		t.Fatalf("error = %v, want ErrSendAsUnsupportedPeer", err)
	}

	if inv.getReq != nil {
		t.Error("channels.getSendAs was invoked for a user peer")
	}
}

func TestSetDefaultSendAs_SendsIdentity(t *testing.T) {
	inv := &sendAsPeersInvoker{}
	identity := sendAsIdentity()

	err := newSendAsPeersWrapper(inv).SetDefaultSendAs(t.Context(), targetPeer(), &identity)
	if err != nil {
		t.Fatalf("SetDefaultSendAs: %v", err)
	}

	if inv.saveReq == nil {
		t.Fatal("messages.saveDefaultSendAs was never invoked")
	}

	assertSendAsIdentity(t, inv.saveReq.SendAs)
}

// A nil identity resets the chat back to the account itself. The domain
// InputPeer cannot express "self" (it has no access hash), so the
// wrapper must substitute inputPeerSelf rather than send a zero peer.
func TestSetDefaultSendAs_NilResetsToSelf(t *testing.T) {
	inv := &sendAsPeersInvoker{}

	err := newSendAsPeersWrapper(inv).SetDefaultSendAs(t.Context(), targetPeer(), nil)
	if err != nil {
		t.Fatalf("SetDefaultSendAs: %v", err)
	}

	if inv.saveReq == nil {
		t.Fatal("messages.saveDefaultSendAs was never invoked")
	}

	if _, ok := inv.saveReq.SendAs.(*tg.InputPeerSelf); !ok {
		t.Errorf("send_as is %T, want *tg.InputPeerSelf", inv.saveReq.SendAs)
	}
}

func TestSetDefaultSendAs_RejectsNonChannelPeerWithoutRPC(t *testing.T) {
	inv := &sendAsPeersInvoker{}

	err := newSendAsPeersWrapper(inv).SetDefaultSendAs(
		t.Context(), InputPeer{Type: PeerUser, ID: 5, AccessHash: 6}, nil,
	)
	if !errors.Is(err, ErrSendAsUnsupportedPeer) {
		t.Fatalf("error = %v, want ErrSendAsUnsupportedPeer", err)
	}

	if inv.saveReq != nil {
		t.Error("messages.saveDefaultSendAs was invoked for a user peer")
	}
}
