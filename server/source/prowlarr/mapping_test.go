package prowlarr

import (
	"testing"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// Prowlarr proxies magnets through its own download endpoint and keeps the
// raw magnet in guid; indexer-specific category ids (100000+) ride along the
// standard ones. Both were observed live against Prowlarr 2.6.
func TestToResultLiveShapes(t *testing.T) {
	s := &Source{id: "prowlarr"}
	proxied := s.toResult(releaseDTO{
		Title:      "Dune Part Two (2024) [1080p]",
		GUID:       "magnet:?xt=urn:btih:2770FE270845674966E184BE60ED1BE0FE494F3A&dn=Dune",
		MagnetURL:  "http://prowlarr:9696/9/download?apikey=x&link=abc",
		InfoHash:   "2770FE270845674966E184BE60ED1BE0FE494F3A",
		Categories: []categoryDTO{{ID: 2040}, {ID: 100207}},
	}, "prowlarr:9")
	if !proxied.HasMagnet || proxied.HasTorrent {
		t.Errorf("flags = magnet %v torrent %v", proxied.HasMagnet, proxied.HasTorrent)
	}
	if proxied.Ref["magnet"] == "" || proxied.Ref["magnet"][:7] != "magnet:" {
		t.Errorf("magnet ref = %q", proxied.Ref["magnet"])
	}
	if len(proxied.Categories) != 1 || proxied.Categories[0] != core.CatMovies {
		t.Errorf("categories = %v", proxied.Categories)
	}

	private := s.toResult(releaseDTO{
		Title:       "Dune 2021 MULTi TRUEFRENCH 1080p",
		GUID:        "https://tracker/api/torrents/dune/download?apikey=y",
		DownloadURL: "http://prowlarr:9696/5/download?apikey=x&link=def",
		Categories:  []categoryDTO{{ID: 2010}, {ID: 102030}},
	}, "prowlarr:5")
	if private.HasMagnet || !private.HasTorrent || private.Ref["magnet"] != "" {
		t.Errorf("private = %+v", private)
	}

	onlyCustom := s.toResult(releaseDTO{Title: "x", Categories: []categoryDTO{{ID: 100001}}}, "prowlarr:1")
	if len(onlyCustom.Categories) != 1 || onlyCustom.Categories[0] != core.CatOther {
		t.Errorf("custom-only categories = %v", onlyCustom.Categories)
	}
}
