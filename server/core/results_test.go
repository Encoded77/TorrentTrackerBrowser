package core

import (
	"strings"
	"testing"
	"time"
)

func TestResultCacheTTLAndCap(t *testing.T) {
	rc := NewResultCache(10*time.Minute, 3)
	now := time.Now()
	rc.c.now = func() time.Time { return now }

	r := rc.Add(Result{Title: "one", Ref: map[string]string{"guid": "g1"}})
	if !strings.HasPrefix(r.ID, "r_") || len(r.ID) != 18 {
		t.Fatalf("id = %q", r.ID)
	}
	got, ok := rc.Get(r.ID)
	if !ok || got.Title != "one" || got.Ref["guid"] != "g1" || got.ID != r.ID {
		t.Fatalf("get = %+v %v", got, ok)
	}

	now = now.Add(11 * time.Minute)
	if _, ok := rc.Get(r.ID); ok {
		t.Error("expired entry still returned")
	}

	// Cap: the oldest entry is evicted when full.
	a := rc.Add(Result{Title: "a"})
	rc.Add(Result{Title: "b"})
	rc.Add(Result{Title: "c"})
	rc.Add(Result{Title: "d"})
	if _, ok := rc.Get(a.ID); ok {
		t.Error("oldest entry should have been evicted at cap")
	}
	if len(rc.c.entries) != 3 {
		t.Errorf("entries = %d, want 3", len(rc.c.entries))
	}
}

func TestPayloadCache(t *testing.T) {
	pc := NewPayloadCache(time.Minute, 10)
	id := pc.Add(Payload{Magnet: "magnet:?xt=urn:btih:x"})
	if !strings.HasPrefix(id, "p_") {
		t.Fatalf("id = %q", id)
	}
	if p, ok := pc.Get(id); !ok || p.Magnet == "" {
		t.Error("payload not found")
	}
	if _, ok := pc.Get("p_nope"); ok {
		t.Error("unknown id found")
	}
}
