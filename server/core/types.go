// Package core holds the vendor-free domain model: sources, engines, storages,
// results, payloads and jobs. Adapters live in sibling packages and only depend
// on this one.
package core

import (
	"context"
	"io"
	"time"
)

// Canonical is the app-wide category vocabulary. Every Source maps its own
// taxonomy onto these so filters behave the same across sources.
type Canonical string

const (
	CatMovies   Canonical = "movies"
	CatTV       Canonical = "tv"
	CatAnime    Canonical = "anime"
	CatMusic    Canonical = "music"
	CatBooks    Canonical = "books"
	CatGames    Canonical = "games"
	CatSoftware Canonical = "software"
	CatOther    Canonical = "other"
)

// Categories lists every Canonical in display order.
var Categories = []Canonical{CatMovies, CatTV, CatAnime, CatMusic, CatBooks, CatGames, CatSoftware, CatOther}

// Language tags detected from a release title.
type Language string

const (
	LangFR     Language = "fr"
	LangMulti  Language = "multi"
	LangVOSTFR Language = "vostfr"
	LangEN     Language = "en"
)

// Languages lists every Language in display order.
var Languages = []Language{LangFR, LangMulti, LangVOSTFR, LangEN}

// Indexer is one upstream tracker inside a Source: the unit of status and
// latency in the UI. ID is namespaced "<sourceID>:<upstreamID>".
type Indexer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Private bool   `json:"private"`
}

// Query is a search request.
type Query struct {
	Text       string
	Categories []Canonical
	Indexers   []string // namespaced indexer IDs; empty = all
	Limit      int      // per indexer
}

// Result is one torrent as seen by one Indexer. ID is assigned by the result
// cache, not by the Source.
type Result struct {
	ID         string      `json:"id"`
	Source     string      `json:"-"`
	Indexer    string      `json:"indexer"`
	Title      string      `json:"title"`
	Size       int64       `json:"size"`
	Seeders    int         `json:"seeders"`
	Leechers   int         `json:"leechers"`
	Grabs      int         `json:"grabs"`
	Published  time.Time   `json:"published"`
	InfoHash   string      `json:"infoHash,omitempty"`
	HasMagnet  bool        `json:"hasMagnet"`
	HasTorrent bool        `json:"hasTorrent"`
	Freeleech  bool        `json:"freeleech"`
	Categories []Canonical `json:"categories"`
	Language   Language    `json:"language,omitempty"`
	InfoURL    string      `json:"infoUrl,omitempty"`

	// Ref is opaque data the Source needs to Fetch this result again
	// (a download URL, an upstream GUID). Never sent to the client.
	Ref map[string]string `json:"-"`
}

// SearchEvent is one batch from one Indexer. Done marks the indexer's last
// event; Err (with Done) marks a failed indexer.
type SearchEvent struct {
	Indexer Indexer
	Results []Result
	Done    bool
	Err     error
	Elapsed time.Duration
}

// Payload is what an Engine can ingest: a magnet URI and/or .torrent bytes.
type Payload struct {
	Name     string
	InfoHash string
	Magnet   string
	Torrent  []byte
	Size     int64
	Files    []FileEntry // from the .torrent when available
}

// FileEntry describes one file inside a torrent before it is fetched.
type FileEntry struct {
	Index int    `json:"index"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
}

// Source produces Results and can turn one into a Payload.
type Source interface {
	ID() string
	Name() string
	Indexers(ctx context.Context) ([]Indexer, error)
	// Search emits one SearchEvent per indexer batch and returns when every
	// indexer is Done or ctx is cancelled. emit must not be called after return.
	Search(ctx context.Context, q Query, emit func(SearchEvent)) error
	Fetch(ctx context.Context, r Result) (Payload, error)
}

// EngineCaps advertises what an Engine can do.
type EngineCaps struct {
	Cached      bool `json:"cached"`      // Cached() works
	Select      bool `json:"select"`      // needs Select() after Add before fetching starts
	DirectLinks bool `json:"directLinks"` // File.DirectURL works
	LocalFiles  bool `json:"localFiles"`  // File.LocalPath is set once ready
	SavePath    bool `json:"savePath"`    // AddOpts.SavePath is honoured
}

// AddOpts tunes Engine.Add.
type AddOpts struct {
	SavePath string // engine-side path, already mapped; only if Caps.SavePath
	Files    []int  // file indexes to fetch; nil = all
	Label    string
}

// Item is an engine-side torrent.
type Item struct {
	ID       string
	Name     string
	InfoHash string
	Size     int64
}

// ItemState is the engine-side lifecycle, mapped by every adapter.
type ItemState string

const (
	ItemQueued           ItemState = "queued"
	ItemWaitingSelection ItemState = "waitingSelection"
	ItemFetching         ItemState = "fetching"
	ItemReady            ItemState = "ready"
	ItemFailed           ItemState = "failed"
)

// ItemStatus is a snapshot of an Item.
type ItemStatus struct {
	State    ItemState
	Progress float64 // 0..1
	Speed    int64   // bytes/s, 0 when unknown
	ETA      int64   // seconds, 0 when unknown
	Error    string
}

// OpenAt opens a read stream starting at offset (Range semantics).
type OpenAt func(ctx context.Context, offset int64) (io.ReadCloser, error)

// File is one file of a ready Item. Exactly one of Open / LocalPath is usable
// for copying; DirectURL is optional (Caps.DirectLinks).
type File struct {
	Index     int
	Path      string // relative, as the torrent names it
	Size      int64
	LocalPath string // Caps.LocalFiles: absolute path on this host after PathMap
	Open      OpenAt
	DirectURL func(ctx context.Context) (string, error)
}

// Engine turns a Payload into readable files.
type Engine interface {
	ID() string
	Name() string
	Caps() EngineCaps
	Add(ctx context.Context, p Payload, o AddOpts) (Item, error)
	Status(ctx context.Context, it Item) (ItemStatus, error)
	Select(ctx context.Context, it Item, files []int) error
	Files(ctx context.Context, it Item) ([]File, error)
	List(ctx context.Context) ([]Item, error)
	Remove(ctx context.Context, it Item, deleteFiles bool) error
	Cached(ctx context.Context, hashes []string) (map[string]bool, error)
}

// Storage is a directory root the app may write into.
type Storage interface {
	ID() string
	Label() string
	Root() string
	Free(ctx context.Context) (int64, error)
	// Partial reports how many bytes of rel are already in the .part file.
	Partial(ctx context.Context, rel string) (int64, error)
	// Put writes rel atomically: resumes from Partial, renames at the end.
	Put(ctx context.Context, rel string, size int64, open OpenAt, progress func(written int64)) error
	// Adopt moves an existing local file into the storage (rename or hardlink
	// when on the same filesystem, else copy).
	Adopt(ctx context.Context, localPath, rel string) error
	Exists(ctx context.Context, rel string, size int64) (bool, error)
	Remove(ctx context.Context, rel string) error
}

// Mode is how a job delivers files.
type Mode string

const (
	ModeCopy  Mode = "copy"  // stream from the engine into the storage
	ModeAdopt Mode = "adopt" // engine wrote into the storage root already; rename/hardlink
	ModeLinks Mode = "links" // keep on the engine; expose links
)

// Notifier receives job outcomes.
type Notifier interface {
	Notify(ctx context.Context, title, message string, failed bool)
}
