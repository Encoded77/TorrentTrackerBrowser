package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// shareRequest is what share.url receives: paths relative to share.storage.
type shareRequest struct {
	Paths       []string `json:"paths"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Days        int      `json:"days"` // 0 = never expires
	Password    string   `json:"password,omitempty"`
}

// shareJob turns the delivered files of a finished job into a share link.
func (s *Server) shareJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.ownJob(w, r, id) {
		return
	}
	if s.Config.Share.URL == "" {
		writeError(w, http.StatusNotFound, "not_found", "sharing is not configured")
		return
	}
	var b struct {
		Days     int    `json:"days"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &b) {
		return
	}
	if b.Days < 0 || b.Days > 365 {
		writeError(w, http.StatusBadRequest, "bad_request", "days must be between 0 and 365")
		return
	}
	j, ok := s.Runner.Store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "unknown job")
		return
	}
	if j.State != core.JobDone || j.Storage != s.Config.Share.Storage {
		writeError(w, http.StatusConflict, "bad_state", "only finished jobs delivered to "+s.Config.Share.Storage+" can be shared")
		return
	}
	req := shareRequest{Name: j.Name, Days: b.Days, Password: b.Password}
	if u := s.user(r); u != "" {
		req.Description = "TTB, " + u
	}
	for _, f := range j.Files {
		if f.State == core.FileDone || f.State == core.FileSkipped {
			req.Paths = append(req.Paths, f.Path)
		}
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusConflict, "bad_state", "the job has no delivered file")
		return
	}
	// The share service may hash a password (argon2) and walk the NAS: be patient.
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	link, err := postShare(ctx, s.Config.Share.URL, req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "share_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": link})
}

func postShare(ctx context.Context, url string, req shareRequest) (string, error) {
	body, _ := json.Marshal(req)
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	hr.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(hr)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	var out struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &out)
	if res.StatusCode != http.StatusOK || out.URL == "" {
		msg := out.Error
		if msg == "" {
			msg = string(bytes.TrimSpace(raw))
		}
		return "", fmt.Errorf("share service: HTTP %d: %s", res.StatusCode, msg)
	}
	return out.URL, nil
}
