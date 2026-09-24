package torbox

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

const hash = "0723937b3b5d7a2c4f7e8a9b0c1d2e3f40516273"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type fakeTorBox struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	requests []*http.Request
	forms    []map[string]string
	fail429  int // remaining 429 answers on mylist
}

func newFakeTorBox(t *testing.T) *fakeTorBox {
	f := &fakeTorBox{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			w.WriteHeader(401)
			w.Write([]byte(`{"success":false,"error":"AUTH_ERROR","detail":"bad key","data":null}`))
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, r)
		f.mu.Unlock()
		path := strings.TrimPrefix(r.URL.Path, "/v1/api/")
		switch path {
		case "torrents/createtorrent":
			r.ParseMultipartForm(1 << 20)
			form := map[string]string{}
			for k, v := range r.MultipartForm.Value {
				form[k] = v[0]
			}
			if fh := r.MultipartForm.File["file"]; len(fh) > 0 {
				fp, _ := fh[0].Open()
				b, _ := io.ReadAll(fp)
				form["file"] = string(b)
				form["filename"] = fh[0].Filename
			}
			f.mu.Lock()
			f.forms = append(f.forms, form)
			f.mu.Unlock()
			w.Write(fixture(t, "createtorrent.json"))
		case "torrents/mylist":
			f.mu.Lock()
			retry := f.fail429 > 0
			if retry {
				f.fail429--
			}
			f.mu.Unlock()
			if retry {
				w.WriteHeader(429)
				w.Write([]byte(`{"success":false,"error":"RATE_LIMIT","detail":"slow down","data":null}`))
				return
			}
			switch r.URL.Query().Get("id") {
			case "":
				w.Write(fixture(t, "mylist_all.json"))
			case "4321":
				w.Write(fixture(t, "mylist_one.json"))
			default:
				w.WriteHeader(404)
				w.Write(fixture(t, "error.json"))
			}
		case "torrents/checkcached":
			if strings.Contains(r.URL.Query().Get("hash"), hash) {
				w.Write(fixture(t, "checkcached.json"))
			} else {
				w.Write([]byte(`{"success":true,"error":null,"detail":"ok","data":false}`))
			}
		case "torrents/requestdl":
			body := strings.Replace(string(fixture(t, "requestdl.json")), "REPLACED_AT_RUNTIME", f.srv.URL+"/cdn/"+r.URL.Query().Get("file_id"), 1)
			w.Write([]byte(body))
		case "torrents/controltorrent":
			b, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(b), `"operation":"delete"`) || !strings.Contains(string(b), `"torrent_id":4321`) {
				w.WriteHeader(400)
				w.Write([]byte(`{"success":false,"error":"BAD","detail":"bad body","data":null}`))
				return
			}
			w.Write([]byte(`{"success":true,"error":null,"detail":"deleted","data":null}`))
		case "torrents/torrentinfo":
			w.Write(fixture(t, "torrentinfo.json"))
		default:
			w.WriteHeader(404)
		}
	})
	mux.HandleFunc("/cdn/", func(w http.ResponseWriter, r *http.Request) {
		content := "0123456789"
		if rng := r.Header.Get("Range"); rng != "" {
			w.Header().Set("Content-Range", "bytes 4-9/10")
			w.WriteHeader(206)
			w.Write([]byte(content[4:]))
			return
		}
		w.Write([]byte(content))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newEngine(t *testing.T, f *fakeTorBox) *Engine {
	e, err := New("torbox", f.srv.URL+"/v1/api", "key")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAddMagnetAndTorrent(t *testing.T) {
	f := newFakeTorBox(t)
	e := newEngine(t, f)
	it, err := e.Add(context.Background(), core.Payload{Name: "Dune", Magnet: "magnet:?xt=urn:btih:" + hash, InfoHash: hash}, core.AddOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != "4321" || it.InfoHash != hash || it.Name != "Dune" {
		t.Errorf("item = %+v", it)
	}
	form := f.forms[0]
	if form["magnet"] == "" || form["seed"] != "1" || form["allow_zip"] != "false" || form["name"] != "Dune" || form["file"] != "" {
		t.Errorf("form = %v", form)
	}
	if _, err := e.Add(context.Background(), core.Payload{Name: "T", Torrent: []byte("d4:infoe"), InfoHash: hash}, core.AddOpts{}); err != nil {
		t.Fatal(err)
	}
	form = f.forms[1]
	if form["file"] != "d4:infoe" || form["filename"] != "T.torrent" || form["magnet"] != "" {
		t.Errorf("torrent form = %v", form)
	}
	if _, err := e.Add(context.Background(), core.Payload{}, core.AddOpts{}); err == nil {
		t.Error("empty payload accepted")
	}
}

func TestStatusMapping(t *testing.T) {
	f := newFakeTorBox(t)
	e := newEngine(t, f)
	st, err := e.Status(context.Background(), core.Item{ID: "4321"})
	if err != nil {
		t.Fatal(err)
	}
	if st.State != core.ItemFetching || st.Progress != 0.42 || st.Speed != 12345678 || st.ETA != 120 {
		t.Errorf("status = %+v", st)
	}
	if _, err := e.Status(context.Background(), core.Item{ID: "999"}); err != core.ErrNotFound {
		t.Errorf("missing item: %v", err)
	}
	// Through List the cache serves the finished/failed/queued rows.
	items, err := e.List(context.Background())
	if err != nil || len(items) != 3 {
		t.Fatalf("list = %+v %v", items, err)
	}
	want := map[string]core.ItemState{"4321": core.ItemReady, "5": core.ItemFailed, "6": core.ItemFetching}
	before := len(f.requests)
	for id, state := range want {
		st, err := e.Status(context.Background(), core.Item{ID: id})
		if err != nil || st.State != state {
			t.Errorf("%s: %+v %v", id, st, err)
		}
	}
	if len(f.requests) != before {
		t.Error("Status right after List should be served from the cache")
	}
}

func TestFilesDirectURLAndOpen(t *testing.T) {
	f := newFakeTorBox(t)
	e := newEngine(t, f)
	files, err := e.Files(context.Background(), core.Item{ID: "4321"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "Dune Part Two 2024/dune.mkv" || files[0].Size != 20 || files[1].Index != 1 {
		t.Fatalf("files = %+v", files)
	}
	u, err := files[0].DirectURL(context.Background())
	if err != nil || !strings.HasSuffix(u, "/cdn/11") {
		t.Errorf("direct url = %q %v", u, err)
	}
	var dl *http.Request
	for _, r := range f.requests {
		if strings.HasSuffix(r.URL.Path, "requestdl") {
			dl = r
		}
	}
	q := dl.URL.Query()
	if q.Get("token") != "key" || q.Get("torrent_id") != "4321" || q.Get("file_id") != "11" || q.Get("zip_link") != "false" {
		t.Errorf("requestdl query = %v", q)
	}
	rc, err := files[1].Open(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "456789" {
		t.Errorf("ranged body = %q", b)
	}
	rc, err = files[1].Open(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(rc)
	rc.Close()
	if string(b) != "0123456789" {
		t.Errorf("full body = %q", b)
	}
}

func TestOpenRejects200ForOffset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("full")) }))
	defer srv.Close()
	if _, err := openRange(context.Background(), srv.URL, 2); err == nil || !core.IsRetryable(err) {
		t.Errorf("expected a retryable error, got %v", err)
	}
}

func TestCachedRemovePreview(t *testing.T) {
	f := newFakeTorBox(t)
	e := newEngine(t, f)
	ctx := context.Background()
	c, err := e.Cached(ctx, []string{strings.ToUpper(hash), "ffffffffffffffffffffffffffffffffffffffff"})
	if err != nil || !c[hash] || c["ffffffffffffffffffffffffffffffffffffffff"] {
		t.Errorf("cached = %v %v", c, err)
	}
	c, err = e.Cached(ctx, []string{"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"})
	if err != nil || c["eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"] {
		t.Errorf("data:false should mean nothing cached: %v %v", c, err)
	}
	if c, _ := e.Cached(ctx, nil); len(c) != 0 {
		t.Error("empty input")
	}
	if err := e.Remove(ctx, core.Item{ID: "4321"}, false); err != nil {
		t.Errorf("remove: %v", err)
	}
	if err := e.Remove(ctx, core.Item{ID: "abc"}, false); err == nil {
		t.Error("bad id accepted")
	}
	files, err := e.Preview(ctx, hash)
	if err != nil || len(files) != 2 || files[1].Path != "Dune Part Two 2024/sample.mkv" || files[1].Index != 1 {
		t.Errorf("preview = %+v %v", files, err)
	}
	if !e.Caps().Cached || !e.Caps().DirectLinks || e.Caps().SavePath || e.Caps().LocalFiles || e.Caps().Select {
		t.Errorf("caps = %+v", e.Caps())
	}
}

func TestRetryOn429AndRateLimit(t *testing.T) {
	f := newFakeTorBox(t)
	f.fail429 = 1
	e := newEngine(t, f)
	start := time.Now()
	if _, err := e.List(context.Background()); err != nil {
		t.Fatal("one 429 should be retried:", err)
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Error("expected a backoff before the retry")
	}
	f.fail429 = 2
	e.invalidate()
	if _, err := e.List(context.Background()); err == nil || !core.IsRetryable(err) {
		t.Errorf("two 429s should give up with a retryable error: %v", err)
	}

	// Limiter: 6 calls must take at least 1 s.
	start = time.Now()
	for i := 0; i < 6; i++ {
		e.c.wait(context.Background())
	}
	if time.Since(start) < time.Second {
		t.Error("limiter did not space requests at 5 req/s")
	}
}

func TestBadKey(t *testing.T) {
	f := newFakeTorBox(t)
	e, _ := New("torbox", f.srv.URL+"/v1/api", "nope")
	if _, err := e.List(context.Background()); err == nil || core.IsRetryable(err) {
		t.Errorf("401 must be a hard error: %v", err)
	}
}
