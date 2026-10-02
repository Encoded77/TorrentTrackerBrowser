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
    { "id": "downloads", "label": "Téléchargements", "free": 9000000000000, "scanned": true }  // scanned: a scanner covers it
  ],
  "categories": [ "movies", "tv", "anime", "music", "books", "games", "software", "other" ],
  "defaults": { "engine": "torbox", "storage": "downloads", "mode": "copy" },
  "languages": [ "fr", "multi", "vostfr", "en" ],
  "scanner": true,                                // a malware scanner is configured
  "user": "ann" | null,                           // users.header value; null = operator
  "shareStorage": "downloads" | null              // finished jobs of this storage can be shared; null = off
}
```

Indexer ids are namespaced by source (`<sourceId>:<upstreamId>`). For a named user (see Users),
`storages` lists only `users.storages` and `defaults.storage` falls back to the first of them.

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
  "state": "queued" | "adding" | "waitingSelection" | "fetching" | "ready" | "copying" | "scanning" | "done" | "infected" | "failed" | "cancelled",
  "progress": 0.42, "speed": 12345678 | null, "eta": 120 | null,  // fraction, B/s, seconds
  "error": "…" | null, "retryable": true,
  "files": [ { "path": "Dune/dune.mkv", "size": 123, "done": 123, "state": "pending"|"copying"|"done"|"failed"|"skipped"|"quarantined", "url": "/api/jobs/j_…/files/Dune/dune.mkv" | null } ],
  "external": false,
  "owner": "ann" | null,                          // users.header value of the creator; null = operator
  "scan": null | {                                // null when no scan ran
    "status": "clean" | "infected" | "skipped" | "error",
    "findings": [ { "path": "Game/crack.exe", "status": "infected", "signature": "Win.Trojan…",
                    "reason": "…", "quarantine": ".quarantine/j_…/Game/crack.exe" } ],  // non-clean files only
    "scannedAt": "…"
  }
}
```

When a scanner covers the job's storage, a copy/adopt job goes `copying -> scanning -> done | infected`.
Infected files are moved to `<storage>/.quarantine/<job id>/<path>`, the engine item this app created is
removed with its data, and the webhook gets a priority-5 message. A scanner failure ends the job `done`
with `scan.status = "error"` and a priority-4 message; files over the scanner's size limit are `skipped`,
except ZIP/RAR/7z archives: their entries are scanned one by one (an infected entry quarantines the whole
archive; encrypted, multi-part or damaged archives and oversized entries are `skipped`).
Links-mode jobs are never scanned.

External engine items (not created by this app) appear in `GET /api/jobs` with `external: true`,
`id: "x_<engine>_<itemId>"`, `state` mapped from the engine, no `storage`, and the same `files`
shape. Actions on them: `DELETE` (removes from the engine), and
`POST /api/jobs/{id}/send { "storage", "subdir", "files", "mode" }` which creates a real job in
`copying`.

- `GET /api/jobs` → `{ "jobs": [ Job ] }` (own jobs newest first, then external).
- `POST /api/jobs/{id}/cancel` → Job. Stops copying, removes the engine item if this app created it.
- `POST /api/jobs/{id}/retry` → Job (only when `retryable`).
- `POST /api/jobs/{id}/rescan` → Job. Only for a `done` copy/adopt job whose `scan` is null, `skipped`
  or `error`, on a storage covered by a scanner; else `409 bad_state`. `cancel` also answers
  `409 bad_state` while the job is `scanning`.
- `DELETE /api/jobs/{id}` → `204`. Query `deleteFiles=true` also deletes on client-type engines.
  Query `purge=true` (cancelled or infected jobs only, else `409 bad_state`) also deletes the files the
  job delivered to its storage (quarantined ones included, folders left empty too, never files marked
  `skipped`) and its engine item with data; a file that cannot be deleted answers `502 storage_error`
  and keeps the job.
- `POST /api/jobs/{id}/share { "days": 7, "password": "…" }` → `{ "url": "…" }`. Only when `share.url`
  is configured (else `404`), for a `done` job delivered to `share.storage` (else `409 bad_state`);
  `days` 0 = never expires, max 365. The server POSTs
  `{ "paths": [delivered paths, relative to the storage], "name", "description", "days", "password" }`
  to `share.url`, which answers `{ "url" }` (any other answer: `502 share_error` with its `error`).
- `GET /api/jobs/{id}/files/{path}` → streams the file (Range supported, `Content-Disposition`).
  In `links` mode with `caps.directLinks`, answers `302` to a freshly resolved engine link.

## Users

With `users.header` set, the reverse proxy authenticates and names the user in that header (it must
overwrite any client-sent value). A request without it is the operator and sees everything. A named
user only gets their own jobs in `GET /api/jobs` (no external items), every other job id answers
`404`, `POST /api/jobs` sets `owner` and refuses a storage outside `users.storages` with
`403 storage_not_allowed`.

## Notifications

On `done`, `infected` or `failed`, if `notify.webhook` is set, the server POSTs (titles in `notify.lang`, `en` or `fr`)
the message as a plain-text body with `Title`, `Priority` (3; 4 on failure or incomplete scan; 5 on malware)
and `Tags` (`torrent`, plus `warning` on malware) headers,
the ntfy topic-URL convention (so the URL may carry ntfy's `?auth=` parameter).
