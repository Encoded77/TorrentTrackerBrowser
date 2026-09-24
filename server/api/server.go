// Package api exposes the HTTP contract of API.md over the core services.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// Server bundles the services the handlers need.
type Server struct {
	Config   *core.Config
	Reg      *core.Registry
	Results  *core.ResultCache
	Payloads *core.PayloadCache
	Searcher *core.Searcher
	Runner   *core.Runner
}

// Handler builds the full router: /healthz, /api/* with the guards, and the
// SPA for everything else. Every request goes through the Host allowlist and
// the access log.
func (s *Server) Handler(ui http.Handler) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /api/capabilities", s.capabilities)
	api.HandleFunc("GET /api/search", s.search)
	api.HandleFunc("POST /api/payloads", s.createPayload)
	api.HandleFunc("GET /api/results/{id}/files", s.resultFiles)
	api.HandleFunc("GET /api/results/{id}/payload", s.resultPayload)
	api.HandleFunc("POST /api/cached", s.cached)
	api.HandleFunc("GET /api/jobs", s.listJobs)
	api.HandleFunc("POST /api/jobs", s.createJob)
	api.HandleFunc("POST /api/jobs/{id}/cancel", s.cancelJob)
	api.HandleFunc("POST /api/jobs/{id}/retry", s.retryJob)
	api.HandleFunc("POST /api/jobs/{id}/send", s.sendJob)
	api.HandleFunc("DELETE /api/jobs/{id}", s.deleteJob)
	api.HandleFunc("GET /api/jobs/{id}/files/{path...}", s.jobFile)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such route")
	})

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	root.Handle("/api/", requireToken(s.Config.Token, mutationGuard(api)))
	root.Handle("/", ui)
	return accessLog(hostAllowlist(s.Config.AllowedHosts, root))
}

// apiError is the JSON error envelope.
type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var e apiError
	e.Error.Code, e.Error.Message = code, msg
	writeJSON(w, status, e)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// readJSON decodes a JSON body (1 MB cap) and rejects unknown fields.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", err.Error())
		return false
	}
	return true
}
