package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Webhook posts ntfy-compatible JSON to a URL. An empty URL is a no-op.
type Webhook struct {
	URL    string
	Client *http.Client
}

// NewWebhook builds a Notifier for url; empty url disables notifications.
func NewWebhook(url string) Notifier {
	return &Webhook{URL: url, Client: &http.Client{Timeout: 10 * time.Second}}
}

// Notify sends {title, message, priority, tags}; failures are only logged.
func (w *Webhook) Notify(ctx context.Context, title, message string, failed bool) {
	if w == nil || w.URL == "" {
		return
	}
	priority := 3
	if failed {
		priority = 4
	}
	body, _ := json.Marshal(map[string]any{
		"title":    title,
		"message":  message,
		"priority": priority,
		"tags":     []string{"torrent"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("notify: build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.Client.Do(req)
	if err != nil {
		slog.Warn("notify: post", "err", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("notify: webhook answered", "status", resp.StatusCode)
	}
}
