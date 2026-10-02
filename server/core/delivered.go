package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// notifyDelivered POSTs {storage, subdir, paths} to the storage's onDelivered
// URL (config key on the storage entry) once files land there, by a finished
// job or a move. Fire and forget: a library behind that storage (RomM...) uses
// it to index the new files now rather than at its next scheduled scan.
func (r *Runner) notifyDelivered(j Job) {
	url := r.Delivered[j.Storage]
	if url == "" {
		return
	}
	body := struct {
		Storage string   `json:"storage"`
		Subdir  string   `json:"subdir"`
		Paths   []string `json:"paths"`
	}{Storage: j.Storage, Subdir: j.Subdir, Paths: []string{}}
	for _, f := range j.Files {
		if f.State == FileDone {
			body.Paths = append(body.Paths, f.Path)
		}
	}
	b, _ := json.Marshal(body)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			var res *http.Response
			if res, err = http.DefaultClient.Do(req); err == nil {
				res.Body.Close()
				if res.StatusCode >= 300 {
					slog.Warn("onDelivered hook", "storage", j.Storage, "status", res.StatusCode)
				}
				return
			}
		}
		slog.Warn("onDelivered hook", "storage", j.Storage, "err", err)
	}()
}
