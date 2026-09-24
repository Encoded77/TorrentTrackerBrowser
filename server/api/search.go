package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Encoded77/TorrentTrackerBrowser/server/core"
)

type batchEvent struct {
	Indexer   string        `json:"indexer"`
	Results   []core.Result `json:"results"`
	Done      bool          `json:"done"`
	Error     *string       `json:"error"`
	ElapsedMs int64         `json:"elapsedMs"`
}

type doneEvent struct {
	Total     int   `json:"total"`
	ElapsedMs int64 `json:"elapsedMs"`
}

// search streams batches over SSE and stops the fan-out when the client
// disconnects.
func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := core.Query{Text: strings.TrimSpace(r.URL.Query().Get("q"))}
	if q.Text == "" {
		writeError(w, http.StatusBadRequest, "missing_query", "q is required")
		return
	}
	for _, c := range splitList(r.URL.Query().Get("cat")) {
		cat := core.Canonical(c)
		if !containsCanonical(cat) {
			writeError(w, http.StatusBadRequest, "bad_category", "unknown category "+c)
			return
		}
		q.Categories = append(q.Categories, cat)
	}
	q.Indexers = splitList(r.URL.Query().Get("indexers"))

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "no_stream", "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	events := make(chan core.SearchEvent, 16)
	finished := make(chan int, 1)
	start := time.Now()
	go func() {
		total := s.Searcher.Search(ctx, q, func(ev core.SearchEvent) {
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		})
		finished <- total
	}()

	id := 0
	send := func(event string, v any) {
		id++
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event, b)
		flusher.Flush()
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-events:
			send("batch", toBatch(ev))
		case total := <-finished:
			// Drain batches that were queued before the fan-out returned.
			for {
				select {
				case ev := <-events:
					send("batch", toBatch(ev))
					continue
				default:
				}
				break
			}
			send("done", doneEvent{Total: total, ElapsedMs: time.Since(start).Milliseconds()})
			return
		}
	}
}

func toBatch(ev core.SearchEvent) batchEvent {
	b := batchEvent{Indexer: ev.Indexer.ID, Results: ev.Results, Done: ev.Done, ElapsedMs: ev.Elapsed.Milliseconds()}
	if b.Results == nil {
		b.Results = []core.Result{}
	}
	if ev.Err != nil {
		msg := ev.Err.Error()
		b.Error = &msg
	}
	return b
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsCanonical(c core.Canonical) bool {
	for _, k := range core.Categories {
		if k == c {
			return true
		}
	}
	return false
}
