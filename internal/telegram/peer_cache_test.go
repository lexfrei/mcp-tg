package telegram

import "testing"

func TestNewPeerCache(t *testing.T) {
	cache := NewPeerCache()
	if cache == nil {
		t.Fatal("NewPeerCache() returned nil")
	}
}

func TestPeerCacheStoreAndLookup(t *testing.T) {
	cache := NewPeerCache()
	peer := InputPeer{
		Type:       PeerUser,
		ID:         12345,
		AccessHash: 9876,
	}

	cache.Store(peer)

	got, found := cache.Lookup(PeerUser, peer.ID)
	if !found {
		t.Fatal("expected peer to be found in cache")
	}

	if got.ID != peer.ID {
		t.Errorf("got ID %d, want %d", got.ID, peer.ID)
	}

	if got.AccessHash != peer.AccessHash {
		t.Errorf("got AccessHash %d, want %d",
			got.AccessHash, peer.AccessHash)
	}

	if got.Type != peer.Type {
		t.Errorf("got Type %d, want %d", got.Type, peer.Type)
	}
}

func TestPeerCacheLookupMiss(t *testing.T) {
	cache := NewPeerCache()

	_, found := cache.Lookup(PeerUser, 99999)
	if found {
		t.Error("expected lookup miss for unknown ID")
	}
}

func TestPeerCacheOverwrite(t *testing.T) {
	cache := NewPeerCache()
	peer := InputPeer{
		Type:       PeerChannel,
		ID:         100,
		AccessHash: 111,
	}

	cache.Store(peer)

	updated := InputPeer{
		Type:       PeerChannel,
		ID:         100,
		AccessHash: 222,
	}

	cache.Store(updated)

	got, found := cache.Lookup(PeerChannel, 100)
	if !found {
		t.Fatal("expected peer to be found after overwrite")
	}

	if got.AccessHash != 222 {
		t.Errorf("got AccessHash %d, want 222", got.AccessHash)
	}
}

func TestPeerCacheSkipsZeroAccessHash(t *testing.T) {
	cache := NewPeerCache()
	peer := InputPeer{
		Type:       PeerUser,
		ID:         555,
		AccessHash: 0,
	}

	cache.Store(peer)

	_, found := cache.Lookup(PeerUser, 555)
	if found {
		t.Error("expected cache to skip peer with zero access hash")
	}
}

func TestPeerCacheDoesNotOverwriteWithZero(t *testing.T) {
	cache := NewPeerCache()
	good := InputPeer{
		Type:       PeerUser,
		ID:         777,
		AccessHash: 999,
	}

	cache.Store(good)

	bad := InputPeer{
		Type:       PeerUser,
		ID:         777,
		AccessHash: 0,
	}

	cache.Store(bad)

	got, found := cache.Lookup(PeerUser, 777)
	if !found {
		t.Fatal("expected peer to still be in cache")
	}

	if got.AccessHash != 999 {
		t.Errorf("got AccessHash %d, want 999", got.AccessHash)
	}
}

func TestPeerCacheMultiplePeers(t *testing.T) {
	cache := NewPeerCache()
	peers := []InputPeer{
		{Type: PeerUser, ID: 1, AccessHash: 10},
		{Type: PeerChannel, ID: 2, AccessHash: 20},
		{Type: PeerChat, ID: 3, AccessHash: 30},
	}

	for _, peer := range peers {
		cache.Store(peer)
	}

	for _, want := range peers {
		got, found := cache.Lookup(want.Type, want.ID)
		if !found {
			t.Errorf("peer %d/%d not found", want.Type, want.ID)
			continue
		}

		if got.AccessHash != want.AccessHash {
			t.Errorf("peer %d/%d: got AccessHash %d, want %d",
				want.Type, want.ID, got.AccessHash, want.AccessHash)
		}
	}
}

func TestPeerCacheStoreAll(t *testing.T) {
	cache := NewPeerCache()
	peers := []InputPeer{
		{Type: PeerUser, ID: 1, AccessHash: 10},
		{Type: PeerChannel, ID: 2, AccessHash: 0},
		{Type: PeerChannel, ID: 3, AccessHash: 30},
	}

	cache.StoreAll(peers)

	if _, found := cache.Lookup(PeerUser, 1); !found {
		t.Error("peer User/1 should be cached")
	}

	if _, found := cache.Lookup(PeerChannel, 2); found {
		t.Error("peer Channel/2 should NOT be cached (zero access hash)")
	}

	if _, found := cache.Lookup(PeerChannel, 3); !found {
		t.Error("peer Channel/3 should be cached")
	}
}

func TestPeerCacheNoCollision(t *testing.T) {
	cache := NewPeerCache()

	cache.Store(InputPeer{Type: PeerUser, ID: 123, AccessHash: 111})
	cache.Store(InputPeer{Type: PeerChannel, ID: 123, AccessHash: 222})

	usr, found := cache.Lookup(PeerUser, 123)
	if !found || usr.AccessHash != 111 {
		t.Errorf("User 123: got %+v, want AccessHash 111", usr)
	}

	chn, found := cache.Lookup(PeerChannel, 123)
	if !found || chn.AccessHash != 222 {
		t.Errorf("Channel 123: got %+v, want AccessHash 222", chn)
	}
}
