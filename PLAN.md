# TorrentTrackerBrowser: plan

Status: draft v2 for review, 2026-09-24. Revised after an independent architecture review.
Nothing is built yet.

## What it is

A self-hosted web app to **search torrents across many sources, pick a result (or paste a
magnet / drop a .torrent), and have it fetched to a storage of your choice**. It is not tied to
any tracker aggregator, debrid service, torrent client or filesystem: each is a pluggable adapter
behind a small interface, and the running set is declared in a config file. The UI never names a
vendor; it renders whatever the backend reports as available.

Design rule: **composition over inheritance, small interfaces, config-driven registry.** One
adapter per interface ships first; the interfaces exist so the second adapter is a new file, not
a refactor. Scope is cut to what a single home user needs; see Non-goals.

## Domain model

```
Source     where results come from: an indexer aggregator, a single torznab feed, a DHT index
Indexer    one upstream tracker inside a Source (the unit of status and latency)
Result     one torrent as seen by one Indexer; server-owned, addressed by a short-lived ID
Payload    what an Engine can ingest: a magnet URI or .torrent bytes (Source.Fetch produces it)
Engine     turns a Payload into readable files: a debrid service, a torrent client
Storage    a directory root the app may write into (mounted NAS share, local disk)
Job        Payload × Engine × Storage (or "links only" delivery mode), with a state machine
```

### Interfaces (Go, package core)

```go
type Source interface {
    ID() string; Name() string
    Indexers(ctx) ([]Indexer, error)                       // id, name, private bool, latency hint
    Search(ctx, Query, emit func(SearchEvent)) error       // one event per indexer batch; Err/Done per indexer
    Fetch(ctx, Result) (Payload, error)                    // magnet or .torrent bytes (through the aggregator when needed)
}
// Query{Text, Categories []Canonical, Indexers []string, Limit}
// SearchEvent{Indexer IndexerRef, Results []Result, Err error, Done bool}
// Result{ID, Indexer, Title, Size, Seeders, Leechers, Grabs, Published, InfoHash?, Freeleech, Canonical []Canonical, Raw categories}

type Engine interface {
    ID() string; Name() string
    Caps() EngineCaps                                      // Cached, Select, DirectLinks, LocalFiles, SavePath
    Add(ctx, Payload, AddOpts) (Item, error)               // AddOpts{SavePath, Files []int, Label}
    Status(ctx, Item) (ItemStatus, error)                  // state, pct, speed, eta, error
    Select(ctx, Item, files []int) error                   // no-op unless Caps.Select (Real-Debrid style)
    Files(ctx, Item) ([]File, error)                       // File{Path, Size, LocalPath?, Open(ctx, offset), DirectURL(ctx)}
    List(ctx) ([]Item, error)                              // restart reconciliation + external items in the queue
    Remove(ctx, Item, deleteFiles bool) error
    Cached(ctx, hashes []string) (map[string]bool, error)  // only if Caps.Cached
}

type Storage interface {
    ID() string; Label() string; Root() string
    Free(ctx) (int64, error)
    Partial(ctx, rel string) (int64, error)                // bytes already in <rel>.part, 0 if none
    Put(ctx, rel string, size int64, open OpenAt, progress func(int64)) error   // resumable, atomic rename
    Adopt(ctx, localPath, rel string) error                // rename or hardlink when same fs, else copy
    Exists(ctx, rel string, size int64) (bool, error)
    Remove(ctx, rel string) error                          // cancel cleanup of .part
}
```

Canonical categories live in core (`Movies, TV, Anime, Music, Books, Games, Software, Other`);
each Source maps its own taxonomy onto them so category chips are consistent across sources.

Adapters planned (each one package):

| Interface | First | Next |
|---|---|---|
| Source | Prowlarr (`/api/v1/search` fanned out per indexer) | Jackett, raw Torznab URL, Bitmagnet |
| Engine | TorBox (debrid) | qBittorrent (client, SavePath + LocalFiles), Real-Debrid (Select), Transmission |
| Storage | Path (a whitelisted root, subfolders on demand) | SFTP |

Dropped on review: WebDAV/S3 (no offset resume), debrid search APIs as Sources (cross-adapter
coupling), hoster-link engines (not torrents), "browser" as a Storage.

### Job pipeline

