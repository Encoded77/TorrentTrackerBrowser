package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// fakeSource answers one indexer with one result.
type fakeSource struct{}

func (fakeSource) ID() string   { return "p" }
func (fakeSource) Name() string { return "P" }
func (fakeSource) Indexers(context.Context) ([]core.Indexer, error) {
	return []core.Indexer{{ID: "p:1", Name: "One", Private: true}}, nil
}
func (fakeSource) Search(ctx context.Context, q core.Query, emit func(core.SearchEvent)) error {
	emit(core.SearchEvent{Indexer: core.Indexer{ID: "p:1"}, Done: true, Elapsed: time.Millisecond,
		Results: []core.Result{{Title: "Dune MULTI " + q.Text, Size: 10, InfoHash: strings.Repeat("a", 40), HasMagnet: true, Ref: map[string]string{"magnet": "magnet:?xt=urn:btih:" + strings.Repeat("a", 40)}}}})
	return nil
}
func (fakeSource) Fetch(ctx context.Context, r core.Result) (core.Payload, error) {
	return core.PayloadFromMagnet(r.Ref["magnet"])
}

func newServer(t *testing.T, cfg *core.Config) http.Handler {
	t.Helper()
	if cfg == nil {
		cfg = &core.Config{}
	}
	cfg.Limits = core.Limits{Jobs: 1, FilesPerJob: 1, ResultsPerIndexer: 10, IndexerTimeout: time.Second, ResultTTL: time.Minute, JobHistory: 10}
	reg := core.NewRegistry()
	reg.AddSource(fakeSource{})
	results := core.NewResultCache(time.Minute, 100)
	store := core.NewJobStore(filepath.Join(t.TempDir(), "jobs.json"), 10)
	runner := core.NewRunner(reg, store, core.NewWebhook(""), cfg.Limits)
	runner.Start(context.Background())
	t.Cleanup(runner.Stop)
	s := &Server{Config: cfg, Reg: reg, Results: results, Payloads: core.NewPayloadCache(time.Minute, 10),
		Searcher: &core.Searcher{Sources: reg.Sources, Results: results, IndexerTimeout: time.Second, PerIndexer: 10}, Runner: runner}
	return s.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ui")) }))
}

