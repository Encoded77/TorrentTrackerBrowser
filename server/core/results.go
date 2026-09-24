package core

import (
	"context"
	"time"
)

// ResultCache assigns short-lived ids ("r_...") to search results so the
// client can refer to them without ever sending URLs back.
type ResultCache struct {
	c *ttlCache[Result]
}

// NewResultCache builds a cache with the given TTL and entry cap.
func NewResultCache(ttl time.Duration, capacity int) *ResultCache {
	return &ResultCache{c: newTTLCache[Result]("r_", ttl, capacity)}
}

// Add stores r, assigns its ID and returns the stored copy.
func (rc *ResultCache) Add(r Result) Result {
	r.ID = rc.c.put(func(id string) Result { r.ID = id; return r })
	return r
}

// Get returns a cached result by id.
func (rc *ResultCache) Get(id string) (Result, bool) { return rc.c.get(id) }

// Previewer is an optional Engine extension: list the files of a torrent the
// engine already knows (cached hash) without adding it.
type Previewer interface {
	Preview(ctx context.Context, infoHash string) ([]FileEntry, error)
}

// TorrentFetcher is an optional Source extension: fetch the .torrent form of
// a result even when a magnet exists (previews and "get .torrent").
type TorrentFetcher interface {
	FetchTorrent(ctx context.Context, r Result) (Payload, error)
}
