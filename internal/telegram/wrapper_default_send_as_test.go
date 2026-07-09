package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// fullChannelInvoker answers channels.getFullChannel with the supplied
// full-chat payload and nothing else.
type fullChannelInvoker struct {
	full *tg.MessagesChatFull
}

func (f *fullChannelInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.ChannelsGetFullChannelRequest); ok {
		return encodeResp(f.full, output)
	}

	return errUnexpectedRequest
}

// chatFullWithDefaultSendAs builds a getFullChannel response for a
// supergroup that posts as ownChannelID by default. The identity itself
// appears in Chats, which is the only place its access hash and title
// exist — ChannelFull.default_send_as is a bare peer.
func chatFullWithDefaultSendAs(withIdentity bool) *tg.MessagesChatFull {
	full := &tg.ChannelFull{ID: 1, About: "about", ChatPhoto: &tg.PhotoEmpty{}}
	if withIdentity {
		full.SetDefaultSendAs(&tg.PeerChannel{ChannelID: ownChannelID})
	}

	return &tg.MessagesChatFull{
		FullChat: full,
		Chats: []tg.ChatClass{
			&tg.Channel{
				ID: 1, AccessHash: 2, Title: "Group",
				Megagroup: true, Photo: &tg.ChatPhotoEmpty{},
			},
			&tg.Channel{
				ID: ownChannelID, AccessHash: ownChannelHash,
				Title: "My Channel", Username: "mychan", Photo: &tg.ChatPhotoEmpty{},
			},
		},
	}
}

func groupInfoWrapper(t *testing.T, withIdentity bool) (*Wrapper, *GroupInfo) {
	t.Helper()

	inv := &fullChannelInvoker{full: chatFullWithDefaultSendAs(withIdentity)}
	wrap := &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}

	info, err := wrap.GetGroupInfo(t.Context(), InputPeer{Type: PeerChannel, ID: 1, AccessHash: 2})
	if err != nil {
		t.Fatalf("GetGroupInfo: %v", err)
	}

	return wrap, info
}

func groupInfoFor(t *testing.T, withIdentity bool) *GroupInfo {
	t.Helper()

	_, info := groupInfoWrapper(t, withIdentity)

	return info
}

// tg_groups_info hands the default identity back as a numeric peer string.
// A caller passing that straight into sendAs would be rejected — a numeric
// channel resolves with access hash 0 — unless the hash we just read from
// this very reply is remembered.
func TestGetGroupInfo_SeedsDefaultSendAsIntoPeerCache(t *testing.T) {
	wrap, _ := groupInfoWrapper(t, true)

	cached, ok := wrap.cache.Lookup(PeerChannel, ownChannelID)
	if !ok {
		t.Fatal("the default send-as identity was not seeded into the peer cache")
	}

	if cached.AccessHash != ownChannelHash {
		t.Errorf("cached access hash = %d, want %d", cached.AccessHash, ownChannelHash)
	}
}

func TestGetGroupInfo_ExposesDefaultSendAs(t *testing.T) {
	info := groupInfoFor(t, true)

	if info.DefaultSendAs == nil {
		t.Fatal("DefaultSendAs is nil even though the channel has one set")
	}

	want := SendAsOption{
		Peer:     InputPeer{Type: PeerChannel, ID: ownChannelID, AccessHash: ownChannelHash},
		Name:     "My Channel",
		Username: "mychan",
	}

	if *info.DefaultSendAs != want {
		t.Errorf("DefaultSendAs = %+v, want %+v", *info.DefaultSendAs, want)
	}
}

func TestGetGroupInfo_NoDefaultSendAsLeavesNil(t *testing.T) {
	info := groupInfoFor(t, false)

	if info.DefaultSendAs != nil {
		t.Errorf("DefaultSendAs = %+v, want nil when the channel has none", *info.DefaultSendAs)
	}
}
