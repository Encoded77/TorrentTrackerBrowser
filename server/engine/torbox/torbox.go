// Package torbox implements core.Engine over the TorBox debrid API.
package torbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

const defaultBase = "https://api.torbox.app/v1/api/"

func init() {
	core.RegisterEngine("torbox", func(cfg map[string]any) (core.Engine, error) {
		return New(core.StringOpt(cfg, "id"), core.StringOpt(cfg, "url"), core.StringOpt(cfg, "apiKey"))
	})
}

// Engine is one TorBox account.
type Engine struct {
	id string
	c  *client

	mu       sync.Mutex
	items    map[string]torrentDTO // by id, refreshed by List
	cachedAt time.Time
}

// New validates the config and returns the engine.
func New(id, base, apiKey string) (*Engine, error) {
	if apiKey == "" {
		return nil, errors.New("missing apiKey")
	}
	if base == "" {
		base = defaultBase
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return &Engine{id: id, c: &client{base: base, apiKey: apiKey, http: &http.Client{Timeout: 60 * time.Second}},
		items: map[string]torrentDTO{}}, nil
}

func (e *Engine) ID() string   { return e.id }
func (e *Engine) Name() string { return "TorBox" }

// Caps: cached lookups and direct links; no selection, no local files.
func (e *Engine) Caps() core.EngineCaps {
	return core.EngineCaps{Cached: true, DirectLinks: true}
}

type fileDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Size      int64  `json:"size"`
	S3Path    string `json:"s3_path"`
}

type torrentDTO struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Hash             string    `json:"hash"`
	Size             int64     `json:"size"`
	DownloadState    string    `json:"download_state"`
	DownloadFinished bool      `json:"download_finished"`
	DownloadPresent  bool      `json:"download_present"`
	Progress         float64   `json:"progress"`
	DownloadSpeed    int64     `json:"download_speed"`
	ETA              int64     `json:"eta"`
	Files            []fileDTO `json:"files"`
}

func (d torrentDTO) item() core.Item {
	return core.Item{ID: strconv.FormatInt(d.ID, 10), Name: d.Name, InfoHash: strings.ToLower(d.Hash), Size: d.Size}
}

func (d torrentDTO) status() core.ItemStatus {
	st := core.ItemStatus{Progress: d.Progress, Speed: d.DownloadSpeed, ETA: d.ETA}
	state := strings.ToLower(d.DownloadState)
	switch {
	case d.DownloadFinished:
		st.State, st.Progress = core.ItemReady, 1
	case strings.Contains(state, "failed"), strings.Contains(state, "error"), strings.Contains(state, "missingfiles"):
		st.State, st.Error = core.ItemFailed, d.DownloadState
	case strings.Contains(state, "metadl"), strings.Contains(state, "checking"), strings.Contains(state, "stalled"),
		strings.Contains(state, "downloading"), strings.Contains(state, "uploading"):
		st.State = core.ItemFetching
	default:
		st.State = core.ItemQueued
	}
	if st.State == core.ItemQueued && d.DownloadState != "" && d.Progress > 0 {
		st.State = core.ItemFetching
	}
	return st
}