```
result ID (or pasted payload) ──► Source.Fetch ──► Engine.Add(payload, {SavePath?, Files?})
      ──► poll Status ──► [WaitingSelection → Select]
      ──► delivery:
            copy   : for each File: Storage.Partial → Put(open at offset) → rename   (remote engines)
            adopt  : Storage.Adopt(LocalPath)                                          (client engines with SavePath: rename/hardlink, no second write)
            links  : keep on engine, UI shows DirectURL resolved lazily at click time    ("Mon PC" mode)
```

States: `queued, adding, waitingSelection, fetching, ready, copying, done, failed, cancelled`.
Rules: cancel removes the engine item only if the app created it; delete never deletes files on a
client engine unless asked; retryable errors are network/5xx/expired-link; a copy failure keeps the
engine item; debrid cleanup after copy is a per-engine config toggle (default off).

Job state is a small JSON file, written debounced on change (a few KB), so `.part` resume and
queue survive a restart. Engine `List` reconciles on startup.

### Safety (no-login LAN app)

- Bind to one interface; Host-header allowlist (DNS rebinding); mutating routes require
  `Content-Type: application/json` plus a custom header (form-POST CSRF); optional shared token in
  config.
- The server never fetches a client-supplied URL: jobs reference result IDs or an uploaded
  payload; only `Source.Fetch` and engine adapters make outbound calls.
- Every `rel` and subfolder is `filepath.Clean`-ed, rejects `..`/absolute, and the final path is
  verified under the root after `EvalSymlinks`.
- Free-space preflight before a copy; secrets never appear in `/api/capabilities` or logs.
- Limits: max concurrent jobs, max parallel files per job, per-engine rate limits, per-indexer
  timeouts, results per indexer, TTL and cap on the result cache and the job list.

## Backend

Go, single static binary, embeds the built frontend. Result cache in memory with TTL; job file on
disk; logs info/warn. Container runs as the UID/GID the NAS export allows (config, documented).

| Route | Purpose |
|---|---|
| `GET /api/capabilities` | sources + their indexers, engines + caps, storages (+ free space), defaults, canonical categories |
| `GET /api/search?q=&cat=&indexers=` | SSE: `batch` events per indexer (results, error, done), heartbeats, `done`; aborts fan-out on disconnect |
| `POST /api/payloads` | pasted magnet or uploaded .torrent → payload ID |
| `GET /api/results/{id}/files` | preview from .torrent bytes (bencode); for cached hashes, from the engine |
| `GET /api/results/{id}/payload` | copy magnet / download the .torrent to the user's machine |
| `POST /api/cached` | `{engine, hashes[]}` |
| `POST /api/jobs` | `{resultId \| payloadId, engine, storage?, subdir?, files?, mode: copy\|adopt\|links}` |
| `GET /api/jobs`, `POST /api/jobs/{id}/cancel`, `/retry`, `DELETE` | jobs + external engine items (`external: true`), one row model |
| `POST /api/engines/{id}/items/{item}/jobs` | send an external engine item to a storage (creates a job in `copying`) |
| `GET /api/jobs/{id}/files/{path}` | Range-capable stream with Content-Disposition (links mode without DirectURL) |
| `GET /healthz` | |

SSE details: comment heartbeats every 15 s, `Last-Event-ID` ignored (a reconnect restarts the
search), the UI closes the previous EventSource before opening a new one, search cancel = close.

### Config (YAML, `${VAR}` env expansion, validated at startup, no hot reload)

```yaml
listen: 0.0.0.0:8080
allowedHosts: [torrents.example.lan, 192.168.1.26]
sources:
  - { id: prowlarr, type: prowlarr, url: http://prowlarr:9696, apiKey: ${PROWLARR_KEY} }
engines:
  - { id: torbox, type: torbox, apiKey: ${TORBOX_KEY}, removeAfterCopy: false }
  - { id: qbit, type: qbittorrent, url: http://qbit:8080, username: ${QBIT_USER}, password: ${QBIT_PASS},
      pathMap: { "/mnt/media": "/data" } }
storages:
  - { id: downloads, type: path, label: Téléchargements, root: /mnt/media/downloads/manual }
defaults: { engine: torbox, storage: downloads, mode: copy }
limits: { jobs: 2, filesPerJob: 2, resultsPerIndexer: 100, indexerTimeout: 20s, resultTTL: 30m }
notify: { webhook: "", lang: en }     # lang: language of notification titles (en | fr)
```

