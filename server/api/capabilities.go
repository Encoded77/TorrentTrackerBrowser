package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

type sourceView struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Indexers []core.Indexer `json:"indexers"`
}

type engineView struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Caps core.EngineCaps `json:"caps"`
}

type storageView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Free  *int64 `json:"free"`
}

type capabilitiesView struct {
	Sources    []sourceView     `json:"sources"`
	Engines    []engineView     `json:"engines"`
	Storages   []storageView    `json:"storages"`
	Categories []core.Canonical `json:"categories"`
	Defaults   core.Defaults    `json:"defaults"`
	Languages  []core.Language  `json:"languages"`
}

// capabilities describes what the backend can do; no secret ever leaves here.
func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	v := capabilitiesView{Sources: []sourceView{}, Engines: []engineView{}, Storages: []storageView{},
		Categories: core.Categories, Defaults: s.Config.Defaults, Languages: core.Languages}
	for _, src := range s.Reg.Sources {
		idx, err := src.Indexers(ctx)
		if err != nil {
			slog.Warn("capabilities: indexers", "source", src.ID(), "err", err)
			idx = []core.Indexer{}
		}
		if idx == nil {
			idx = []core.Indexer{}
		}
		v.Sources = append(v.Sources, sourceView{ID: src.ID(), Name: src.Name(), Indexers: idx})
	}
	for _, e := range s.Reg.Engines {
		v.Engines = append(v.Engines, engineView{ID: e.ID(), Name: e.Name(), Caps: e.Caps()})
	}
	for _, st := range s.Reg.Storages {
		sv := storageView{ID: st.ID(), Label: st.Label()}
		if free, err := st.Free(ctx); err == nil {
			sv.Free = &free
		} else {
			slog.Warn("capabilities: free space", "storage", st.ID(), "err", err)
		}
		v.Storages = append(v.Storages, sv)
	}
	writeJSON(w, http.StatusOK, v)
}
