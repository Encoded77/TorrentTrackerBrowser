package prowlarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

const torrentBytes = "d8:announce9:http://tr4:infod6:lengthi1234e4:name8:file.mkv12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newServer fakes Prowlarr: indexer list, per-indexer search, and download
// endpoints that redirect to a magnet, serve a torrent, or fail.
func newServer(t *testing.T) (*httptest.Server, *[]*http.Request) {
	t.Helper()
	var mu sync.Mutex
	var seen []*http.Request
	mux := http.NewServeMux()
	record := func(r *http.Request) {
		mu.Lock()
		seen = append(seen, r)
		mu.Unlock()
	}
	mux.HandleFunc("/api/v1/indexer", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if r.Header.Get("X-Api-Key") != "key" {
			w.WriteHeader(401)
			return
		}
		w.Write(fixture(t, "indexers.json"))
	})
	mux.HandleFunc("/api/v1/search", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		switch r.URL.Query().Get("indexerIds") {
		case "16":
			w.Write(fixture(t, "search_16.json"))
		case "3":
			w.Write([]byte("[]"))
		default:
			http.Error(w, `{"message":"indexer is down"}`, 500)
		}
	})
	mux.HandleFunc("/16/download", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		switch r.URL.Query().Get("link") {
		case "abc":
			http.Redirect(w, r, "magnet:?xt=urn:btih:0723937B3B5D7A2C4F7E8A9B0C1D2E3F40516273&dn=Dune", 302)
		case "game":
			http.Redirect(w, r, "/hop", 301)
		default:
			http.Error(w, "nope", 404)
		}
	})
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write([]byte(torrentBytes))
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", 302)
	})
	mux.HandleFunc("/html", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>login</html>"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &seen
}

func TestIndexersOnlyEnabled(t *testing.T) {
	srv, _ := newServer(t)
	s, err := New("prowlarr", srv.URL+"/", "key")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := s.Indexers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 3 {
		t.Fatalf("indexers = %+v", idx)
	}
	if idx[0].ID != "prowlarr:16" || idx[0].Name != "TR4KER" || !idx[0].Private {
		t.Errorf("first = %+v", idx[0])
	}
	if idx[1].ID != "prowlarr:3" || !idx[1].Private {
		t.Errorf("semiPrivate should count as private: %+v", idx[1])
	}
	if idx[2].ID != "prowlarr:9" || idx[2].Private {
		t.Errorf("public: %+v", idx[2])
	}
}

func TestIndexersBadKey(t *testing.T) {
	srv, _ := newServer(t)
	s, _ := New("prowlarr", srv.URL, "wrong")
	if _, err := s.Indexers(context.Background()); err == nil {
		t.Error("expected an auth error")
	}
}

func TestSearchFanOut(t *testing.T) {
	srv, seen := newServer(t)
	s, _ := New("prowlarr", srv.URL, "key")
	var mu sync.Mutex
	events := map[string]core.SearchEvent{}
	err := s.Search(context.Background(), core.Query{Text: "dune", Categories: []core.Canonical{core.CatMovies, core.CatGames}, Limit: 50,
		Indexers: []string{"prowlarr:16", "prowlarr:3", "prowlarr:9"}}, func(ev core.SearchEvent) {
		mu.Lock()
		events[ev.Indexer.ID] = ev
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d", len(events))
	}
	ev := events["prowlarr:16"]
	if !ev.Done || ev.Err != nil || len(ev.Results) != 3 {
		t.Fatalf("16 = %+v", ev)
	}
	r := ev.Results[0]
	if r.Indexer != "prowlarr:16" || r.Source != "prowlarr" || r.InfoHash != "0723937b3b5d7a2c4f7e8a9b0c1d2e3f40516273" || !r.Freeleech ||
		r.HasMagnet || !r.HasTorrent || r.Seeders != 120 || r.Published.Year() != 2026 || r.InfoURL == "" {
		t.Errorf("result = %+v", r)
	}
	if len(r.Categories) != 1 || r.Categories[0] != core.CatMovies {
		t.Errorf("categories = %v", r.Categories)
	}
	if r.Ref["download"] == "" || r.Ref["guid"] == "" {
		t.Errorf("ref = %v", r.Ref)
	}
	r2 := ev.Results[1]
	if !r2.HasMagnet || !r2.Freeleech || len(r2.Categories) != 2 || r2.Categories[0] != core.CatTV || r2.Categories[1] != core.CatAnime {
		t.Errorf("result 2 = %+v", r2)
	}
	if ev.Results[2].Categories[0] != core.CatGames {
		t.Errorf("4050 should map to games: %v", ev.Results[2].Categories)
	}
	if e3 := events["prowlarr:3"]; e3.Err != nil || len(e3.Results) != 0 {
		t.Errorf("3 = %+v", e3)
	}
	if e9 := events["prowlarr:9"]; e9.Err == nil || !e9.Done {
		t.Errorf("9 should fail: %+v", e9)
	}

	// Query parameters: one request per indexer, categories expanded.
	var searches []string
	for _, r := range *seen {
		if r.URL.Path == "/api/v1/search" {
			searches = append(searches, r.URL.RawQuery)
		}
	}
	sort.Strings(searches)
	if len(searches) != 3 {
		t.Fatalf("search requests = %v", searches)
	}
	q := searches[0]
	for _, want := range []string{"query=dune", "type=search", "limit=50", "categories=2000", "categories=1000", "categories=4050"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q lacks %s", q, want)
		}
	}
}

func TestFetchMagnetFromRef(t *testing.T) {
	srv, _ := newServer(t)
	s, _ := New("prowlarr", srv.URL, "key")
	p, err := s.Fetch(context.Background(), core.Result{Title: "T", Ref: map[string]string{"magnet": "magnet:?xt=urn:btih:" + strings.Repeat("ab", 20)}})
	if err != nil || p.Magnet == "" || p.InfoHash != strings.Repeat("ab", 20) || p.Name != "T" {
		t.Errorf("payload = %+v err=%v", p, err)
	}
}

func TestFetchRedirectToMagnet(t *testing.T) {
	srv, _ := newServer(t)
	s, _ := New("prowlarr", srv.URL, "key")
	p, err := s.Fetch(context.Background(), core.Result{Ref: map[string]string{"download": srv.URL + "/16/download?link=abc"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Magnet == "" || p.InfoHash != "0723937b3b5d7a2c4f7e8a9b0c1d2e3f40516273" || p.Name != "Dune" || len(p.Torrent) != 0 {
		t.Errorf("payload = %+v", p)
	}
}

func TestFetchTorrentBody(t *testing.T) {
	srv, _ := newServer(t)
	s, _ := New("prowlarr", srv.URL, "key")
	p, err := s.Fetch(context.Background(), core.Result{Ref: map[string]string{"download": srv.URL + "/16/download?link=game"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(p.Torrent) != torrentBytes || p.Name != "file.mkv" || p.Size != 1234 || len(p.Files) != 1 || p.InfoHash == "" {
		t.Errorf("payload = %+v", p)
	}
}

func TestFetchErrors(t *testing.T) {
	srv, _ := newServer(t)
	s, _ := New("prowlarr", srv.URL, "key")
	for name, ref := range map[string]map[string]string{
		"404":         {"download": srv.URL + "/16/download?link=zzz"},
		"loop":        {"download": srv.URL + "/loop"},
		"not-bencode": {"download": srv.URL + "/html"},
		"empty":       {},
	} {
		if _, err := s.Fetch(context.Background(), core.Result{Ref: ref}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
