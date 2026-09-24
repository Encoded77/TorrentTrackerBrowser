package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

type createJobBody struct {
	ResultID  *string   `json:"resultId"`
	PayloadID *string   `json:"payloadId"`
	Engine    string    `json:"engine"`
	Storage   *string   `json:"storage"`
	Subdir    *string   `json:"subdir"`
	Files     []int     `json:"files"`
	Mode      core.Mode `json:"mode"`
}

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// validateTarget checks engine, mode, storage and subdir; shared by create and send.
func (s *Server) validateTarget(w http.ResponseWriter, engineID string, mode core.Mode, storage, subdir string, files []int) (core.Mode, string, bool) {
	eng := s.Reg.Engine(engineID)
	if eng == nil {
		writeError(w, http.StatusBadRequest, "unknown_engine", "unknown engine "+engineID)
		return "", "", false
	}
	if mode == "" {
		mode = s.Config.Defaults.Mode
	}
	switch mode {
	case core.ModeCopy, core.ModeLinks:
	case core.ModeAdopt:
		if !eng.Caps().SavePath || !eng.Caps().LocalFiles {
			writeError(w, http.StatusBadRequest, "bad_mode", "engine "+engineID+" cannot adopt (needs savePath and localFiles)")
			return "", "", false
		}
	default:
		writeError(w, http.StatusBadRequest, "bad_mode", "mode must be copy, adopt or links")
		return "", "", false
	}
	if mode == core.ModeLinks {
		if !eng.Caps().DirectLinks && !eng.Caps().LocalFiles {
			writeError(w, http.StatusBadRequest, "bad_mode", "engine "+engineID+" cannot serve links")
			return "", "", false
		}
	} else if s.Reg.Storage(storage) == nil {
		writeError(w, http.StatusBadRequest, "unknown_storage", "storage is required for this mode")
		return "", "", false
	}
	clean, err := core.SafeSubdir(subdir)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_subdir", err.Error())
		return "", "", false
	}
	for _, f := range files {
		if f < 0 {
			writeError(w, http.StatusBadRequest, "bad_files", "file indexes must be >= 0")
			return "", "", false
		}
	}
	return mode, clean, true
}

// createJob turns a result or an uploaded payload into a job.
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var b createJobBody
	if !readJSON(w, r, &b) {
		return
	}
	if (b.ResultID == nil) == (b.PayloadID == nil) {
		writeError(w, http.StatusBadRequest, "bad_request", "exactly one of resultId or payloadId is required")
		return
	}
	if b.Engine == "" {
		b.Engine = s.Config.Defaults.Engine
	}
	storage := strOf(b.Storage)
	if storage == "" && b.Mode != core.ModeLinks {
		storage = s.Config.Defaults.Storage
	}
	mode, subdir, ok := s.validateTarget(w, b.Engine, b.Mode, storage, strOf(b.Subdir), b.Files)
	if !ok {
		return
	}
	var p core.Payload
	var name, hash string
	if b.PayloadID != nil {
		p, ok = s.Payloads.Get(*b.PayloadID)
		if !ok {
			writeError(w, http.StatusNotFound, "payload_expired", "unknown or expired payload id")
			return
		}
	} else {
		var res core.Result
		res, p, ok = s.fetchResult(r.Context(), w, *b.ResultID)
		if !ok {
			return
		}
		name, hash = res.Title, res.InfoHash
	}
	if p.Name != "" && name == "" {
		name = p.Name
	}
	if name == "" {
		name = p.InfoHash
	}
	if hash == "" {
		hash = p.InfoHash
	}
	if mode == core.ModeLinks {
		storage = ""
	}
	j := &core.Job{Name: name, InfoHash: hash, Engine: b.Engine, Storage: storage, Subdir: subdir, Mode: mode,
		Payload: p, Selection: b.Files}
	writeJSON(w, http.StatusCreated, s.Runner.Submit(j).View())
}

// listJobs returns own jobs newest first, then external engine items.
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	views := []core.JobView{}
	for _, j := range s.Runner.Store.List() {
		views = append(views, j.View())
	}
	for _, j := range s.Runner.External(ctx) {
		views = append(views, j.View())
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": views})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.Runner.Cancel(r.Context(), r.PathValue("id"))
	if errors.Is(err, core.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "unknown job")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "bad_state", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, j.View())
}

func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.Runner.Retry(r.PathValue("id"))
	if errors.Is(err, core.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "unknown job")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "bad_state", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, j.View())
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	deleteFiles := r.URL.Query().Get("deleteFiles") == "true"
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	err := s.Runner.Delete(ctx, r.PathValue("id"), deleteFiles)
	if errors.Is(err, core.ErrJobNotFound) || errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "unknown job")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "engine_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sendJob sends an external engine item to a storage.
func (s *Server) sendJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !strings.HasPrefix(id, "x_") {
		writeError(w, http.StatusBadRequest, "bad_request", "send applies to external items only")
		return
	}
	var b struct {
		Storage *string   `json:"storage"`
		Subdir  *string   `json:"subdir"`
		Files   []int     `json:"files"`
		Mode    core.Mode `json:"mode"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	engineID := ""
	for _, e := range s.Reg.Engines {
		if strings.HasPrefix(id, "x_"+e.ID()+"_") {
			engineID = e.ID()
		}
	}
	if engineID == "" {
		writeError(w, http.StatusNotFound, "not_found", "unknown item")
		return
	}
	storage := strOf(b.Storage)
	if storage == "" && b.Mode != core.ModeLinks {
		storage = s.Config.Defaults.Storage
	}
	mode, subdir, ok := s.validateTarget(w, engineID, b.Mode, storage, strOf(b.Subdir), b.Files)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	j, err := s.Runner.Send(ctx, id, core.SendOpts{Storage: storage, Subdir: subdir, Files: b.Files, Mode: mode})
	if errors.Is(err, core.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "unknown item")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "engine_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, j.View())
}
