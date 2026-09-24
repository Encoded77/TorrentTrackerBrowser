package core

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// fakeSource emits canned results per indexer, optionally hanging.
type fakeSource struct {
	id       string
	indexers []Indexer
	results  map[string][]Result // by indexer id
	hang     map[string]bool     // indexers that never answer
	errs     map[string]error
	idxErr   error
}

func (f *fakeSource) ID() string   { return f.id }
func (f *fakeSource) Name() string { return "Fake " + f.id }
func (f *fakeSource) Indexers(ctx context.Context) ([]Indexer, error) {
	return f.indexers, f.idxErr
}
func (f *fakeSource) Search(ctx context.Context, q Query, emit func(SearchEvent)) error {
	var wg sync.WaitGroup
	for _, id := range q.Indexers {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if f.hang[id] {
				<-ctx.Done()
				return
			}
			ix := Indexer{ID: id}
			emit(SearchEvent{Indexer: ix, Results: f.results[id], Done: true, Err: f.errs[id], Elapsed: time.Millisecond})
		}(id)
	}
	wg.Wait()
	return nil
}
func (f *fakeSource) Fetch(ctx context.Context, r Result) (Payload, error) {
	return Payload{Magnet: "magnet:?xt=urn:btih:" + strings.Repeat("a", 40), InfoHash: strings.Repeat("a", 40), Name: r.Title}, nil
}

// fakeEngine is a scripted debrid-style engine.
type fakeEngine struct {
	mu       sync.Mutex
	id       string
	caps     EngineCaps
	nextID   int
	items    map[string]*fakeItem
	addErr   error
	statuses []ItemState // successive Status answers, last one repeats
	calls    map[string]int
}

type fakeItem struct {
	item  Item
	files []File
	polls int
}

func newFakeEngine(id string) *fakeEngine {
	return &fakeEngine{id: id, caps: EngineCaps{Cached: true, DirectLinks: true}, items: map[string]*fakeItem{},
		statuses: []ItemState{ItemFetching, ItemReady}, calls: map[string]int{}}
}

func (e *fakeEngine) ID() string       { return e.id }
func (e *fakeEngine) Name() string     { return "Fake" }
func (e *fakeEngine) Caps() EngineCaps { return e.caps }
func (e *fakeEngine) count(op string)  { e.mu.Lock(); e.calls[op]++; e.mu.Unlock() }

func (e *fakeEngine) Add(ctx context.Context, p Payload, o AddOpts) (Item, error) {
	e.count("add")
	if e.addErr != nil {
		return Item{}, e.addErr
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nextID++
	id := string(rune('0' + e.nextID))
	it := Item{ID: id, Name: p.Name, InfoHash: p.InfoHash, Size: 30}
	files := []File{
		{Index: 0, Path: "Root/a.txt", Size: 10, Open: contentOpener("aaaaaaaaaa")},
		{Index: 1, Path: "Root/b.txt", Size: 20, Open: contentOpener("bbbbbbbbbbbbbbbbbbbb")},
	}
	for i := range files {
		u := "https://cdn.example/" + files[i].Path
		files[i].DirectURL = func(context.Context) (string, error) { return u, nil }
	}
	e.items[id] = &fakeItem{item: it, files: files}
	return it, nil
}

func (e *fakeEngine) Status(ctx context.Context, it Item) (ItemStatus, error) {
	e.count("status")
	e.mu.Lock()
	defer e.mu.Unlock()
	fi, ok := e.items[it.ID]
	if !ok {
		return ItemStatus{}, ErrNotFound
	}
	i := fi.polls
	if i >= len(e.statuses) {
		i = len(e.statuses) - 1
	}
	fi.polls++
	st := ItemStatus{State: e.statuses[i], Progress: 0.5}
	if st.State == ItemFailed {
		st.Error = "scripted failure"
	}
	return st, nil
}

func (e *fakeEngine) Select(ctx context.Context, it Item, files []int) error {
	e.count("select")
	return nil
}

func (e *fakeEngine) Files(ctx context.Context, it Item) ([]File, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	fi, ok := e.items[it.ID]
	if !ok {
		return nil, ErrNotFound
	}
	return fi.files, nil
}

func (e *fakeEngine) List(ctx context.Context) ([]Item, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []Item{}
	for _, fi := range e.items {
		out = append(out, fi.item)
	}
	return out, nil
}

func (e *fakeEngine) Remove(ctx context.Context, it Item, deleteFiles bool) error {
	e.count("remove")
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.items[it.ID]; !ok {
		return ErrNotFound
	}
	delete(e.items, it.ID)
	return nil
}

func (e *fakeEngine) Cached(ctx context.Context, hashes []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

func contentOpener(content string) OpenAt {
	return func(ctx context.Context, offset int64) (io.ReadCloser, error) {
		if offset > int64(len(content)) {
			return nil, errors.New("offset past end")
		}
		return io.NopCloser(strings.NewReader(content[offset:])), nil
	}
}

// memStorage keeps files in memory and records .part sizes.
type memStorage struct {
	mu      sync.Mutex
	id      string
	files   map[string][]byte
	parts   map[string][]byte
	free    int64
	putErr  error
	adopted map[string]string
}

func newMemStorage(id string) *memStorage {
	return &memStorage{id: id, files: map[string][]byte{}, parts: map[string][]byte{}, free: 1 << 40, adopted: map[string]string{}}
}

func (s *memStorage) ID() string                              { return s.id }
func (s *memStorage) Label() string                           { return s.id }
func (s *memStorage) Root() string                            { return "/mem/" + s.id }
func (s *memStorage) Free(ctx context.Context) (int64, error) { return s.free, nil }
func (s *memStorage) Partial(ctx context.Context, rel string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.parts[rel])), nil
}
func (s *memStorage) Put(ctx context.Context, rel string, size int64, open OpenAt, progress func(int64)) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.mu.Lock()
	off := int64(len(s.parts[rel]))
	s.mu.Unlock()
	rc, err := open(ctx, off)
	if err != nil {
		return err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data := append(s.parts[rel], b...)
	delete(s.parts, rel)
	s.files[rel] = data
	if progress != nil {
		progress(int64(len(data)))
	}
	return nil
}
func (s *memStorage) Adopt(ctx context.Context, localPath, rel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adopted[rel] = localPath
	return nil
}
func (s *memStorage) Exists(ctx context.Context, rel string, size int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.files[rel]
	return ok && (size < 0 || int64(len(b)) == size), nil
}
func (s *memStorage) Remove(ctx context.Context, rel string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.parts, rel)
	return nil
}

type recordingNotifier struct {
	mu    sync.Mutex
	calls []string
}

func (n *recordingNotifier) Notify(ctx context.Context, title, message string, failed bool) {
	n.mu.Lock()
	n.calls = append(n.calls, title)
	n.mu.Unlock()
}
