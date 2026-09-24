package api

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// hostAllowlist rejects requests whose Host header is not listed (DNS
// rebinding guard). An empty list allows every host.
func hostAllowlist(allowed []string, next http.Handler) http.Handler {
	if len(allowed) == 0 {
		return next
	}
	set := map[string]bool{}
	for _, h := range allowed {
		set[strings.ToLower(h)] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Liveness probes come from the container runtime or a dashboard on
		// the loopback / LAN address, never through the public name.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		host := strings.ToLower(r.Host)
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if !set[host] {
			writeError(w, http.StatusForbidden, "host_not_allowed", "host "+r.Host+" is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireToken checks the shared secret, as a bearer header or ?token=, when
// one is configured.
func requireToken(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || got == r.Header.Get("Authorization") {
			got = r.URL.Query().Get("token")
		}
		if got != token {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// mutationGuard enforces the CSRF rules of API.md on POST and DELETE: a JSON
// content type (multipart allowed for the payload upload) and the custom
// X-Requested-With header.
func mutationGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost || r.Method == http.MethodDelete {
			if r.Header.Get("X-Requested-With") != "TorrentTrackerBrowser" {
				writeError(w, http.StatusForbidden, "forbidden", "missing X-Requested-With header")
				return
			}
			ct := strings.ToLower(r.Header.Get("Content-Type"))
			isJSON := strings.HasPrefix(ct, "application/json")
			isUpload := r.Method == http.MethodPost && r.URL.Path == "/api/payloads" && strings.HasPrefix(ct, "multipart/form-data")
			if !isJSON && !isUpload {
				writeError(w, http.StatusForbidden, "forbidden", "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// accessLog logs method, path, status and duration at info level.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}
