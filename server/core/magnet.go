package core

import (
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Magnet is the parsed form of a magnet URI.
type Magnet struct {
	InfoHash string // lowercase hex
	Name     string
	Trackers []string
	Raw      string
}

// ParseMagnet parses a magnet URI with a BitTorrent info hash (hex or
// base32) and returns the normalized fields.
func ParseMagnet(s string) (Magnet, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(strings.ToLower(s), "magnet:?") {
		return Magnet{}, errors.New("not a magnet URI")
	}
	q, err := url.ParseQuery(s[len("magnet:?"):])
	if err != nil {
		return Magnet{}, fmt.Errorf("magnet: %w", err)
	}
	m := Magnet{Raw: s, Name: q.Get("dn"), Trackers: q["tr"]}
	for _, xt := range q["xt"] {
		if !strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
			continue
		}
		h, err := NormalizeInfoHash(xt[len("urn:btih:"):])
		if err != nil {
			return Magnet{}, err
		}
		m.InfoHash = h
		break
	}
	if m.InfoHash == "" {
		return Magnet{}, errors.New("magnet: no urn:btih info hash")
	}
	return m, nil
}

// NormalizeInfoHash accepts a 40-char hex or 32-char base32 info hash and
// returns lowercase hex.
func NormalizeInfoHash(h string) (string, error) {
	h = strings.TrimSpace(h)
	switch len(h) {
	case 40:
		if _, err := hex.DecodeString(h); err != nil {
			return "", fmt.Errorf("bad hex info hash %q", h)
		}
		return strings.ToLower(h), nil
	case 32:
		raw, err := base32.StdEncoding.DecodeString(strings.ToUpper(h))
		if err != nil {
			return "", fmt.Errorf("bad base32 info hash %q", h)
		}
		return hex.EncodeToString(raw), nil
	}
	return "", fmt.Errorf("bad info hash length %d", len(h))
}

// BuildMagnet assembles a magnet URI from a hash, a display name and trackers.
func BuildMagnet(infoHash, name string, trackers []string) string {
	v := url.Values{}
	v.Set("xt", "urn:btih:"+infoHash)
	if name != "" {
		v.Set("dn", name)
	}
	for _, t := range trackers {
		v.Add("tr", t)
	}
	return "magnet:?" + v.Encode()
}
