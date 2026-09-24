package core

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Webhook posts job outcomes to an ntfy-style topic URL: the message is the
// plain-text body and the title, priority and tags travel as headers. That is
// what ntfy expects on a topic URL (a JSON body would be shown verbatim), and
// it keeps the URL free to carry ntfy's ?auth= query parameter. An empty URL
// is a no-op.
type Webhook struct {
	URL    string
	Client *http.Client
}

// NewWebhook builds a Notifier for url; empty url disables notifications.
func NewWebhook(url string) Notifier {
	return &Webhook{URL: url, Client: &http.Client{Timeout: 10 * time.Second}}
}

// Notify sends the message; failures are only logged.
func (w *Webhook) Notify(ctx context.Context, title, message string, failed bool) {
	if w == nil || w.URL == "" {
		return
	}
	priority := 3
	if failed {
		priority = 4
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, strings.NewReader(message))
	if err != nil {
		slog.Warn("notify: build request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", title)
	req.Header.Set("Priority", strconv.Itoa(priority))
	req.Header.Set("Tags", "torrent")
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
