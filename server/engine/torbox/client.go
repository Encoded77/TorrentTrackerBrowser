package torbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// client wraps the TorBox JSON API: bearer auth, a 5 req/s limiter and one
// retry with backoff on 429/5xx.
type client struct {
	base   string
	apiKey string
	http   *http.Client

	mu   sync.Mutex
	last time.Time
}

const minInterval = 200 * time.Millisecond // 5 requests per second

// envelope is the common TorBox response shape.
type envelope struct {
	Success bool            `json:"success"`
	Error   *string         `json:"error"`
	Detail  string          `json:"detail"`
	Data    json.RawMessage `json:"data"`
}

func (c *client) wait(ctx context.Context) error {
	c.mu.Lock()
	next := c.last.Add(minInterval)
	now := time.Now()
	if next.After(now) {
		c.last = next
	} else {
		c.last = now
	}
	delay := c.last.Sub(now)
	c.mu.Unlock()
	if delay <= 0 {
		return nil
	}
	select {
	case <-time.After(delay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// do sends a request (body built by mk so it can be replayed on retry) and
// decodes the envelope, returning data when success is true.
func (c *client) do(ctx context.Context, method, path string, mk func() (io.Reader, string)) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
		var body io.Reader
		var ctype string
		if mk != nil {
			body, ctype = mk()
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = core.Retryable(err)
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = core.Retryable(err)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = core.Retryable(fmt.Errorf("torbox %s: HTTP %d", path, resp.StatusCode))
			continue
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, fmt.Errorf("torbox %s: bad JSON (HTTP %d): %s", path, resp.StatusCode, snippet(raw))
		}
		if resp.StatusCode >= 400 || !env.Success {
			msg := env.Detail
			if env.Error != nil && *env.Error != "" {
				msg = *env.Error + ": " + msg
			}
			if msg == "" {
				msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("torbox %s: %s", path, msg)
			}
			return nil, fmt.Errorf("torbox %s: %s", path, msg)
		}
		return env.Data, nil
	}
	return nil, lastErr
}

func (c *client) get(ctx context.Context, path string, out any) error {
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *client) postJSON(ctx context.Context, path string, in any, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	data, err := c.do(ctx, http.MethodPost, path, func() (io.Reader, string) {
		return bytes.NewReader(body), "application/json"
	})
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	return json.Unmarshal(data, out)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
