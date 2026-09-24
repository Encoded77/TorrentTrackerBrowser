package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// jobFile streams one file of a job: a 302 to a fresh direct link when the
// engine has one, else a Range-capable stream from the local path or the
// engine's Open.
func (s *Server) jobFile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	eng, f, err := s.Runner.ResolveFile(ctx, r.PathValue("id"), r.PathValue("path"))
	cancel()
	if errors.Is(err, core.ErrJobNotFound) || errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "unknown job or file")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "engine_error", err.Error())
		return
	}
	if eng.Caps().DirectLinks && f.DirectURL != nil {
		u, err := f.DirectURL(r.Context())
		if err != nil {
			writeError(w, http.StatusBadGateway, "engine_error", err.Error())
			return
		}
		http.Redirect(w, r, u, http.StatusFound)
		return
	}
	name := path.Base(f.Path)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if f.LocalPath != "" {
		fh, err := os.Open(f.LocalPath)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "file is not on disk")
			return
		}
		defer fh.Close()
		st, _ := fh.Stat()
		var mod time.Time
		if st != nil {
			mod = st.ModTime()
		}
		http.ServeContent(w, r, name, mod, fh)
		return
	}
	if f.Open == nil {
		writeError(w, http.StatusConflict, "no_stream", "engine exposes no stream for this file")
		return
	}
	streamRange(w, r, f)
}

// streamRange serves a File through its Open with single-range support
// ("bytes=start-" or "bytes=start-end"). Other ranges get the whole file.
func streamRange(w http.ResponseWriter, r *http.Request, f core.File) {
	start, end := int64(0), f.Size-1
	partial := false
	if rng := r.Header.Get("Range"); strings.HasPrefix(rng, "bytes=") && f.Size > 0 {
		spec := strings.TrimPrefix(rng, "bytes=")
		if !strings.Contains(spec, ",") {
			a, b, _ := strings.Cut(spec, "-")
			if s, err := strconv.ParseInt(a, 10, 64); err == nil && s >= 0 && s < f.Size {
				start, partial = s, true
				if e, err := strconv.ParseInt(b, 10, 64); err == nil && e >= s && e < f.Size {
					end = e
				}
			} else if a == "" {
				if n, err := strconv.ParseInt(b, 10, 64); err == nil && n > 0 {
					start, partial = max(f.Size-n, 0), true
				}
			} else {
				w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(f.Size, 10))
				writeError(w, http.StatusRequestedRangeNotSatisfiable, "bad_range", "range out of bounds")
				return
			}
		}
	}
	rc, err := f.Open(r.Context(), start)
	if err != nil {
		writeError(w, http.StatusBadGateway, "engine_error", err.Error())
		return
	}
	defer rc.Close()
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	}
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, f.Size))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if r.Method == http.MethodHead {
		return
	}
	var src io.Reader = rc
	if f.Size > 0 {
		src = io.LimitReader(rc, end-start+1)
	}
	_, _ = io.Copy(w, src)
}