// Add submits a magnet or .torrent and returns the TorBox item.
func (e *Engine) Add(ctx context.Context, p core.Payload, o core.AddOpts) (core.Item, error) {
	if p.Magnet == "" && len(p.Torrent) == 0 {
		return core.Item{}, errors.New("payload has neither a magnet nor a torrent")
	}
	mk := func() (io.Reader, string) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if p.Magnet != "" {
			w.WriteField("magnet", p.Magnet)
		} else {
			name := p.Name
			if name == "" {
				name = p.InfoHash
			}
			fw, _ := w.CreateFormFile("file", name+".torrent")
			fw.Write(p.Torrent)
		}
		w.WriteField("seed", "1")
		w.WriteField("allow_zip", "false")
		if p.Name != "" {
			w.WriteField("name", p.Name)
		}
		w.Close()
		return &buf, w.FormDataContentType()
	}
	data, err := e.c.do(ctx, http.MethodPost, "torrents/createtorrent", mk)
	if err != nil {
		return core.Item{}, err
	}
	var out struct {
		TorrentID *int64 `json:"torrent_id"`
		QueuedID  *int64 `json:"queued_id"`
		Hash      string `json:"hash"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return core.Item{}, fmt.Errorf("torbox createtorrent: %w", err)
	}
	hash := strings.ToLower(out.Hash)
	if hash == "" {
		hash = p.InfoHash
	}
	name := p.Name
	if name == "" {
		name = out.Name
	}
	e.invalidate()
	if out.TorrentID != nil && *out.TorrentID > 0 {
		return core.Item{ID: strconv.FormatInt(*out.TorrentID, 10), Name: name, InfoHash: hash, Size: p.Size}, nil
	}
	if hash == "" {
		return core.Item{}, errors.New("torbox createtorrent: no torrent_id in response")
	}
	// Account queue is full: TorBox holds the torrent until a slot frees up.
	return core.Item{ID: "queued:" + hash, Name: name, InfoHash: hash, Size: p.Size}, nil
}

func (e *Engine) invalidate() {
	e.mu.Lock()
	e.cachedAt = time.Time{}
	e.mu.Unlock()
}

// record returns the TorBox row of an item, from the 5 s List cache when
// fresh, else from mylist. Missing items yield core.ErrNotFound.
func (e *Engine) record(ctx context.Context, it core.Item) (torrentDTO, error) {
	e.mu.Lock()
	d, ok := e.items[it.ID]
	fresh := time.Since(e.cachedAt) < 5*time.Second
	e.mu.Unlock()
	if ok && fresh {
		return d, nil
	}
	if hash, queued := strings.CutPrefix(it.ID, "queued:"); queued {
		all, err := e.list(ctx)
		if err != nil {
			return torrentDTO{}, err
		}
		for _, d := range all {
			if strings.EqualFold(d.Hash, hash) {
				return d, nil
			}
		}
		return torrentDTO{}, errQueued
	}
	var raw json.RawMessage
	err := e.c.get(ctx, "torrents/mylist?bypass_cache=true&id="+url.QueryEscape(it.ID), &raw)
	if err != nil {
		if core.IsRetryable(err) {
			return torrentDTO{}, err
		}
		return torrentDTO{}, core.ErrNotFound
	}
	if len(raw) == 0 {
		return torrentDTO{}, core.ErrNotFound
	}
	if raw[0] == '[' {
		var list []torrentDTO
		if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
			return torrentDTO{}, core.ErrNotFound
		}
		d = list[0]
	} else if err := json.Unmarshal(raw, &d); err != nil || d.ID == 0 {
		return torrentDTO{}, core.ErrNotFound
	}
	e.mu.Lock()
	e.items[it.ID] = d
	e.mu.Unlock()
	return d, nil
}

var errQueued = errors.New("torbox: torrent is waiting in the account queue")

// Status maps the TorBox download state.
func (e *Engine) Status(ctx context.Context, it core.Item) (core.ItemStatus, error) {
	d, err := e.record(ctx, it)
	if errors.Is(err, errQueued) {
		return core.ItemStatus{State: core.ItemQueued}, nil
	}
	if err != nil {
		return core.ItemStatus{}, err
	}
	return d.status(), nil
}

// Select is not supported: TorBox fetches every file.
func (e *Engine) Select(ctx context.Context, it core.Item, files []int) error { return nil }

// Files lists the item's files with lazy direct links and Range-capable streams.
func (e *Engine) Files(ctx context.Context, it core.Item) ([]core.File, error) {
	d, err := e.record(ctx, it)
	if errors.Is(err, errQueued) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]core.File, 0, len(d.Files))
	for i, f := range d.Files {
		torrentID, fileID := d.ID, f.ID
		name := f.Name
		if name == "" {
			name = f.ShortName
		}
		direct := func(ctx context.Context) (string, error) { return e.requestDL(ctx, torrentID, fileID) }
		out = append(out, core.File{
			Index: i, Path: name, Size: f.Size,
			DirectURL: direct,
			Open: func(ctx context.Context, offset int64) (io.ReadCloser, error) {
				u, err := direct(ctx)
				if err != nil {
					return nil, err
				}
				return openRange(ctx, u, offset)
			},
		})
	}
	return out, nil
}

// requestDL resolves a fresh direct URL (they expire, so never cached).
func (e *Engine) requestDL(ctx context.Context, torrentID, fileID int64) (string, error) {
	v := url.Values{}
	v.Set("token", e.c.apiKey)
	v.Set("torrent_id", strconv.FormatInt(torrentID, 10))
	v.Set("file_id", strconv.FormatInt(fileID, 10))
	v.Set("zip_link", "false")
	v.Set("redirect", "false")
	var link string
	if err := e.c.get(ctx, "torrents/requestdl?"+v.Encode(), &link); err != nil {
		return "", err
	}
	if link == "" {
		return "", core.Retryable(errors.New("torbox requestdl: empty link"))
	}
	return link, nil
}

// openRange GETs a direct link from offset; a 200 answer to a ranged request
// is an error because the stream would restart from zero.
func openRange(ctx context.Context, u string, offset int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, core.Retryable(err)
	}
	switch {
	case offset > 0 && resp.StatusCode == http.StatusPartialContent:
	case offset == 0 && resp.StatusCode == http.StatusOK:
	default:
		resp.Body.Close()
		return nil, core.Retryable(fmt.Errorf("torbox download: HTTP %d for offset %d", resp.StatusCode, offset))
	}
	return resp.Body, nil
}

func (e *Engine) list(ctx context.Context) ([]torrentDTO, error) {
	var all []torrentDTO
	if err := e.c.get(ctx, "torrents/mylist?bypass_cache=true", &all); err != nil {
		return nil, err
	}
	e.mu.Lock()
	e.items = map[string]torrentDTO{}
	for _, d := range all {
		e.items[strconv.FormatInt(d.ID, 10)] = d
	}
	e.cachedAt = time.Now()
	e.mu.Unlock()
	return all, nil
}

// List returns every torrent of the account.
func (e *Engine) List(ctx context.Context) ([]core.Item, error) {
	all, err := e.list(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.Item, 0, len(all))
	for _, d := range all {
		out = append(out, d.item())
	}
	return out, nil
}

// Remove deletes the torrent from the account (deleteFiles is implied).
func (e *Engine) Remove(ctx context.Context, it core.Item, deleteFiles bool) error {
	id := it.ID
	if strings.HasPrefix(it.ID, "queued:") {
		d, err := e.record(ctx, it)
		if errors.Is(err, errQueued) {
			return nil
		}
		if err != nil {
			return err
		}
		id = strconv.FormatInt(d.ID, 10)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("torbox: bad item id %q", it.ID)
	}
	defer e.invalidate()
	err = e.c.postJSON(ctx, "torrents/controltorrent", map[string]any{"torrent_id": n, "operation": "delete"}, nil)
	if err != nil && !core.IsRetryable(err) && strings.Contains(strings.ToLower(err.Error()), "not found") {
		return core.ErrNotFound
	}
	return err
}

// Cached reports which hashes TorBox already holds.
func (e *Engine) Cached(ctx context.Context, hashes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(hashes) == 0 {
		return out, nil
	}
	lower := make([]string, 0, len(hashes))
	for _, h := range hashes {
		lower = append(lower, strings.ToLower(h))
		out[strings.ToLower(h)] = false
	}
	v := url.Values{}
	v.Set("hash", strings.Join(lower, ","))
	v.Set("format", "object")
	v.Set("list_files", "false")
	var data map[string]json.RawMessage
	if err := e.c.get(ctx, "torrents/checkcached?"+v.Encode(), &data); err != nil {
		var syn *json.UnmarshalTypeError
		if errors.As(err, &syn) {
			return out, nil // data was false/[]: nothing cached
		}
		return nil, err
	}
	for k := range data {
		out[strings.ToLower(k)] = true
	}
	return out, nil
}

// Preview lists the files of a hash TorBox knows without adding it.
func (e *Engine) Preview(ctx context.Context, infoHash string) ([]core.FileEntry, error) {
	var info struct {
		Name  string `json:"name"`
		Files []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := e.c.get(ctx, "torrents/torrentinfo?hash="+url.QueryEscape(infoHash), &info); err != nil {
		return nil, err
	}
	out := make([]core.FileEntry, 0, len(info.Files))
	for i, f := range info.Files {
		out = append(out, core.FileEntry{Index: i, Path: f.Name, Size: f.Size})
	}
	return out, nil
}
