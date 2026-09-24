# HTTP API contract

Shared contract between `server/` (Go) and `web/` (SvelteKit). Both sides are built against this
file; change it here first. All JSON, all times RFC 3339 UTC, all sizes in bytes.

Mutating routes (`POST`, `DELETE`) require `Content-Type: application/json` and the header
`X-Requested-With: TorrentTrackerBrowser`; otherwise `403`. Errors are
`{ "error": { "code": "string", "message": "string" } }` with a 4xx/5xx status.

## GET /healthz

`200 {"ok":true}`.

## GET /api/capabilities

```jsonc
{
  "sources": [
    { "id": "prowlarr", "name": "Prowlarr",
      "indexers": [ { "id": "prowlarr:16", "name": "TR4KER", "private": true } ] }
  ],
  "engines": [
    { "id": "torbox", "name": "TorBox",
      "caps": { "cached": true, "select": false, "directLinks": true, "localFiles": false, "savePath": false } }
  ],
  "storages": [
    { "id": "downloads", "label": "Téléchargements", "free": 9000000000000 }
  ],
  "categories": [ "movies", "tv", "anime", "music", "books", "games", "software", "other" ],
  "defaults": { "engine": "torbox", "storage": "downloads", "mode": "copy" },
  "languages": [ "fr", "multi", "vostfr", "en" ]
}
```

Indexer ids are namespaced by source (`<sourceId>:<upstreamId>`).

## GET /api/search

Query: `q` (required), `cat` (comma-separated canonical categories, optional), `indexers`
(comma-separated indexer ids, optional; default all). Response is `text/event-stream`.

Events, each `data:` a JSON object; `id:` is a counter; a `: ping` comment every 15 s.

```jsonc
// event: batch  (one per indexer; several may arrive for the same indexer if it pages)
{ "indexer": "prowlarr:16", "results": [ Result ], "done": true, "error": null, "elapsedMs": 234 }
// event: done   (after every indexer reported done or error)
{ "total": 412, "elapsedMs": 6120 }
```

`Result`:

```jsonc
{
  "id": "r_8f3a…",             // server-side id, valid for resultTTL (30 min)
  "indexer": "prowlarr:16",
  "title": "Dune Part Two 2024 MULTI 1080p WEB H265-XYZ",
  "size": 4812345678,
  "seeders": 120, "leechers": 4, "grabs": 31,
  "published": "2026-05-15T00:01:24Z",
  "infoHash": "0723937b…" | null,
  "hasMagnet": true, "hasTorrent": true,
  "freeleech": false,
  "categories": [ "movies" ],
  "language": "multi" | "fr" | "vostfr" | "en" | null,
  "infoUrl": "https://…" | null
}
```

Closing the EventSource cancels the fan-out server-side.

## POST /api/payloads

Body: `{ "magnet": "magnet:?xt=…" }` **or** multipart with a `torrent` file field.
`201 { "id": "p_…", "name": "…", "infoHash": "…", "size": 123 | null, "files": [ File ] | null }`.

## GET /api/results/{id}/files

`200 { "files": [ { "path": "Dune/dune.mkv", "size": 123, "index": 0 } ] }` from the `.torrent`
(bencode) or, when the default engine reports the hash cached, from the engine.
`409 { code: "no_preview" }` when neither is possible.

## GET /api/results/{id}/payload

`?as=magnet` → `200 text/plain` magnet URI. `?as=torrent` → `200 application/x-bittorrent` with
`Content-Disposition`. `404` when the source cannot provide that form.

## POST /api/cached

`{ "engine": "torbox", "hashes": [ "…" ] }` → `200 { "cached": { "<hash>": true } }`. Engines
without `caps.cached` answer `400`.

## Jobs

```jsonc
// POST /api/jobs
{ "resultId": "r_…" | null, "payloadId": "p_…" | null,     // exactly one
  "engine": "torbox", "storage": "downloads" | null,          // storage required unless mode = links
  "subdir": "Dune" | null, "files": [0, 3] | null,            // null = all files
  "mode": "copy" | "adopt" | "links" }
// 201 → Job
```

`Job`:

```jsonc
{
  "id": "j_…", "createdAt": "…", "updatedAt": "…",
  "name": "Dune Part Two 2024 …", "infoHash": "…" | null,
  "engine": "torbox", "storage": "downloads" | null, "subdir": "Dune" | null, "mode": "copy",
  "state": "queued" | "adding" | "waitingSelection" | "fetching" | "ready" | "copying" | "done" | "failed" | "cancelled",
  "progress": 0.42, "speed": 12345678 | null, "eta": 120 | null,  // fraction, B/s, seconds
  "error": "…" | null, "retryable": true,
  "files": [ { "path": "Dune/dune.mkv", "size": 123, "done": 123, "state": "pending"|"copying"|"done"|"failed"|"skipped", "url": "/api/jobs/j_…/files/Dune/dune.mkv" | null } ],
  "external": false
}
```

External engine items (not created by this app) appear in `GET /api/jobs` with `external: true`,
`id: "x_<engine>_<itemId>"`, `state` mapped from the engine, no `storage`, and the same `files`
shape. Actions on them: `DELETE` (removes from the engine), and
`POST /api/jobs/{id}/send { "storage", "subdir", "files", "mode" }` which creates a real job in
`copying`.

- `GET /api/jobs` → `{ "jobs": [ Job ] }` (own jobs newest first, then external).
- `POST /api/jobs/{id}/cancel` → Job. Stops copying, removes the engine item if this app created it.
- `POST /api/jobs/{id}/retry` → Job (only when `retryable`).
- `DELETE /api/jobs/{id}` → `204`. Query `deleteFiles=true` also deletes on client-type engines.
- `GET /api/jobs/{id}/files/{path}` → streams the file (Range supported, `Content-Disposition`).
  In `links` mode with `caps.directLinks`, answers `302` to a freshly resolved engine link.

## Notifications

On `done` or `failed`, if `notify.webhook` is set, the server POSTs (titles in `notify.lang`, `en` or `fr`)
the message as a plain-text body with `Title`, `Priority` (3, or 4 on failure) and `Tags: torrent` headers,
the ntfy topic-URL convention (so the URL may carry ntfy's `?auth=` parameter).
