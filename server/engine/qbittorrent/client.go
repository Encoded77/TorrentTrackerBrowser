package qbittorrent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

// client speaks the qBittorrent WebUI API v2 with cookie auth. With an empty
// username the login call is skipped (IP-whitelisted WebUI).
type client struct {
	base     string
	username string
	password string
	http     *http.Client

	mu       sync.Mutex
	loggedIn bool
}

func newClient(base, username, password string) (*client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &client{base: strings.TrimRight(base, "/"), username: username, password: password,
		http: &http.Client{Jar: jar, Timeout: 60 * time.Second}}, nil
}

func (c *client) login(ctx context.Context) error {
	if c.username == "" {
		return nil
	}
	form := url.Values{"username": {c.username}, "password": {c.password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		return core.Retryable(err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "Ok") {
		return fmt.Errorf("qbittorrent login failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	c.mu.Lock()
	c.loggedIn = true
	c.mu.Unlock()
	return nil
}

// do sends a request, logging in first when needed and once more on 403.
// mk builds the body so it can be replayed. The response body is returned.
func (c *client) do(ctx context.Context, method, path string, mk func() (io.Reader, string)) ([]byte, error) {
	c.mu.Lock()
	needLogin := !c.loggedIn && c.username != ""
	c.mu.Unlock()
	if needLogin {
		if err := c.login(ctx); err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		var body io.Reader
		var ctype string
		if mk != nil {
			body, ctype = mk()
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
		if err != nil {
			return nil, err
		}
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		req.Header.Set("Referer", c.base)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, core.Retryable(err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if err != nil {
			return nil, core.Retryable(err)
		}
		if resp.StatusCode == http.StatusForbidden && attempt == 0 && c.username != "" {
			c.mu.Lock()
			c.loggedIn = false
			c.mu.Unlock()
			if err := c.login(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, core.ErrNotFound
		}
		if resp.StatusCode >= 500 {
			return nil, core.Retryable(fmt.Errorf("qbittorrent %s: HTTP %d", path, resp.StatusCode))
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("qbittorrent %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return raw, nil
	}
	return nil, errors.New("qbittorrent: unauthorized after re-login")
}

func (c *client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	raw, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (c *client) postForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, func() (io.Reader, string) {
		return strings.NewReader(form.Encode()), "application/x-www-form-urlencoded"
	})
}
