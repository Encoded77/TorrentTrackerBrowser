package core

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Searcher fans a query out across every configured Source concurrently and
// forwards the per-indexer batches, with ids assigned and results capped.
type Searcher struct {
	Sources        []Source
	Results        *ResultCache
	IndexerTimeout time.Duration
	PerIndexer     int
}

// Search runs q on every source and returns the number of results emitted.
// emit is called from several goroutines but never concurrently, and never
// after Search returns. Cancelling ctx stops the fan-out.
func (s *Searcher) Search(ctx context.Context, q Query, emit func(SearchEvent)) int {
	var (
		mu    sync.Mutex
		total int
		wg    sync.WaitGroup
	)
	safeEmit := func(ev SearchEvent) {
		mu.Lock()
		defer mu.Unlock()
		total += len(ev.Results)
		emit(ev)
	}
	for _, src := range s.Sources {
		wg.Add(1)
		go func(src Source) {
			defer wg.Done()
			s.searchSource(ctx, src, q, safeEmit)
		}(src)
	}
	wg.Wait()
	return total
}

func (s *Searcher) searchSource(ctx context.Context, src Source, q Query, emit func(SearchEvent)) {
	ictx, cancel := context.WithTimeout(ctx, s.IndexerTimeout)
	defer cancel()
	start := time.Now()
	all, err := src.Indexers(ictx)
	if err != nil {
		emit(SearchEvent{Indexer: Indexer{ID: src.ID(), Name: src.Name()}, Done: true, Err: err, Elapsed: time.Since(start)})
		return
	}
	expected := map[string]Indexer{}
	sq := q
	sq.Indexers = nil
	sq.Limit = s.PerIndexer
	for _, ix := range all {
		if !strings.HasPrefix(ix.ID, src.ID()+":") {
			continue
		}
		if len(q.Indexers) > 0 && !contains(q.Indexers, ix.ID) {
			continue
		}
		expected[ix.ID] = ix
		sq.Indexers = append(sq.Indexers, ix.ID)
	}
	if len(expected) == 0 {
		return
	}

	var (
		mu     sync.Mutex
		closed bool
		counts = map[string]int{}
		done   = map[string]bool{}
	)
	wrapped := func(ev SearchEvent) {
		mu.Lock()
		defer mu.Unlock()
		if closed || done[ev.Indexer.ID] {
			return
		}
		room := s.PerIndexer - counts[ev.Indexer.ID]
		if room < 0 {
			room = 0
		}
		if len(ev.Results) > room {
			ev.Results = ev.Results[:room]
		}
		out := make([]Result, 0, len(ev.Results))
		for _, r := range ev.Results {
			r.Source = src.ID()
			if r.Language == "" {
				r.Language = DetectLanguage(r.Title)
			}
			if r.Categories == nil {
				r.Categories = []Canonical{}
			}
			out = append(out, s.Results.Add(r))
		}
		ev.Results = out
		counts[ev.Indexer.ID] += len(out)
		if ev.Done {
			done[ev.Indexer.ID] = true
		}
		emit(ev)
	}
	if err := src.Search(ictx, sq, wrapped); err != nil {
		slog.Warn("search: source failed", "source", src.ID(), "err", err)
	}
	mu.Lock()
	closed = true
	missing := []Indexer{}
	for id, ix := range expected {
		if !done[id] {
			missing = append(missing, ix)
		}
	}
	mu.Unlock()
	for _, ix := range missing {
		cause := ictx.Err()
		if cause == nil {
			cause = errors.New("indexer did not report")
		} else if errors.Is(cause, context.DeadlineExceeded) {
			cause = errors.New("timeout")
		}
		emit(SearchEvent{Indexer: ix, Done: true, Err: cause, Elapsed: time.Since(start)})
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