## Frontend

SvelteKit with `adapter-static` (shadcn-svelte's CLI and docs assume that layout), Svelte 5,
Tailwind v4, shadcn-svelte, mode-watcher. French UI strings, English code.

- **Search**: query box, canonical category chips, indexer chips (from capabilities), results
  table streaming in with a per-indexer status strip; columns: title, indexer, size, seeders,
  leechers, grabs, age, freeleech, language tag (FR / MULTI / VOSTFR / EN, hardcoded regexes),
  cached badge when an engine supports it. Rows sharing an infoHash are one row with indexer
  badges. Row actions: download, copy magnet, get .torrent, preview files. Cancel button while
  streaming.
- **Filters/sort**: category, indexer, min seeders, size range, age, language, cached-only;
  sort by seeders, size, date. Persisted in localStorage.
- **Download dialog**: engine, storage (with free space), subfolder, per-file selection when a
  preview is available, delivery mode. One click when defaults suffice. Bulk: same dialog for
  several selected rows.
- **Paste/drop**: magnet text or .torrent file → same dialog.
- **Queue**: jobs with state, progress, speed, ETA, per-file status, cancel/retry/delete, links
  in `links` mode; plus engine-side items the app did not create, flagged "externe", with
  send-to-storage and delete. Polled every 3 s while open.
- Later: history and saved searches (localStorage), keyboard navigation, mobile layout, dark mode.

## Repository layout

```
/web              SvelteKit app (adapter-static)
/server           Go module
  /core           types, interfaces, registry, config, job runner + state file, search fan-out, SSE, safety
  /source/prowlarr
  /engine/torbox  /engine/qbittorrent
  /storage/path
  /api
  main.go
/deploy           compose.yaml (prod), config.example.yaml, reverse-proxy snippet
compose.yaml      dev: web (node, Vite HMR) + api (Go hot reload), .env for secrets
Dockerfile        multi-stage: node build → go build with embed → static final image
PLAN.md
```

Dev: `docker compose up`, UI on http://localhost:5173, `/api` proxied to the api container.
Adapters point at real services over the network; the Path storage points at a local folder.

## Milestones

1. **Skeleton + image**: core types, registry, config, `/healthz`, `/api/capabilities`, SvelteKit
   shell served by Go, Dockerfile and prod compose. Deployed to the target host from here on.
2. **Search**: Prowlarr source (indexer enumeration, per-indexer fan-out, `.torrent` fetch),
   SSE, results table, sort, status strip, cancel. Read-only tool.
3. **First download (first release)**: TorBox engine, Path storage, job runner with state file,
   path safety, mutation guards, free-space preflight, queue panel, copy magnet / get .torrent.
4. **Selection and input**: `.torrent` preview (bencode), per-file selection, magnet paste and
   .torrent upload, cached badge + cached-only filter, `links` mode with lazy DirectURL and
   Range streaming, webhook notification.
5. **Second engine**: qBittorrent with SavePath + Adopt (rename/hardlink), pathMap. Proves the
   Engine and Storage interfaces on a client-type engine.
6. **Polish**: history and saved searches, keyboard navigation, mobile, design pass and
   accessibility audit, bulk download.

Each milestone ends with adapter unit tests against recorded HTTP fixtures and one manual
end-to-end check. Milestone 3 adds an integration test for the copy path: traversal names,
resume from a truncated `.part`, ENOSPC.

## Non-goals

Multi-user accounts, a database, RSS/automation rules, media-library management, seeding
management, a public deployment, fuzzy result merging (title/size heuristics).

## Decisions taken on review (2026-09-24)

- Results are **grouped by infoHash only**: identical hashes from several indexers become one row
  with indexer badges and the best seeders. No fuzzy title matching. Hash-less results stay
  separate rows.
- The queue **also shows engine-side items the app did not create**, flagged "externe", with the
  same send-to-storage and delete actions (a debrid account view). Internally they are `Item`s
  without a `Job`; the UI renders both through one row model (state, progress, files, actions).
- Job completion/failure posts to an **ntfy-compatible webhook URL**; empty disables it.
