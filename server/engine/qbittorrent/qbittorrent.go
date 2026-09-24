// Package qbittorrent implements core.Engine over the qBittorrent WebUI API v2.
package qbittorrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

func init() {
	core.RegisterEngine("qbittorrent", func(cfg map[string]any) (core.Engine, error) {
		return New(core.StringOpt(cfg, "id"), core.StringOpt(cfg, "url"), core.StringOpt(cfg, "username"),
			core.StringOpt(cfg, "password"), core.StringMapOpt(cfg, "pathMap"), core.StringOpt(cfg, "category"))
	})
}

// Engine is one qBittorrent instance. pathMap translates app-side storage
// roots to client-side paths (and back for LocalPath).
type Engine struct {
	id       string
	c        *client
	category string
	pathMap  map[string]string // app prefix -> client prefix
}

// New validates the config and returns the engine.
func New(id, base, username, password string, pathMap map[string]string, category string) (*Engine, error) {
	if base == "" {
		return nil, errors.New("missing url")
	}
	if category == "" {
		category = "ttb"
	}
	c, err := newClient(base, username, password)
	if err != nil {
		return nil, err
	}
	return &Engine{id: id, c: c, category: category, pathMap: pathMap}, nil
}

func (e *Engine) ID() string   { return e.id }
func (e *Engine) Name() string { return "qBittorrent" }

// Caps: honours a save path and exposes local files; no cache, no links.
func (e *Engine) Caps() core.EngineCaps {
	return core.EngineCaps{SavePath: true, LocalFiles: true}
}

// toClient maps an app-side path to the client's view. Without a pathMap
// both sides share the same filesystem view.
func (e *Engine) toClient(p string) (string, error) {
	if len(e.pathMap) == 0 {
		return p, nil
	}
	if mapped, ok := replacePrefix(p, e.pathMap); ok {
		return mapped, nil
	}
	return "", fmt.Errorf("storage not reachable by this engine: %q matches no pathMap prefix", p)
}

// toApp maps a client-side path back to the app's view (identity when the
// path matches no mapping).
func (e *Engine) toApp(p string) string {
	if len(e.pathMap) == 0 {
		return p
	}
	reverse := map[string]string{}
	for app, cl := range e.pathMap {
		reverse[cl] = app
	}
	if mapped, ok := replacePrefix(p, reverse); ok {
		return mapped
	}
	return p
}

// replacePrefix swaps the longest matching prefix (on a path boundary).
func replacePrefix(p string, m map[string]string) (string, bool) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	norm := strings.ReplaceAll(p, "\\", "/")
	for _, k := range keys {
		nk := strings.TrimRight(strings.ReplaceAll(k, "\\", "/"), "/")
		if norm == nk || strings.HasPrefix(norm, nk+"/") {
			return strings.TrimRight(m[k], "/") + norm[len(nk):], true
		}
	}
	return "", false
}

// Add submits the payload, paused=false, into the configured category.
func (e *Engine) Add(ctx context.Context, p core.Payload, o core.AddOpts) (core.Item, error) {
	hash := p.InfoHash
	if hash == "" && len(p.Torrent) > 0 {
		t, err := core.ParseTorrent(p.Torrent)
		if err != nil {
			return core.Item{}, err
		}
		hash = t.InfoHash
	}
	if hash == "" && p.Magnet != "" {
		m, err := core.ParseMagnet(p.Magnet)
		if err != nil {
			return core.Item{}, err
		}
		hash = m.InfoHash
	}
	if hash == "" {
		return core.Item{}, errors.New("payload has no info hash")
	}
	savePath := ""
	if o.SavePath != "" {
		mapped, err := e.toClient(o.SavePath)
		if err != nil {
			return core.Item{}, err
		}
		savePath = mapped
	}
	mk := func() (io.Reader, string) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if p.Magnet != "" {
			w.WriteField("urls", p.Magnet)
		} else {
			fw, _ := w.CreateFormFile("torrents", hash+".torrent")
			fw.Write(p.Torrent)
		}
		if savePath != "" {
			w.WriteField("savepath", savePath)
		}
		w.WriteField("category", e.category)
		w.WriteField("paused", "false")
		w.WriteField("stopped", "false")
		w.Close()
		return &buf, w.FormDataContentType()
	}
	raw, err := e.c.do(ctx, http.MethodPost, "/api/v2/torrents/add", mk)
	if err != nil {
		return core.Item{}, err
	}
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "Fails") {
		return core.Item{}, errors.New("qbittorrent refused the torrent (already present or invalid)")
	}
	it := core.Item{ID: hash, Name: p.Name, InfoHash: hash, Size: p.Size}
	if o.Files != nil {
		if err := e.Select(ctx, it, o.Files); err != nil {
			return it, nil // metadata not there yet (magnet): fetch everything
		}
	}
	return it, nil
}

