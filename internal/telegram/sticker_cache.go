package telegram

import (
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// maxStickerCacheEntries bounds the sticker document cache. A handful of
// sets is the realistic working set; the limit only stops an unbounded
// walk of the sticker catalogue from growing the process forever.
const maxStickerCacheEntries = 5000

// ErrStickerNotCached is returned when a sticker is sent before its set
// has been read.
//
// inputDocument carries an id, an access hash and a file reference.
// Only the id is a stable public number; the other two are handed out
// with the sticker set and cannot be derived from it. Sending an id
// alone answers MEDIA_EMPTY, which names neither the sticker nor the
// remedy.
var ErrStickerNotCached = errors.New(
	"sticker is unknown; call tg_stickers_get_set for its set first, " +
		"which caches the access hash and file reference the send needs",
)

// StickerDoc is the trio inputDocument needs to reference a sticker.
type StickerDoc struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
}

// StickerCache remembers sticker documents seen in sticker sets so a
// later send can address them.
type StickerCache struct {
	mut     sync.RWMutex
	entries map[int64]StickerDoc
}

// NewStickerCache creates an empty sticker document cache.
func NewStickerCache() *StickerCache {
	return &StickerCache{entries: make(map[int64]StickerDoc)}
}

// StoreAll remembers every sticker document of a set.
//
// Room is made before inserting, never after: evicting afterwards could
// drop the very set just fetched, and the caller would be told to fetch
// it again by the send that follows.
func (cache *StickerCache) StoreAll(documents []tg.DocumentClass) {
	if cache == nil {
		return
	}

	cache.mut.Lock()
	defer cache.mut.Unlock()

	cache.evictFor(len(documents))

	for _, doc := range documents {
		typed, ok := doc.(*tg.Document)
		if !ok {
			continue
		}

		cache.entries[typed.ID] = StickerDoc{
			ID:            typed.ID,
			AccessHash:    typed.AccessHash,
			FileReference: typed.FileReference,
		}
	}
}

// Lookup returns a cached sticker document by its file ID.
func (cache *StickerCache) Lookup(fileID int64) (StickerDoc, bool) {
	if cache == nil {
		return StickerDoc{}, false
	}

	cache.mut.RLock()
	defer cache.mut.RUnlock()

	doc, found := cache.entries[fileID]

	return doc, found
}

// evictFor clears the cache when the incoming documents would not fit.
// Eviction drops everything rather than tracking recency: a sticker set
// holds a few hundred documents, so the limit is a runaway guard, not a
// working-set policy. Must be called with mut held.
func (cache *StickerCache) evictFor(incoming int) {
	if len(cache.entries)+incoming <= maxStickerCacheEntries {
		return
	}

	clear(cache.entries)
}
