package qbittorrent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

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

type fakeQbit struct {
	srv       *httptest.Server
	mu        sync.Mutex
	logins    int
	adds      []map[string]string
	posts     map[string][]string // path -> raw bodies
	done      bool
	expireSID bool // next authenticated call answers 403 once
	auth      bool // require the SID cookie
}

func newFakeQbit(t *testing.T, auth bool) *fakeQbit {
	f := &fakeQbit{posts: map[string][]string{}, auth: auth}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		f.logins++
		f.mu.Unlock()
		if r.PostForm.Get("username") != "admin" || r.PostForm.Get("password") != "pw" {
			w.Write([]byte("Fails."))
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s3cret", Path: "/"})
		w.Write([]byte("Ok."))
	})
	mux.HandleFunc("/api/v2/", func(w http.ResponseWriter, r *http.Request) {
		if f.auth {
			c, err := r.Cookie("SID")
			f.mu.Lock()
			expire := f.expireSID
			f.expireSID = false
			f.mu.Unlock()
			if err != nil || c.Value != "s3cret" || expire {
				w.WriteHeader(403)
				w.Write([]byte("Forbidden"))
				return
			}
		}
		switch r.URL.Path {
		case "/api/v2/torrents/add":
			r.ParseMultipartForm(1 << 20)
			form := map[string]string{}
			for k, v := range r.MultipartForm.Value {
				form[k] = v[0]
			}
			if fh := r.MultipartForm.File["torrents"]; len(fh) > 0 {
				fp, _ := fh[0].Open()
				b, _ := io.ReadAll(fp)
				form["torrents"] = string(b)
			}
			f.mu.Lock()
			f.adds = append(f.adds, form)
			f.mu.Unlock()
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			f.mu.Lock()
			done := f.done
			f.mu.Unlock()
			q := r.URL.Query()
			if q.Get("hashes") == "deadbeef" {
				w.Write([]byte("[]"))
				return
			}
			name := "info.json"
			if done || q.Get("category") != "" {
				name = "info_done.json"
			}
			var rows []map[string]any
			json.Unmarshal(fixture(t, name), &rows)
			if h := q.Get("hashes"); h != "" {
				var kept []map[string]any
				for _, row := range rows {
					if row["hash"] == h {
						kept = append(kept, row)
					}
				}
				rows = kept
			}
			json.NewEncoder(w).Encode(rows)
		case "/api/v2/torrents/files":
			w.Write(fixture(t, "files.json"))
		case "/api/v2/torrents/filePrio", "/api/v2/torrents/delete":
			b, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.posts[r.URL.Path] = append(f.posts[r.URL.Path], string(b))
			f.mu.Unlock()
			w.Write([]byte(""))
		default:
			w.WriteHeader(404)
		}
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newEngine(t *testing.T, f *fakeQbit, user string, pathMap map[string]string) *Engine {
	e, err := New("qbit", f.srv.URL, user, "pw", pathMap, "")
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAddWithLoginAndPathMap(t *testing.T) {
	f := newFakeQbit(t, true)
	e := newEngine(t, f, "admin", map[string]string{"/mnt/media": "/data"})
	it, err := e.Add(context.Background(), core.Payload{Name: "Dune", Magnet: "magnet:?xt=urn:btih:" + hash, InfoHash: hash},
		core.AddOpts{SavePath: "/mnt/media/downloads/manual/Dune", Files: []int{0}})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != hash || it.Name != "Dune" {
		t.Errorf("item = %+v", it)
	}
	if f.logins != 1 {
		t.Errorf("logins = %d", f.logins)
	}
	form := f.adds[0]
	if form["urls"] == "" || form["savepath"] != "/data/downloads/manual/Dune" || form["category"] != "ttb" || form["paused"] != "false" {
		t.Errorf("add form = %v", form)
	}
	prio := f.posts["/api/v2/torrents/filePrio"]
	if len(prio) != 1 || !strings.Contains(prio[0], "priority=0") || !strings.Contains(prio[0], "id=1") {
		t.Errorf("filePrio = %v", prio)
	}
}

func TestAddTorrentBytesComputesHash(t *testing.T) {
	f := newFakeQbit(t, false)
	e := newEngine(t, f, "", nil)
	raw := "d4:infod6:lengthi5e4:name5:a.txt12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee"
	ti, _ := core.ParseTorrent([]byte(raw))
	it, err := e.Add(context.Background(), core.Payload{Torrent: []byte(raw)}, core.AddOpts{SavePath: "/anywhere"})
	if err != nil {
		t.Fatal(err)
	}
	if it.ID != ti.InfoHash {
		t.Errorf("hash = %s, want %s", it.ID, ti.InfoHash)
	}
	if f.logins != 0 {
		t.Error("empty username must skip login")
	}
	if f.adds[0]["torrents"] != raw || f.adds[0]["savepath"] != "/anywhere" {
		t.Errorf("form = %v", f.adds[0])
	}
}

func TestAddUnreachableStorage(t *testing.T) {
	f := newFakeQbit(t, false)
	e := newEngine(t, f, "", map[string]string{"/mnt/media": "/data"})
	_, err := e.Add(context.Background(), core.Payload{Magnet: "magnet:?xt=urn:btih:" + hash, InfoHash: hash}, core.AddOpts{SavePath: "/srv/other"})
	if err == nil || !strings.Contains(err.Error(), "storage not reachable by this engine") {
		t.Errorf("err = %v", err)
	}
	if len(f.adds) != 0 {
		t.Error("nothing must be sent for an unmapped path")
	}
}

func TestStatusFilesListRemove(t *testing.T) {
	f := newFakeQbit(t, false)
	e := newEngine(t, f, "", map[string]string{"/mnt/media": "/data"})
	ctx := context.Background()
	st, err := e.Status(ctx, core.Item{ID: hash})
	if err != nil || st.State != core.ItemFetching || st.Progress != 0.42 || st.Speed != 5000000 || st.ETA != 300 {
		t.Errorf("status = %+v %v", st, err)
	}
	if _, err := e.Status(ctx, core.Item{ID: "deadbeef"}); err != core.ErrNotFound {
		t.Errorf("missing: %v", err)
	}
	f.done = true
	st, _ = e.Status(ctx, core.Item{ID: hash})
	if st.State != core.ItemReady || st.Progress != 1 || st.ETA != 0 {
		t.Errorf("done status = %+v", st)
	}
	files, err := e.Files(ctx, core.Item{ID: hash})
	if err != nil || len(files) != 2 {
		t.Fatalf("files = %+v %v", files, err)
	}
	if files[0].Path != "Dune Part Two 2024/dune.mkv" || files[0].LocalPath != "/mnt/media/downloads/manual/Dune/Dune Part Two 2024/dune.mkv" || files[1].Index != 1 {
		t.Errorf("files = %+v", files)
	}
	items, err := e.List(ctx)
	if err != nil || len(items) != 2 || items[0].ID != hash || items[1].Name != "Broken" {
		t.Errorf("list = %+v %v", items, err)
	}
	if st, _ := e.Status(ctx, core.Item{ID: "ffffffffffffffffffffffffffffffffffffffff"}); st.State != core.ItemFailed {
		t.Errorf("missingFiles should map to failed: %+v", st)
	}
	if err := e.Remove(ctx, core.Item{ID: hash}, true); err != nil {
		t.Fatal(err)
	}
	if del := f.posts["/api/v2/torrents/delete"]; len(del) != 1 || !strings.Contains(del[0], "deleteFiles=true") || !strings.Contains(del[0], "hashes="+hash) {
		t.Errorf("delete = %v", del)
	}
	if _, err := e.Cached(ctx, []string{hash}); err != core.ErrUnsupported {
		t.Errorf("cached: %v", err)
	}
	if c := e.Caps(); !c.SavePath || !c.LocalFiles || c.Cached || c.DirectLinks {
		t.Errorf("caps = %+v", c)
	}
}

func TestReloginOn403(t *testing.T) {
	f := newFakeQbit(t, true)
	e := newEngine(t, f, "admin", nil)
	if _, err := e.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.expireSID = true
	if _, err := e.List(context.Background()); err != nil {
		t.Fatal("expected a transparent re-login:", err)
	}
	if f.logins != 2 {
		t.Errorf("logins = %d, want 2", f.logins)
	}
}

func TestBadCredentials(t *testing.T) {
	f := newFakeQbit(t, true)
	e, _ := New("qbit", f.srv.URL, "admin", "wrong", nil, "")
	if _, err := e.List(context.Background()); err == nil {
		t.Error("expected a login failure")
	}
}

func TestReplacePrefix(t *testing.T) {
	m := map[string]string{"/mnt/media": "/data", "/mnt/media/other": "/other"}
	cases := map[string]string{
		"/mnt/media/downloads/x": "/data/downloads/x",
		"/mnt/media":             "/data",
		"/mnt/media/other/y":     "/other/y",
		"/mnt/mediax/y":          "",
	}
	for in, want := range cases {
		got, ok := replacePrefix(in, m)
		if (want == "") == ok || got != want {
			t.Errorf("%s: got %q %v, want %q", in, got, ok, want)
		}
	}
}
