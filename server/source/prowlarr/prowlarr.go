// Package prowlarr implements core.Source over the Prowlarr v1 API, fanning
// every search out per indexer.
package prowlarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

func init() {
	core.RegisterSource("prowlarr", func(cfg map[string]any) (core.Source, error) {
		return New(core.StringOpt(cfg, "id"), core.StringOpt(cfg, "url"), core.StringOpt(cfg, "apiKey"))
	})
}

// Source is one Prowlarr instance.
type Source struct {
	id     string
	base   string
	apiKey string
	client *http.Client

	mu       sync.Mutex
	indexers []core.Indexer
	upstream map[string]int // namespaced id -> Prowlarr indexer id
	cachedAt time.Time
}

// New validates the config and returns the source.
func New(id, base, apiKey string) (*Source, error) {
	if base == "" {
		return nil, errors.New("missing url")
	}
	if apiKey == "" {
		return nil, errors.New("missing apiKey")
	}
	return &Source{id: id, base: strings.TrimRight(base, "/"), apiKey: apiKey, client: &http.Client{}}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Name() string { return "Prowlarr" }

// get performs an authenticated GET and decodes JSON into out.
func (s *Source) get(ctx context.Context, path string, q url.Values, out any) error {
	u := s.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", s.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("prowlarr %s: HTTP %d: %s", path, resp.StatusCode, snippet(body))
	}
	return json.Unmarshal(body, out)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

type indexerDTO struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Privacy string `json:"privacy"`
	Enable  bool   `json:"enable"`
}

// Indexers lists the enabled Prowlarr indexers (cached for one minute).
func (s *Source) Indexers(ctx context.Context) ([]core.Indexer, error) {
	s.mu.Lock()
	if s.indexers != nil && time.Since(s.cachedAt) < time.Minute {
		out := s.indexers
		s.mu.Unlock()
		return out, nil
	}
	s.mu.Unlock()
	var dtos []indexerDTO
	if err := s.get(ctx, "/api/v1/indexer", nil, &dtos); err != nil {
		return nil, err
	}
	out := []core.Indexer{}
	upstream := map[string]int{}
	for _, d := range dtos {
		if !d.Enable {
			continue
		}
		nsID := s.id + ":" + strconv.Itoa(d.ID)
		out = append(out, core.Indexer{ID: nsID, Name: d.Name, Private: d.Privacy != "public"})
		upstream[nsID] = d.ID
	}
	s.mu.Lock()
	s.indexers, s.upstream, s.cachedAt = out, upstream, time.Now()
	s.mu.Unlock()
	return out, nil
}

type categoryDTO struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type releaseDTO struct {
	GUID         string        `json:"guid"`
	IndexerID    int           `json:"indexerId"`
	Indexer      string        `json:"indexer"`
	Title        string        `json:"title"`
	Size         int64         `json:"size"`
	Seeders      int           `json:"seeders"`
	Leechers     int           `json:"leechers"`
	Grabs        int           `json:"grabs"`
	PublishDate  time.Time     `json:"publishDate"`
	InfoHash     string        `json:"infoHash"`
	MagnetURL    string        `json:"magnetUrl"`
	DownloadURL  string        `json:"downloadUrl"`
	InfoURL      string        `json:"infoUrl"`
	Categories   []categoryDTO `json:"categories"`
	IndexerFlags []string      `json:"indexerFlags"`
}

// Search runs one request per indexer in parallel and emits each as a batch.
func (s *Source) Search(ctx context.Context, q core.Query, emit func(core.SearchEvent)) error {
	all, err := s.Indexers(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	upstream := s.upstream
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, ix := range all {
		if len(q.Indexers) > 0 && !containsStr(q.Indexers, ix.ID) {
			continue
		}
		wg.Add(1)
		go func(ix core.Indexer, upID int) {
			defer wg.Done()
			start := time.Now()
			results, err := s.searchOne(ctx, q, ix, upID)
			emit(core.SearchEvent{Indexer: ix, Results: results, Done: true, Err: err, Elapsed: time.Since(start)})
		}(ix, upstream[ix.ID])
	}
	wg.Wait()
	return nil
}

func (s *Source) searchOne(ctx context.Context, q core.Query, ix core.Indexer, upID int) ([]core.Result, error) {
	v := url.Values{}
	v.Set("query", q.Text)
	v.Set("indexerIds", strconv.Itoa(upID))
	v.Set("type", "search")
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	for _, c := range q.Categories {
		for _, id := range queryCategories[c] {
			v.Add("categories", strconv.Itoa(id))
		}
	}
	var dtos []releaseDTO
	if err := s.get(ctx, "/api/v1/search", v, &dtos); err != nil {
		return nil, err
	}
	out := make([]core.Result, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, s.toResult(d, ix.ID))
	}
	return out, nil
}

func (s *Source) toResult(d releaseDTO, indexerID string) core.Result {
	ids := make([]int, 0, len(d.Categories))
	for _, c := range d.Categories {
		ids = append(ids, c.ID)
	}
	freeleech := false
	for _, f := range d.IndexerFlags {
		if strings.Contains(strings.ToLower(f), "freeleech") {
			freeleech = true
		}
	}
	hash := ""
	if h, err := core.NormalizeInfoHash(d.InfoHash); err == nil {
		hash = h
	}
	// Prowlarr proxies magnets through its own download endpoint (magnetUrl is
	// then an http URL that redirects to the magnet) and keeps the raw magnet in
	// guid. Keep a real magnet when one is available, and the proxied link as a
	// download URL otherwise.
	magnet, download := "", d.DownloadURL
	switch {
	case isMagnet(d.MagnetURL):
		magnet = d.MagnetURL
	case isMagnet(d.GUID):
		magnet = d.GUID
	}
	if magnet == "" && download == "" && d.MagnetURL != "" {
		download = d.MagnetURL
	}
	return core.Result{
		Source: s.id, Indexer: indexerID, Title: d.Title, Size: d.Size,
		Seeders: d.Seeders, Leechers: d.Leechers, Grabs: d.Grabs, Published: d.PublishDate.UTC(),
		InfoHash: hash, HasMagnet: magnet != "", HasTorrent: download != "",
		Freeleech: freeleech, Categories: canonicalSet(ids), InfoURL: d.InfoURL,
		Ref: map[string]string{"magnet": magnet, "download": download, "guid": d.GUID},
	}
}

func isMagnet(u string) bool {
	return strings.HasPrefix(strings.ToLower(u), "magnet:")
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
