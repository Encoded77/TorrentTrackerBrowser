package core

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// bencode decoder: strings decode to string, integers to int64, lists to
// []any and dictionaries to map[string]any. The raw span of the top-level
// "info" dictionary is recorded so its SHA-1 can be computed.
type bdecoder struct {
	b         []byte
	pos       int
	depth     int
	infoStart int
	infoEnd   int
}

func (d *bdecoder) next() (any, error) {
	if d.pos >= len(d.b) {
		return nil, errors.New("bencode: unexpected end")
	}
	switch c := d.b[d.pos]; {
	case c == 'i':
		end := d.find('e')
		if end < 0 {
			return nil, errors.New("bencode: unterminated integer")
		}
		n, err := strconv.ParseInt(string(d.b[d.pos+1:end]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bencode: bad integer: %w", err)
		}
		d.pos = end + 1
		return n, nil
	case c == 'l':
		d.pos++
		list := []any{}
		for d.pos < len(d.b) && d.b[d.pos] != 'e' {
			v, err := d.next()
			if err != nil {
				return nil, err
			}
			list = append(list, v)
		}
		if d.pos >= len(d.b) {
			return nil, errors.New("bencode: unterminated list")
		}
		d.pos++
		return list, nil
	case c == 'd':
		d.pos++
		d.depth++
		dict := map[string]any{}
		for d.pos < len(d.b) && d.b[d.pos] != 'e' {
			k, err := d.next()
			if err != nil {
				return nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, errors.New("bencode: dictionary key is not a string")
			}
			start := d.pos
			v, err := d.next()
			if err != nil {
				return nil, err
			}
			if d.depth == 1 && key == "info" {
				d.infoStart, d.infoEnd = start, d.pos
			}
			dict[key] = v
		}
		if d.pos >= len(d.b) {
			return nil, errors.New("bencode: unterminated dictionary")
		}
		d.pos++
		d.depth--
		return dict, nil
	case c >= '0' && c <= '9':
		colon := d.find(':')
		if colon < 0 {
			return nil, errors.New("bencode: bad string length")
		}
		n, err := strconv.Atoi(string(d.b[d.pos:colon]))
		if err != nil || n < 0 || colon+1+n > len(d.b) {
			return nil, errors.New("bencode: bad string length")
		}
		s := string(d.b[colon+1 : colon+1+n])
		d.pos = colon + 1 + n
		return s, nil
	}
	return nil, fmt.Errorf("bencode: unexpected byte %q at %d", d.b[d.pos], d.pos)
}

func (d *bdecoder) find(c byte) int {
	for i := d.pos; i < len(d.b); i++ {
		if d.b[i] == c {
			return i
		}
	}
	return -1
}

// TorrentInfo is what the app needs from a .torrent file.
type TorrentInfo struct {
	Name     string
	InfoHash string // lowercase hex
	Size     int64
	Files    []FileEntry
}

// ParseTorrent decodes .torrent bytes and computes the info hash.
func ParseTorrent(b []byte) (TorrentInfo, error) {
	d := &bdecoder{b: b}
	v, err := d.next()
	if err != nil {
		return TorrentInfo{}, err
	}
	top, ok := v.(map[string]any)
	if !ok || d.infoEnd == 0 {
		return TorrentInfo{}, errors.New("bencode: not a torrent (no info dictionary)")
	}
	info, _ := top["info"].(map[string]any)
	if info == nil {
		return TorrentInfo{}, errors.New("bencode: info is not a dictionary")
	}
	sum := sha1.Sum(b[d.infoStart:d.infoEnd])
	t := TorrentInfo{InfoHash: hex.EncodeToString(sum[:])}
	t.Name, _ = info["name"].(string)
	if files, ok := info["files"].([]any); ok {
		for i, f := range files {
			fm, _ := f.(map[string]any)
			parts := []string{}
			if p, ok := fm["path"].([]any); ok {
				for _, e := range p {
					if s, ok := e.(string); ok {
						parts = append(parts, s)
					}
				}
			}
			size, _ := fm["length"].(int64)
			path := strings.Join(parts, "/")
			if t.Name != "" {
				path = t.Name + "/" + path
			}
			t.Files = append(t.Files, FileEntry{Index: i, Path: path, Size: size})
			t.Size += size
		}
	} else {
		size, _ := info["length"].(int64)
		t.Size = size
		t.Files = []FileEntry{{Index: 0, Path: t.Name, Size: size}}
	}
	return t, nil
}
