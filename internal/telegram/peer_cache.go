package telegram

import "sync"

const maxCacheEntries = 10000

// peerKey uniquely identifies a peer by type and ID.
// Necessary because User ID=123 and Channel ID=123 are different entities.
type peerKey struct {
	typ PeerType
	id  int64
}

// PeerCache stores resolved peers with their access hashes for reuse.
// Evicts all entries when maxCacheEntries is exceeded to bound memory.
type PeerCache struct {
	mut     sync.RWMutex
	entries map[peerKey]InputPeer
}

// NewPeerCache creates an empty peer cache.
func NewPeerCache() *PeerCache {
	return &PeerCache{
		entries: make(map[peerKey]InputPeer),
	}
}

// Store saves a peer in the cache. Peers with AccessHash=0 are ignored
// to prevent overwriting previously cached valid access hashes.
func (cache *PeerCache) Store(peer InputPeer) {
	if peer.AccessHash == 0 {
		return
	}

	key := peerKey{typ: peer.Type, id: peer.ID}

	cache.mut.Lock()
	defer cache.mut.Unlock()

	cache.entries[key] = peer
	cache.evictIfNeeded()
}

// StoreAll saves multiple peers in the cache, skipping those
// with AccessHash=0.
func (cache *PeerCache) StoreAll(peers []InputPeer) {
	cache.mut.Lock()
	defer cache.mut.Unlock()

	for _, peer := range peers {
		if peer.AccessHash != 0 {
			key := peerKey{typ: peer.Type, id: peer.ID}
			cache.entries[key] = peer
		}
	}

	cache.evictIfNeeded()
}

// Lookup returns a cached peer by type and ID if available.
func (cache *PeerCache) Lookup(typ PeerType, peerID int64) (InputPeer, bool) {
	key := peerKey{typ: typ, id: peerID}

	cache.mut.RLock()
	defer cache.mut.RUnlock()

	peer, found := cache.entries[key]

	return peer, found
}

// evictIfNeeded clears the cache when it exceeds the size limit.
// Must be called with mut held.
func (cache *PeerCache) evictIfNeeded() {
	if len(cache.entries) <= maxCacheEntries {
		return
	}

	clear(cache.entries)
}
