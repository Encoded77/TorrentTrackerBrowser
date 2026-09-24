# TorrentTrackerBrowser web frontend

SvelteKit 2 + Svelte 5 (runes) + Tailwind v4 + shadcn-svelte, built as a static SPA
(`adapter-static`, `ssr = false`) that the Go server embeds and serves. The HTTP contract is
`../API.md`; the typed client in `src/lib/api/` mirrors it exactly.

## Run

Node 24 is expected. On this machine it lives in `C:\Program Files\nodejs`; add it to `PATH`
if it is not there already.

| Command | What it does |
| --- | --- |
| `npm install` | install dependencies |
| `npm run dev` | Vite dev server on http://localhost:5173, `/api` and `/healthz` proxied to the Go server (`API_URL`, default `http://localhost:8080`) |
| `npm run dev:mock` | same, but with the in-memory mock backend from `mock/` instead of the Go server (`vite dev --mode mock`; `VITE_MOCK=1 vite dev` works too) |
| `npm run check` | `svelte-check` type check |
| `npm run build` | production build into `build/` (embedded by the server) |
| `npm run preview` | serve the production build locally |

## Mock backend

`mock/plugin.ts` is a Vite dev plugin that implements every route of `API.md` in memory:

- two sources (Prowlarr with TR4KER, C411, YTS; a Torznab feed with Nyaa), with latencies of
  200 ms (paged in two batches), 400 ms, 2 s (fails with a 502) and 6 s;
- SSE search on the real timing, results grouped across indexers by infoHash, a 30 min result
  TTL, `/files` previews (409 `no_preview` for some releases), magnet/.torrent payloads;
- two engines (TorBox with `cached`, qBittorrent with `localFiles`/`savePath`), two storages;
- `POST /api/cached` says a hash is cached when its first hex digit is even;
- a jobs list seeded with done/copying/failed/links jobs plus two external items, that
  advances every second; created jobs go queued, adding, fetching, copying, done;
- mutation guard (`X-Requested-With` + `Content-Type`), the error envelope, `/healthz`.

Nothing in `mock/` ships: it is only loaded when the mock mode is on.

## Layout

```
src/lib/api/         types.ts (API.md shapes), client.ts (fetch wrappers), sse.ts (search stream)
src/lib/state/       app.svelte.ts (AppState: capabilities, search, filters, selection, jobs, history),
                     persist.ts (guarded localStorage), actions.ts (row actions)
src/lib/components/app/   Header, SearchBar, IndexerPicker, FiltersPopover, HistoryChips, StatusStrip,
                     ResultsTable, ResultCard (phone), RowActions, RowBadges, FilePreview,
                     DownloadDialog, PayloadDialog, QueueSheet, JobCard, EmptyState
src/lib/components/ui/    shadcn-svelte components (style "nova", zinc)
src/lib/i18n/        fr.ts (reference dictionary), en.ts (typed against fr), locale.svelte.ts (active locale)
src/lib/strings.ts   `s`, the active dictionary (a Proxy over the locale state)
src/lib/format.ts    sizes, ages, durations, subfolder sanitising, language fallback
src/routes/          +layout (ModeWatcher, Toaster, Tooltip provider), +page (the single view)
```

Conventions: English code and comments, no UI string outside `src/lib/i18n/`, Svelte 5 runes
only, no stores, keyed each blocks, `$state.raw` for API result arrays.

## Languages

The UI is available in French and English. The header dropdown (languages icon) switches it
live; the choice is stored in `localStorage.locale`, and a first visit follows
`navigator.language` (fr* gives French, anything else English). Every component reads its
strings through `s` from `src/lib/strings.ts`: a Proxy whose property reads go through the
`$state` locale, so templates and `$derived` values re-render on change. Formatting
(`src/lib/format.ts`) follows the same locale: units (Go/GB), relative ages, durations, and
numbers and dates through `Intl`.

To add a language:

1. Copy `src/lib/i18n/en.ts` to `src/lib/i18n/<code>.ts` and translate every value. The file is
   typed against the French shape (`Dictionary`), so a missing or extra key fails
   `npm run check`. Keep the function signatures.
2. Add the code to `Locale` and `LOCALES` in `src/lib/i18n/locale.svelte.ts`, and its
   display name to `localeName` in every dictionary.
3. Register it in the `dictionaries` map in `src/lib/strings.ts`.

Screenshots are taken in one language at a time: `LOCALE=fr node scripts/screenshots.mjs <baseUrl>`
(default `en`); the script imports the dictionaries directly for the labels it clicks on.

Keyboard: `/` focuses the search box, `Enter` searches, `Escape` cancels a running search,
`Enter` on a focused row opens the download dialog, `Space` toggles its selection.
