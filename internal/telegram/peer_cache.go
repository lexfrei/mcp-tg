package telegram

import "sync"

// PeerCache stores resolved peers with their access hashes for reuse.
// When peers are resolved via username, they come with valid access hashes.
// This cache allows numeric-ID lookups to reuse those hashes instead of
// falling back to AccessHash=0, which causes many Telegram API calls to fail.
type PeerCache struct {
	mut  sync.RWMutex
	byID map[int64]InputPeer
}

// NewPeerCache creates an empty peer cache.
func NewPeerCache() *PeerCache {
	return &PeerCache{
		byID: make(map[int64]InputPeer),
	}
}

// Store saves a peer in the cache. Peers with AccessHash=0 are ignored
// to prevent overwriting previously cached valid access hashes.
func (cache *PeerCache) Store(peer InputPeer) {
	if peer.AccessHash == 0 {
		return
	}

	cache.mut.Lock()
	defer cache.mut.Unlock()

	cache.byID[peer.ID] = peer
}

// StoreAll saves multiple peers in the cache, skipping those
// with AccessHash=0.
func (cache *PeerCache) StoreAll(peers []InputPeer) {
	cache.mut.Lock()
	defer cache.mut.Unlock()

	for _, peer := range peers {
		if peer.AccessHash != 0 {
			cache.byID[peer.ID] = peer
		}
	}
}

// Lookup returns a cached peer by ID if available.
func (cache *PeerCache) Lookup(peerID int64) (InputPeer, bool) {
	cache.mut.RLock()
	defer cache.mut.RUnlock()

	peer, found := cache.byID[peerID]

	return peer, found
}