func do(h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var mutating = map[string]string{"Content-Type": "application/json", "X-Requested-With": "TorrentTrackerBrowser"}

func TestHealthAndUIFallback(t *testing.T) {
	h := newServer(t, nil)
	if rec := do(h, "GET", "/healthz", "", nil); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Errorf("healthz = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "GET", "/some/spa/route", "", nil); rec.Code != 200 || rec.Body.String() != "ui" {
		t.Errorf("ui fallback = %d %s", rec.Code, rec.Body.String())
	}
	rec := do(h, "GET", "/api/nope", "", nil)
	var e apiError
	json.Unmarshal(rec.Body.Bytes(), &e)
	if rec.Code != 404 || e.Error.Code != "not_found" {
		t.Errorf("unknown api route = %d %s", rec.Code, rec.Body.String())
	}
}

func TestMutationGuard(t *testing.T) {
	h := newServer(t, nil)
	if rec := do(h, "POST", "/api/payloads", `{"magnet":"x"}`, map[string]string{"Content-Type": "application/json"}); rec.Code != 403 {
		t.Errorf("missing header: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/payloads", `magnet=x`, map[string]string{"Content-Type": "application/x-www-form-urlencoded", "X-Requested-With": "TorrentTrackerBrowser"}); rec.Code != 403 {
		t.Errorf("form content type: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/jobs/j_x", "", map[string]string{"X-Requested-With": "TorrentTrackerBrowser"}); rec.Code != 403 {
		t.Errorf("delete without content type: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/jobs/j_x", "", mutating); rec.Code != 404 {
		t.Errorf("delete unknown job: %d", rec.Code)
	}
}

func TestHostAllowlistAndToken(t *testing.T) {
	h := newServer(t, &core.Config{AllowedHosts: []string{"ttb.lan"}, Token: "s3cret"})
	if rec := do(h, "GET", "/healthz", "", nil); rec.Code != 200 {
		t.Errorf("healthz must bypass the host allowlist: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/capabilities", "", nil); rec.Code != 403 {
		t.Errorf("default host should be rejected: %d", rec.Code)
	}
	req := httptest.NewRequest("GET", "/api/capabilities", nil)
	req.Host = "ttb.lan:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("no token: %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/api/capabilities?token=s3cret", nil)
	req.Host = "ttb.lan"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("query token: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest("GET", "/api/capabilities", nil)
	req.Host = "ttb.lan"
	req.Header.Set("Authorization", "Bearer s3cret")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("bearer token: %d", rec.Code)
	}
}

func TestCapabilities(t *testing.T) {
	h := newServer(t, &core.Config{Defaults: core.Defaults{Mode: core.ModeCopy}})
	rec := do(h, "GET", "/api/capabilities", "", nil)
	var v struct {
		Sources []struct {
			ID       string
			Indexers []core.Indexer
		}
		Engines    []any
		Storages   []any
		Categories []string
		Defaults   map[string]string
		Languages  []string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if len(v.Sources) != 1 || v.Sources[0].Indexers[0].ID != "p:1" || len(v.Engines) != 0 || len(v.Categories) != 8 || len(v.Languages) != 4 {
		t.Errorf("capabilities = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"engines":[]`) || !strings.Contains(rec.Body.String(), `"storages":[]`) {
		t.Errorf("empty lists must be [] not null: %s", rec.Body.String())
	}
}

func TestSearchSSE(t *testing.T) {
	h := newServer(t, nil)
	if rec := do(h, "GET", "/api/search", "", nil); rec.Code != 400 {
		t.Errorf("missing q: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/search?q=x&cat=nope", "", nil); rec.Code != 400 {
		t.Errorf("bad cat: %d", rec.Code)
	}
	rec := do(h, "GET", "/api/search?q=dune&cat=movies,tv&indexers=p:1", "", nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("sse = %d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: 1\nevent: batch\ndata: ") || !strings.Contains(body, "id: 2\nevent: done\ndata: ") {
		t.Fatalf("events:\n%s", body)
	}
	line := body[strings.Index(body, "data: ")+6:]
	line = line[:strings.Index(line, "\n")]
	var b batchEvent
	if err := json.Unmarshal([]byte(line), &b); err != nil {
		t.Fatal(err)
	}
	if b.Indexer != "p:1" || !b.Done || b.Error != nil || len(b.Results) != 1 {
		t.Errorf("batch = %+v", b)
	}
	r := b.Results[0]
	if !strings.HasPrefix(r.ID, "r_") || r.Language != core.LangMulti || r.Title != "Dune MULTI dune" || !r.HasMagnet {
		t.Errorf("result = %+v", r)
	}
	if !strings.Contains(line, `"categories":[]`) || !strings.Contains(line, `"error":null`) {
		t.Errorf("json shape: %s", line)
	}
	if !strings.Contains(body, `"total":1`) {
		t.Errorf("done event: %s", body)
	}

	// The result id now resolves to its payload.
	rec = do(h, "GET", "/api/results/"+r.ID+"/payload?as=magnet", "", nil)
	if rec.Code != 200 || !strings.HasPrefix(rec.Body.String(), "magnet:?") {
		t.Errorf("payload magnet = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "GET", "/api/results/"+r.ID+"/payload?as=torrent", "", nil); rec.Code != 404 {
		t.Errorf("payload torrent = %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/results/"+r.ID+"/files", "", nil); rec.Code != 409 {
		t.Errorf("no preview = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "GET", "/api/results/r_unknown/files", "", nil); rec.Code != 404 {
		t.Errorf("unknown result = %d", rec.Code)
	}
	// No engine configured: job creation is refused cleanly.
	rec = do(h, "POST", "/api/jobs", `{"resultId":"`+r.ID+`","engine":"torbox","mode":"copy"}`, mutating)
	if rec.Code != 400 {
		t.Errorf("job without engine = %d %s", rec.Code, rec.Body.String())
	}
}

func TestPayloads(t *testing.T) {
	h := newServer(t, nil)
	rec := do(h, "POST", "/api/payloads", `{"magnet":"magnet:?xt=urn:btih:`+strings.Repeat("ab", 20)+`&dn=Dune"}`, mutating)
	var v struct {
		ID, Name, InfoHash string
		Size               *int64
		Files              []core.FileEntry
	}
	json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != 201 || !strings.HasPrefix(v.ID, "p_") || v.Name != "Dune" || v.InfoHash != strings.Repeat("ab", 20) || v.Size != nil {
		t.Errorf("magnet payload = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"size":null`) || !strings.Contains(rec.Body.String(), `"files":null`) {
		t.Errorf("nulls: %s", rec.Body.String())
	}
	if rec := do(h, "POST", "/api/payloads", `{"magnet":"nope"}`, mutating); rec.Code != 400 {
		t.Errorf("bad magnet = %d", rec.Code)
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("torrent", "x.torrent")
	io.WriteString(fw, "d4:infod6:lengthi7e4:name5:x.mkv12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaaee")
	mw.Close()
	rec = do(h, "POST", "/api/payloads", buf.String(), map[string]string{"Content-Type": mw.FormDataContentType(), "X-Requested-With": "TorrentTrackerBrowser"})
	json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != 201 || v.Name != "x.mkv" || v.Size == nil || *v.Size != 7 || len(v.Files) != 1 || v.Files[0].Path != "x.mkv" {
		t.Errorf("torrent payload = %d %s", rec.Code, rec.Body.String())
	}
	rec = do(h, "POST", "/api/jobs", `{"payloadId":"`+v.ID+`","engine":"","mode":"links"}`, mutating)
	if rec.Code != 400 {
		t.Errorf("no engine configured: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(h, "POST", "/api/jobs", `{"resultId":"r","payloadId":"p"}`, mutating)
	if rec.Code != 400 {
		t.Errorf("both ids: %d", rec.Code)
	}
}

func TestJobsListAndCached(t *testing.T) {
	h := newServer(t, nil)
	rec := do(h, "GET", "/api/jobs", "", nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"jobs":[]}` {
		t.Errorf("jobs = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(h, "POST", "/api/cached", `{"engine":"nope","hashes":["a"]}`, mutating); rec.Code != 400 {
		t.Errorf("cached unknown engine = %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/jobs/j_x/cancel", `{}`, mutating); rec.Code != 404 {
		t.Errorf("cancel unknown = %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/jobs/j_x/retry", `{}`, mutating); rec.Code != 404 {
		t.Errorf("retry unknown = %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/jobs/j_x/send", `{"storage":"s"}`, mutating); rec.Code != 400 {
		t.Errorf("send on own id = %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/jobs/j_x/files/a/b.mkv", "", nil); rec.Code != 404 {
		t.Errorf("file of unknown job = %d", rec.Code)
	}
}

func TestStreamRange(t *testing.T) {
	content := "0123456789"
	f := core.File{Path: "d/x.bin", Size: 10, Open: func(ctx context.Context, off int64) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(content[off:])), nil
	}}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Range", "bytes=4-6")
	rec := httptest.NewRecorder()
	streamRange(rec, req, f)
	if rec.Code != 206 || rec.Body.String() != "456" || rec.Header().Get("Content-Range") != "bytes 4-6/10" || rec.Header().Get("Content-Length") != "3" {
		t.Errorf("range = %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	req.Header.Set("Range", "bytes=7-")
	rec = httptest.NewRecorder()
	streamRange(rec, req, f)
	if rec.Code != 206 || rec.Body.String() != "789" {
		t.Errorf("open range = %d %q", rec.Code, rec.Body.String())
	}
	req.Header.Del("Range")
	rec = httptest.NewRecorder()
	streamRange(rec, req, f)
	if rec.Code != 200 || rec.Body.String() != content || rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("full = %d %q", rec.Code, rec.Body.String())
	}
	req.Header.Set("Range", "bytes=50-")
	rec = httptest.NewRecorder()
	streamRange(rec, req, f)
	if rec.Code != 416 {
		t.Errorf("out of range = %d", rec.Code)
	}
}