type infoDTO struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	DLSpeed     int64   `json:"dlspeed"`
	ETA         int64   `json:"eta"`
	Size        int64   `json:"size"`
	TotalSize   int64   `json:"total_size"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
}

func (d infoDTO) item() core.Item {
	size := d.TotalSize
	if size == 0 {
		size = d.Size
	}
	return core.Item{ID: strings.ToLower(d.Hash), Name: d.Name, InfoHash: strings.ToLower(d.Hash), Size: size}
}

func (d infoDTO) status() core.ItemStatus {
	st := core.ItemStatus{Progress: d.Progress, Speed: d.DLSpeed, ETA: d.ETA}
	if d.ETA >= 8640000 {
		st.ETA = 0
	}
	switch d.State {
	case "uploading", "stalledUP", "pausedUP", "stoppedUP", "queuedUP", "forcedUP", "completed", "checkingUP":
		st.State, st.Progress = core.ItemReady, 1
	case "error", "missingFiles":
		st.State, st.Error = core.ItemFailed, "qbittorrent state "+d.State
	default:
		st.State = core.ItemFetching
	}
	return st
}

func (e *Engine) info(ctx context.Context, hash string) (infoDTO, error) {
	var list []infoDTO
	if err := e.c.getJSON(ctx, "/api/v2/torrents/info", url.Values{"hashes": {hash}}, &list); err != nil {
		return infoDTO{}, err
	}
	if len(list) == 0 {
		return infoDTO{}, core.ErrNotFound
	}
	return list[0], nil
}

// Status maps the qBittorrent state.
func (e *Engine) Status(ctx context.Context, it core.Item) (core.ItemStatus, error) {
	d, err := e.info(ctx, it.ID)
	if err != nil {
		return core.ItemStatus{}, err
	}
	return d.status(), nil
}

// Select sets priority 0 on every file not in files.
func (e *Engine) Select(ctx context.Context, it core.Item, files []int) error {
	if files == nil {
		return nil
	}
	var all []fileDTO
	if err := e.c.getJSON(ctx, "/api/v2/torrents/files", url.Values{"hash": {it.ID}}, &all); err != nil {
		return err
	}
	if len(all) == 0 {
		return errors.New("no file list yet")
	}
	keep := map[int]bool{}
	for _, i := range files {
		keep[i] = true
	}
	var skip []string
	for _, f := range all {
		if !keep[f.Index] {
			skip = append(skip, strconv.Itoa(f.Index))
		}
	}
	if len(skip) == 0 {
		return nil
	}
	_, err := e.c.postForm(ctx, "/api/v2/torrents/filePrio", url.Values{"hash": {it.ID}, "id": {strings.Join(skip, "|")}, "priority": {"0"}})
	return err
}

type fileDTO struct {
	Index    int     `json:"index"`
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress"`
	Priority int     `json:"priority"`
}

// Files lists the torrent's files with their app-side local path.
func (e *Engine) Files(ctx context.Context, it core.Item) ([]core.File, error) {
	d, err := e.info(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	var all []fileDTO
	if err := e.c.getJSON(ctx, "/api/v2/torrents/files", url.Values{"hash": {it.ID}}, &all); err != nil {
		return nil, err
	}
	out := make([]core.File, 0, len(all))
	for _, f := range all {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		local := e.toApp(path.Join(strings.ReplaceAll(d.SavePath, "\\", "/"), name))
		out = append(out, core.File{Index: f.Index, Path: name, Size: f.Size, LocalPath: local})
	}
	return out, nil
}

// List returns the torrents in the app's category.
func (e *Engine) List(ctx context.Context) ([]core.Item, error) {
	var list []infoDTO
	if err := e.c.getJSON(ctx, "/api/v2/torrents/info", url.Values{"category": {e.category}}, &list); err != nil {
		return nil, err
	}
	out := make([]core.Item, 0, len(list))
	for _, d := range list {
		out = append(out, d.item())
	}
	return out, nil
}

// Remove deletes the torrent, and its files only when asked.
func (e *Engine) Remove(ctx context.Context, it core.Item, deleteFiles bool) error {
	_, err := e.c.postForm(ctx, "/api/v2/torrents/delete", url.Values{"hashes": {it.ID}, "deleteFiles": {strconv.FormatBool(deleteFiles)}})
	return err
}

// Cached is not supported by a torrent client.
func (e *Engine) Cached(ctx context.Context, hashes []string) (map[string]bool, error) {
	return nil, core.ErrUnsupported
}
