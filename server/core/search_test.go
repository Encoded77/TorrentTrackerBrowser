package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newSearcher(sources ...Source) *Searcher {
	return &Searcher{Sources: sources, Results: NewResultCache(time.Minute, 100), IndexerTimeout: 200 * time.Millisecond, PerIndexer: 2}
}

func TestSearchFanOutCapsAndTags(t *testing.T) {
	src := &fakeSource{
		id:       "p",
		indexers: []Indexer{{ID: "p:1", Name: "One"}, {ID: "p:2", Name: "Two"}},
		results: map[string][]Result{
			"p:1": {{Title: "Film FRENCH"}, {Title: "Film MULTI"}, {Title: "Film 3 dropped by cap"}},
			"p:2": {{Title: "Show VOSTFR"}},
		},
	}
	s := newSearcher(src)
	var events []SearchEvent
	total := s.Search(context.Background(), Query{Text: "film"}, func(ev SearchEvent) { events = append(events, ev) })
	if total != 3 {
		t.Errorf("total = %d, want 3 (cap 2 + 1)", total)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d", len(events))
	}
	for _, ev := range events {
		if !ev.Done || ev.Err != nil {
			t.Errorf("event %+v not done", ev)
		}
		for _, r := range ev.Results {
			if r.ID == "" || r.Source != "p" {
				t.Errorf("result missing id/source: %+v", r)
			}
			if _, ok := s.Results.Get(r.ID); !ok {
				t.Errorf("result %s not cached", r.ID)
			}
			if r.Categories == nil {
				t.Error("categories should be [] not nil")
			}
		}
		if ev.Indexer.ID == "p:1" {
			if len(ev.Results) != 2 || ev.Results[0].Language != LangFR || ev.Results[1].Language != LangMulti {
				t.Errorf("p:1 results = %+v", ev.Results)
			}
		}
	}
}

func TestSearchIndexerFilterAndTimeout(t *testing.T) {
	src := &fakeSource{
		id:       "p",
		indexers: []Indexer{{ID: "p:1"}, {ID: "p:2"}, {ID: "p:3"}},
		results:  map[string][]Result{"p:1": {{Title: "x"}}},
		hang:     map[string]bool{"p:2": true},
		errs:     map[string]error{"p:3": errors.New("boom")},
	}
	other := &fakeSource{id: "q", indexers: []Indexer{{ID: "q:9"}}, results: map[string][]Result{"q:9": {{Title: "never"}}}}
	s := newSearcher(src, other)
	got := map[string]SearchEvent{}
	start := time.Now()
	s.Search(context.Background(), Query{Text: "x", Indexers: []string{"p:1", "p:2", "p:3"}}, func(ev SearchEvent) { got[ev.Indexer.ID] = ev })
	if time.Since(start) > 2*time.Second {
		t.Error("timeout not enforced")
	}
	if _, ok := got["q:9"]; ok {
		t.Error("unselected source should not run")
	}
	if ev := got["p:2"]; !ev.Done || ev.Err == nil {
		t.Errorf("hanging indexer should end with an error: %+v", ev)
	}
	if ev := got["p:3"]; ev.Err == nil || ev.Err.Error() != "boom" {
		t.Errorf("p:3 = %+v", ev)
	}
	if ev := got["p:1"]; len(ev.Results) != 1 {
		t.Errorf("p:1 = %+v", ev)
	}
}

func TestSearchCancel(t *testing.T) {
	src := &fakeSource{id: "p", indexers: []Indexer{{ID: "p:1"}}, hang: map[string]bool{"p:1": true}}
	s := newSearcher(src)
	s.IndexerTimeout = time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	done := make(chan struct{})
	go func() { s.Search(ctx, Query{Text: "x"}, func(SearchEvent) {}); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("search did not stop on cancel")
	}
}

func TestSearchIndexerListError(t *testing.T) {
	src := &fakeSource{id: "p", idxErr: errors.New("down")}
	var events []SearchEvent
	newSearcher(src).Search(context.Background(), Query{Text: "x"}, func(ev SearchEvent) { events = append(events, ev) })
	if len(events) != 1 || events[0].Err == nil || events[0].Indexer.ID != "p" {
		t.Errorf("events = %+v", events)
	}
}
