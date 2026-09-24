package core

import (
	"fmt"
	"time"
)

// PayloadCache keeps user-supplied payloads ("p_...") until a job uses them.
type PayloadCache struct {
	c *ttlCache[Payload]
}

// NewPayloadCache builds a cache with the given TTL and entry cap.
func NewPayloadCache(ttl time.Duration, capacity int) *PayloadCache {
	return &PayloadCache{c: newTTLCache[Payload]("p_", ttl, capacity)}
}

// Add stores p and returns its id.
func (pc *PayloadCache) Add(p Payload) string {
	return pc.c.put(func(string) Payload { return p })
}

// Get returns a cached payload by id.
func (pc *PayloadCache) Get(id string) (Payload, bool) { return pc.c.get(id) }

// PayloadFromMagnet builds a Payload from a magnet URI.
func PayloadFromMagnet(s string) (Payload, error) {
	m, err := ParseMagnet(s)
	if err != nil {
		return Payload{}, err
	}
	return Payload{Name: m.Name, InfoHash: m.InfoHash, Magnet: m.Raw}, nil
}

// PayloadFromTorrent builds a Payload from .torrent bytes.
func PayloadFromTorrent(b []byte) (Payload, error) {
	t, err := ParseTorrent(b)
	if err != nil {
		return Payload{}, fmt.Errorf("invalid .torrent: %w", err)
	}
	return Payload{Name: t.Name, InfoHash: t.InfoHash, Torrent: b, Size: t.Size, Files: t.Files}, nil
}
