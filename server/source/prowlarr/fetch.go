package prowlarr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

const maxRedirects = 3

// Fetch turns a result into a payload: the magnet when the release has one,
// else the download URL, which may redirect to a magnet or serve a .torrent.
func (s *Source) Fetch(ctx context.Context, r core.Result) (core.Payload, error) {
	if m := r.Ref["magnet"]; m != "" {
		p, err := core.PayloadFromMagnet(m)
		if err != nil {
			return core.Payload{}, err
		}
		if p.Name == "" {
			p.Name = r.Title
		}
		p.Size = r.Size
		return p, nil
	}
	return s.FetchTorrent(ctx, r)
}

// FetchTorrent follows the download URL (max 3 hops) and returns the
// .torrent payload, or the magnet a redirect points to.
func (s *Source) FetchTorrent(ctx context.Context, r core.Result) (core.Payload, error) {
	u := r.Ref["download"]
	if u == "" {
		return core.Payload{}, errors.New("result has no download link")
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for hop := 0; hop <= maxRedirects; hop++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return core.Payload{}, err
		}
		req.Header.Set("X-Api-Key", s.apiKey)
		resp, err := noRedirect.Do(req)
		if err != nil {
			return core.Payload{}, core.Retryable(err)
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if strings.HasPrefix(strings.ToLower(loc), "magnet:") {
				p, err := core.PayloadFromMagnet(loc)
				if err != nil {
					return core.Payload{}, err
				}
				if p.Name == "" {
					p.Name = r.Title
				}
				p.Size = r.Size
				return p, nil
			}
			next, err := resp.Request.URL.Parse(loc)
			if err != nil || loc == "" {
				return core.Payload{}, fmt.Errorf("bad redirect from %s", u)
			}
			u = next.String()
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if err != nil {
			return core.Payload{}, core.Retryable(err)
		}
		if resp.StatusCode >= 500 {
			return core.Payload{}, core.Retryable(fmt.Errorf("download: HTTP %d", resp.StatusCode))
		}
		if resp.StatusCode >= 400 {
			return core.Payload{}, fmt.Errorf("download: HTTP %d: %s", resp.StatusCode, snippet(body))
		}
		p, err := core.PayloadFromTorrent(body)
		if err != nil {
			return core.Payload{}, fmt.Errorf("download did not return a torrent: %w", err)
		}
		if p.Name == "" {
			p.Name = r.Title
		}
		return p, nil
	}
	return core.Payload{}, errors.New("too many redirects")
}
