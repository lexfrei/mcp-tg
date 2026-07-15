package telegram

import "sync"

// maxCacheEntries bounds each generation of the peer cache. On overflow
// the cache ROTATES rather than clears: the current generation is demoted
// to a read-only fallback and a fresh generation starts, so entries
// written just before the overflow survive one more generation instead of
// being wiped. A single dialog warm inserts at most
// warmDialogsMaxPages * warmDialogsPageLimit * 2 folders = 10000 peers,
// well under this bound, so a warm triggers at most one rotation and its
// freshly-cached target is always still readable (in the fresh generation
// or the fallback) when ResolvePeer looks it up. Memory is bounded to two
// generations.
const maxCacheEntries = 30000

// peerKey uniquely identifies a peer by type and ID.
// Necessary because User ID=123 and Channel ID=123 are different entities.
type peerKey struct {
	typ PeerType
	id  int64
}

// PeerCache stores resolved peers with their access hashes for reuse.
// It bounds memory with a two-generation scheme: when the current
// generation exceeds maxCacheEntries it is demoted to a read-only
// fallback and a fresh one starts, so a burst of inserts (a dialog warm)
// never evicts the very entries it just wrote.
type PeerCache struct {
	mut     sync.RWMutex
	entries map[peerKey]InputPeer
	prev    map[peerKey]InputPeer
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
	cache.rotateIfNeeded()
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

	cache.rotateIfNeeded()
}

// Lookup returns a cached peer by type and ID if available. It checks the
// current generation first, then the read-only fallback, so an entry a
// recent rotation demoted is still found.
func (cache *PeerCache) Lookup(typ PeerType, peerID int64) (InputPeer, bool) {
	key := peerKey{typ: typ, id: peerID}

	cache.mut.RLock()
	defer cache.mut.RUnlock()

	if peer, found := cache.entries[key]; found {
		return peer, true
	}

	peer, found := cache.prev[key]

	return peer, found
}

// rotateIfNeeded demotes the current generation to the fallback and starts
// a fresh one when the size limit is exceeded, bounding memory to two
// generations without discarding the newest entries. Must be called with
// mut held.
func (cache *PeerCache) rotateIfNeeded() {
	if len(cache.entries) <= maxCacheEntries {
		return
	}

	cache.prev = cache.entries
	cache.entries = make(map[peerKey]InputPeer)
}
