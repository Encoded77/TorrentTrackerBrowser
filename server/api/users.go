package api

import (
	"net/http"
	"slices"
	"strings"
)

// user names the requester from users.header; "" is the operator.
func (s *Server) user(r *http.Request) string {
	if s.Config.Users.Header == "" {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(s.Config.Users.Header))
}

// ownJob reports whether the requester may act on job id, answering 404
// otherwise: a named user only reaches their own jobs, never engine items.
func (s *Server) ownJob(w http.ResponseWriter, r *http.Request, id string) bool {
	u := s.user(r)
	if u == "" {
		return true
	}
	if j, ok := s.Runner.Store.Get(id); ok && j.Owner == u {
		return true
	}
	writeError(w, http.StatusNotFound, "not_found", "unknown job")
	return false
}

// storageAllowed reports whether user may deliver to storage.
func (s *Server) storageAllowed(user, storage string) bool {
	return user == "" || len(s.Config.Users.Storages) == 0 || slices.Contains(s.Config.Users.Storages, storage)
}
