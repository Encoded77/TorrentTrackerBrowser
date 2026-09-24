package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// fetchResult resolves a result id to its cached entry and fetches its payload.
func (s *Server) fetchResult(ctx context.Context, w http.ResponseWriter, id string) (core.Result, core.Payload, bool) {
	res, ok := s.Results.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "result_expired", "unknown or expired result id")
		return res, core.Payload{}, false
	}
	src := s.Reg.Source(res.Source)
	if src == nil {
		writeError(w, http.StatusInternalServerError, "no_source", "source of this result is gone")
		return res, core.Payload{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := src.Fetch(ctx, res)
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch_failed", err.Error())
		return res, core.Payload{}, false
	}
	return res, p, true
}

// fetchTorrent prefers the .torrent form when the source can provide it.
func (s *Server) fetchTorrent(ctx context.Context, res core.Result) (core.Payload, error) {
	src := s.Reg.Source(res.Source)
	if src == nil {
		return core.Payload{}, errors.New("source of this result is gone")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if tf, ok := src.(core.TorrentFetcher); ok {
		return tf.FetchTorrent(ctx, res)
	}
	return src.Fetch(ctx, res)
}

// resultFiles previews a result's files from its .torrent, or from an engine
// that already holds the hash.
func (s *Server) resultFiles(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, ok := s.Results.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "result_expired", "unknown or expired result id")
		return
	}
	if res.HasTorrent {
		if p, err := s.fetchTorrent(r.Context(), res); err != nil {
			slog.Warn("preview from torrent", "result", id, "err", err)
		} else if len(p.Torrent) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{"files": p.Files})
			return
		}
	}
	if res.InfoHash != "" {
		if files, err := s.previewFromEngine(r.Context(), res.InfoHash); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"files": files})
			return
		} else if !errors.Is(err, errNoPreview) {
			slog.Warn("preview from engine", "hash", res.InfoHash, "err", err)
		}
	}
	writeError(w, http.StatusConflict, "no_preview", "no file list available for this result")
}

var errNoPreview = errors.New("no preview")

// previewFromEngine asks the default engine (falling back to any engine that
// can preview) for the file list of a cached hash.
func (s *Server) previewFromEngine(ctx context.Context, hash string) ([]core.FileEntry, error) {
	engines := []core.Engine{}
	if def := s.Reg.Engine(s.Config.Defaults.Engine); def != nil {
		engines = append(engines, def)
	}
	for _, e := range s.Reg.Engines {
		if e.ID() != s.Config.Defaults.Engine {
			engines = append(engines, e)
		}
	}
	for _, e := range engines {
		pv, ok := e.(core.Previewer)
		if !ok || !e.Caps().Cached {
			continue
		}
		cached, err := e.Cached(ctx, []string{hash})
		if err != nil {
			return nil, err
		}
		if !cached[strings.ToLower(hash)] {
			continue
		}
		files, err := pv.Preview(ctx, hash)
		if err != nil {
			return nil, err
		}
		if files == nil {
			files = []core.FileEntry{}
		}
		return files, nil
	}
	return nil, errNoPreview
}

// resultPayload hands the magnet or the .torrent to the user's machine.
func (s *Server) resultPayload(w http.ResponseWriter, r *http.Request) {
	as := r.URL.Query().Get("as")
	if as != "magnet" && as != "torrent" {
		writeError(w, http.StatusBadRequest, "bad_form", "as must be magnet or torrent")
		return
	}
	res, ok := s.Results.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "result_expired", "unknown or expired result id")
		return
	}
	var p core.Payload
	var err error
	if as == "torrent" && res.HasTorrent {
		p, err = s.fetchTorrent(r.Context(), res)
	} else {
		_, p, ok = s.fetchResult(r.Context(), w, res.ID)
		if !ok {
			return
		}
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "fetch_failed", err.Error())
		return
	}
	switch as {
	case "magnet":
		if p.Magnet == "" {
			writeError(w, http.StatusNotFound, "no_magnet", "this result has no magnet link")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, p.Magnet)
	case "torrent":
		if len(p.Torrent) == 0 {
			writeError(w, http.StatusNotFound, "no_torrent", "this result has no .torrent file")
			return
		}
		name := p.Name
		if name == "" {
			name = res.Title
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".torrent"}))
		w.Write(p.Torrent)
	}
}

// cached answers {engine, hashes} for engines with caps.cached.
func (s *Server) cached(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Engine string   `json:"engine"`
		Hashes []string `json:"hashes"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	e := s.Reg.Engine(body.Engine)
	if e == nil {
		writeError(w, http.StatusBadRequest, "unknown_engine", "unknown engine "+body.Engine)
		return
	}
	if !e.Caps().Cached {
		writeError(w, http.StatusBadRequest, "not_supported", "engine "+body.Engine+" cannot report cached hashes")
		return
	}
	if len(body.Hashes) > 500 {
		writeError(w, http.StatusBadRequest, "too_many", "at most 500 hashes per call")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := e.Cached(ctx, body.Hashes)
	if err != nil {
		writeError(w, http.StatusBadGateway, "engine_error", err.Error())
		return
	}
	if res == nil {
		res = map[string]bool{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cached": res})
}
