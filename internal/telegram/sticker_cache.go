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
func (cache *StickerCache) StoreAll(documents []tg.DocumentClass) {
	if cache == nil {
		return
	}

	cache.mut.Lock()
	defer cache.mut.Unlock()

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

	cache.evictIfNeeded()
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

// evictIfNeeded clears the cache when it exceeds the size limit.
// Must be called with mut held.
func (cache *StickerCache) evictIfNeeded() {
	if len(cache.entries) <= maxStickerCacheEntries {
		return
	}

	clear(cache.entries)
}
