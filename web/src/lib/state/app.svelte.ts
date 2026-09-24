import { SvelteSet } from 'svelte/reactivity';
import { api, errorMessage } from '$lib/api/client';
import { openSearch, type SearchStream } from '$lib/api/sse';
import type {
	BatchEvent,
	Capabilities,
	Engine,
	Indexer,
	Job,
	Payload,
	Result,
	TorrentFile
} from '$lib/api/types';
import { guessLanguage } from '$lib/format';
import { s } from '$lib/strings';
import { load, save } from './persist';

export interface IndexerStatus {
	state: 'pending' | 'done' | 'failed' | 'cancelled';
	count: number;
	elapsedMs: number;
	error: string | null;
}

export interface Filters {
	minSeeders: number;
	sizeMinGb: number | null;
	sizeMaxGb: number | null;
	maxAgeDays: number | null;
	languages: string[];
	cachedOnly: boolean;
	categories: string[];
	indexers: string[];
}

export const DEFAULT_FILTERS: Filters = {
	minSeeders: 0,
	sizeMinGb: null,
	sizeMaxGb: null,
	maxAgeDays: null,
	languages: [],
	cachedOnly: false,
	categories: [],
	indexers: []
};

export type SortKey = 'seeders' | 'size' | 'published' | 'title';
export interface Sort {
	key: SortKey;
	dir: 'asc' | 'desc';
}

/** One table row: results sharing an infoHash are merged, hash-less results stay alone. */
export interface ResultRow {
	key: string;
	primary: Result;
	sources: Result[];
	indexers: string[];
	title: string;
	size: number;
	seeders: number;
	leechers: number;
	grabs: number;
	published: string;
	infoHash: string | null;
	language: string | null;
	freeleech: boolean;
	categories: string[];
}

export type DownloadTarget =
	| { kind: 'result'; row: ResultRow; files: TorrentFile[] | null; preselected: number[] | null }
	| { kind: 'payload'; payload: Payload }
	| { kind: 'send'; job: Job };

export type FilesState = { state: 'loading' } | { state: 'error'; message: string } | { state: 'ok'; files: TorrentFile[] };

const ACTIVE_STATES = new Set(['queued', 'adding', 'waitingSelection', 'fetching', 'copying']);
const POLL_MS = 3000;
const HISTORY_MAX = 20;

export class AppState {
	// Capabilities
	caps = $state<Capabilities | null>(null);
	capsError = $state<string | null>(null);
	capsLoading = $state(true);

	// Search inputs
	query = $state('');
	categories = $state<string[]>(load('search.categories', [] as string[]));
	indexers = $state<string[]>(load('search.indexers', [] as string[]));

	// Search run
	running = $state(false);
	searchedQuery = $state<string | null>(null);
	results = $state.raw<Result[]>([]);
	indexerStatus = $state<Record<string, IndexerStatus>>({});
	total = $state(0);
	elapsedMs = $state<number | null>(null);
	searchError = $state<string | null>(null);
	#stream: SearchStream | null = null;

	// Cached lookups: engineId -> hash -> cached
	cached = $state<Record<string, Record<string, boolean>>>({});
	#cachedQueried = new Map<string, Set<string>>();

	// Table
	filters = $state<Filters>(load('filters', DEFAULT_FILTERS));
	sort = $state<Sort>(load('sort', { key: 'seeders', dir: 'desc' } as Sort));
	selection = new SvelteSet<string>();
	expandedKey = $state<string | null>(null);
	filesByResult = $state<Record<string, FilesState>>({});
	fileSelection = $state<Record<string, number[]>>({});

	// Jobs
	jobs = $state.raw<Job[]>([]);
	jobsError = $state<string | null>(null);
	jobsLoaded = $state(false);
	queueOpen = $state(false);
	#pollTimer: ReturnType<typeof setTimeout> | null = null;
	#pollInFlight = false;

	// History
	history = $state<string[]>(load('history', [] as string[]));
	saved = $state<string[]>(load('saved', [] as string[]));

	// Dialogs
	download = $state<DownloadTarget[] | null>(null);
	payloadDialogOpen = $state(false);

	// Derived
	allIndexers = $derived<Indexer[]>(this.caps?.sources.flatMap((src) => src.indexers) ?? []);
	indexerById = $derived<Map<string, Indexer>>(new Map(this.allIndexers.map((i) => [i.id, i])));
	engineById = $derived<Map<string, Engine>>(new Map((this.caps?.engines ?? []).map((e) => [e.id, e])));
	cachedEngines = $derived<Engine[]>((this.caps?.engines ?? []).filter((e) => e.caps.cached));

	rows = $derived.by<ResultRow[]>(() => groupByHash(this.results));
	visibleRows = $derived.by<ResultRow[]>(() => this.#sortRows(this.#filterRows(this.rows)));
	hiddenCount = $derived(this.rows.length - this.visibleRows.length);
	activeFilterCount = $derived.by(() => countActiveFilters(this.filters));

	activeJobs = $derived(this.jobs.filter((j) => !j.external && ACTIVE_STATES.has(j.state)));
	hasActiveJobs = $derived(this.activeJobs.length > 0);

	selectedRows = $derived(this.visibleRows.filter((r) => this.selection.has(r.key)));

	// ---- Capabilities -------------------------------------------------------

	async loadCapabilities(): Promise<void> {
		this.capsLoading = true;
		this.capsError = null;
		try {
			this.caps = await api.capabilities();
			// Drop persisted indexer selections that no longer exist.
			const known = new Set(this.caps.sources.flatMap((src) => src.indexers.map((i) => i.id)));
			this.indexers = this.indexers.filter((id) => known.has(id));
			this.filters.indexers = this.filters.indexers.filter((id) => known.has(id));
		} catch (e) {
			this.capsError = errorMessage(e, s.networkError);
		} finally {
			this.capsLoading = false;
		}
		void this.refreshJobs();
	}

	engineName(id: string): string {
		return this.engineById.get(id)?.name ?? id;
	}

	indexerName(id: string): string {
		return this.indexerById.get(id)?.name ?? id.split(':').pop() ?? id;
	}

	storageLabel(id: string | null): string {
		if (!id) return '';
		return this.caps?.storages.find((st) => st.id === id)?.label ?? id;
	}

	// ---- Search -------------------------------------------------------------

	setCategories(next: string[]) {
		this.categories = next;
		save('search.categories', next);
	}

	setIndexers(next: string[]) {
		this.indexers = next;
		save('search.indexers', next);
	}

	search(query = this.query): void {
		const q = query.trim();
		if (!q) return;
		this.cancel();
		this.query = q;
		this.searchedQuery = q;
		this.running = true;
		this.results = [];
		this.total = 0;
		this.elapsedMs = null;
		this.searchError = null;
		this.selection.clear();
		this.expandedKey = null;
		this.filesByResult = {};
		this.fileSelection = {};
		this.pushHistory(q);
		history.replaceState(null, '', `?q=${encodeURIComponent(q)}`);

		const targets = this.indexers.length
			? this.allIndexers.filter((i) => this.indexers.includes(i.id))
			: this.allIndexers;
		const status: Record<string, IndexerStatus> = {};
		for (const ix of targets) status[ix.id] = { state: 'pending', count: 0, elapsedMs: 0, error: null };
		this.indexerStatus = status;

		this.#stream = openSearch(q, this.categories, this.indexers, {
			onBatch: (b) => this.#onBatch(b),
			onDone: (d) => {
				this.total = d.total;
				this.elapsedMs = d.elapsedMs;
				this.#finish();
			},
			onError: () => {
				this.searchError = s.searchStreamLost;
				this.#finish();
			}
		});
	}

	cancel(): void {
		if (!this.#stream) return;
		this.#stream.cancel();
		this.#finish(true);
	}

	#finish(cancelled = false) {
		this.#stream = null;
		this.running = false;
		for (const st of Object.values(this.indexerStatus)) {
			if (st.state === 'pending') st.state = cancelled ? 'cancelled' : st.count ? 'done' : 'failed';
		}
	}

	#onBatch(b: BatchEvent) {
		const st = this.indexerStatus[b.indexer] ?? { state: 'pending', count: 0, elapsedMs: 0, error: null };
		st.count += b.results.length;
		st.elapsedMs = b.elapsedMs;
		if (b.error) {
			st.state = 'failed';
			st.error = b.error;
		} else if (b.done) {
			st.state = 'done';
		}
		this.indexerStatus[b.indexer] = st;
		if (b.results.length) {
			this.results = [...this.results, ...b.results];
			this.total = this.results.length;
			void this.#lookupCached(b.results);
		}
	}

	async #lookupCached(fresh: Result[]) {
		const hashes = fresh.map((r) => r.infoHash).filter((h): h is string => !!h);
		if (!hashes.length) return;
		await Promise.all(
			this.cachedEngines.map(async (engine) => {
				let asked = this.#cachedQueried.get(engine.id);
				if (!asked) {
					asked = new Set();
					this.#cachedQueried.set(engine.id, asked);
				}
				const todo = [...new Set(hashes.filter((h) => !asked!.has(h)))];
				if (!todo.length) return;
				todo.forEach((h) => asked!.add(h));
				try {
					const res = await api.cached(engine.id, todo);
					this.cached[engine.id] = { ...(this.cached[engine.id] ?? {}), ...res.cached };
				} catch {
					// unknown stays unknown; the hashes can be asked again next search
					todo.forEach((h) => asked!.delete(h));
				}
			})
		);
	}

	/** Engines reporting the hash as cached. */
	cachedEnginesFor(hash: string | null): Engine[] {
		if (!hash) return [];
		return this.cachedEngines.filter((e) => this.cached[e.id]?.[hash] === true);
	}

	/** true / false / null (not asked or unknown). */
	cachedOn(engineId: string, hash: string | null): boolean | null {
		if (!hash) return null;
		const v = this.cached[engineId]?.[hash];
		return v === undefined ? null : v;
	}

	// ---- Filters, sort, selection ------------------------------------------

	setFilters(patch: Partial<Filters>) {
		this.filters = { ...this.filters, ...patch };
		save('filters', this.filters);
	}

	resetFilters() {
		this.filters = { ...DEFAULT_FILTERS };
		save('filters', this.filters);
	}

	setSort(key: SortKey) {
		if (this.sort.key === key) {
			this.sort = { key, dir: this.sort.dir === 'desc' ? 'asc' : 'desc' };
		} else {
			this.sort = { key, dir: key === 'title' ? 'asc' : 'desc' };
		}
		save('sort', this.sort);
	}

	#filterRows(rows: ResultRow[]): ResultRow[] {
		const f = this.filters;
		const now = Date.now();
		const minSize = f.sizeMinGb != null ? f.sizeMinGb * 1e9 : null;
		const maxSize = f.sizeMaxGb != null ? f.sizeMaxGb * 1e9 : null;
		const maxAge = f.maxAgeDays != null ? f.maxAgeDays * 86_400_000 : null;
		return rows.filter((r) => {
			if (r.seeders < f.minSeeders) return false;
			if (minSize != null && r.size < minSize) return false;
			if (maxSize != null && r.size > maxSize) return false;
			if (maxAge != null && now - Date.parse(r.published) > maxAge) return false;
			if (f.languages.length && !f.languages.includes(r.language ?? '')) return false;
			if (f.categories.length && !r.categories.some((c) => f.categories.includes(c))) return false;
			if (f.indexers.length && !r.indexers.some((i) => f.indexers.includes(i))) return false;
			if (f.cachedOnly && this.cachedEnginesFor(r.infoHash).length === 0) return false;
			return true;
		});
	}

	#sortRows(rows: ResultRow[]): ResultRow[] {
		const { key, dir } = this.sort;
		const sign = dir === 'asc' ? 1 : -1;
		const collator = new Intl.Collator('fr', { sensitivity: 'base', numeric: true });
		return [...rows].sort((a, b) => {
			let c = 0;
			switch (key) {
				case 'seeders':
					c = a.seeders - b.seeders || a.leechers - b.leechers;
					break;
				case 'size':
					c = a.size - b.size;
					break;
				case 'published':
					c = Date.parse(a.published) - Date.parse(b.published);
					break;
				case 'title':
					c = collator.compare(a.title, b.title);
					break;
			}
			return c * sign || collator.compare(a.key, b.key);
		});
	}

	toggleRow(key: string, on?: boolean) {
		const next = on ?? !this.selection.has(key);
		if (next) this.selection.add(key);
		else this.selection.delete(key);
	}

	toggleAllVisible(on: boolean) {
		if (on) for (const r of this.visibleRows) this.selection.add(r.key);
		else this.selection.clear();
	}

	// ---- File preview -------------------------------------------------------

	async toggleExpanded(row: ResultRow) {
		if (this.expandedKey === row.key) {
			this.expandedKey = null;
			return;
		}
		this.expandedKey = row.key;
		await this.loadFiles(row);
	}

	async loadFiles(row: ResultRow): Promise<TorrentFile[] | null> {
		const id = row.primary.id;
		const existing = this.filesByResult[id];
		if (existing?.state === 'ok') return existing.files;
		this.filesByResult[id] = { state: 'loading' };
		try {
			const res = await api.resultFiles(id);
			this.filesByResult[id] = { state: 'ok', files: res.files };
			if (!this.fileSelection[id]) this.fileSelection[id] = res.files.map((f) => f.index);
			return res.files;
		} catch (e) {
			this.filesByResult[id] = { state: 'error', message: errorMessage(e, s.networkError) };
			return null;
		}
	}

	setFileSelected(resultId: string, index: number, on: boolean) {
		const cur = new Set(this.fileSelection[resultId] ?? []);
		if (on) cur.add(index);
		else cur.delete(index);
		this.fileSelection[resultId] = [...cur].sort((a, b) => a - b);
	}

	// ---- Download dialog ----------------------------------------------------

	openDownload(rows: ResultRow[]) {
		this.download = rows.map((row) => {
			const files = this.filesByResult[row.primary.id];
			const known = files?.state === 'ok' ? files.files : null;
			const sel = this.fileSelection[row.primary.id] ?? null;
			return {
				kind: 'result',
				row,
				files: known,
				preselected: known && sel && sel.length !== known.length ? sel : null
			};
		});
	}

	openDownloadForPayload(payload: Payload) {
		this.download = [{ kind: 'payload', payload }];
	}

	openSend(job: Job) {
		// One modal at a time: the queue reopens once the job is created.
		this.queueOpen = false;
		this.download = [{ kind: 'send', job }];
	}

	closeDownload() {
		this.download = null;
	}

	// ---- Jobs ---------------------------------------------------------------

	setQueueOpen(open: boolean) {
		this.queueOpen = open;
		if (open) void this.refreshJobs();
		else this.#schedulePoll();
	}

	async refreshJobs(): Promise<void> {
		if (this.#pollInFlight) return;
		this.#pollInFlight = true;
		try {
			const res = await api.jobs();
			this.jobs = res.jobs;
			this.jobsError = null;
			this.jobsLoaded = true;
		} catch (e) {
			this.jobsError = errorMessage(e, s.networkError);
		} finally {
			this.#pollInFlight = false;
			this.#schedulePoll();
		}
	}

	#schedulePoll() {
		if (this.#pollTimer) {
			clearTimeout(this.#pollTimer);
			this.#pollTimer = null;
		}
		if (!this.queueOpen && !this.hasActiveJobs) return;
		this.#pollTimer = setTimeout(() => {
			this.#pollTimer = null;
			void this.refreshJobs();
		}, POLL_MS);
	}

	/** Replace one job in place after a mutation, then poll again soon. */
	upsertJob(job: Job) {
		const idx = this.jobs.findIndex((j) => j.id === job.id);
		this.jobs = idx >= 0 ? this.jobs.map((j) => (j.id === job.id ? job : j)) : [job, ...this.jobs];
		this.#schedulePoll();
	}

	removeJob(id: string) {
		this.jobs = this.jobs.filter((j) => j.id !== id);
	}

	// ---- History ------------------------------------------------------------

	pushHistory(q: string) {
		const next = [q, ...this.history.filter((h) => h.toLowerCase() !== q.toLowerCase())].slice(0, HISTORY_MAX);
		this.history = next;
		save('history', next);
	}

	removeHistory(q: string) {
		this.history = this.history.filter((h) => h !== q);
		save('history', this.history);
	}

	clearHistory() {
		this.history = [];
		save('history', this.history);
	}

	isSaved(q: string): boolean {
		return this.saved.some((x) => x.toLowerCase() === q.trim().toLowerCase());
	}

	toggleSaved(q: string) {
		const t = q.trim();
		if (!t) return;
		this.saved = this.isSaved(t)
			? this.saved.filter((x) => x.toLowerCase() !== t.toLowerCase())
			: [t, ...this.saved];
		save('saved', this.saved);
	}
}

function groupByHash(results: Result[]): ResultRow[] {
	const byKey = new Map<string, ResultRow>();
	for (const r of results) {
		const key = r.infoHash ? `h:${r.infoHash.toLowerCase()}` : `r:${r.id}`;
		const existing = byKey.get(key);
		if (!existing) {
			byKey.set(key, {
				key,
				primary: r,
				sources: [r],
				indexers: [r.indexer],
				title: r.title,
				size: r.size,
				seeders: r.seeders,
				leechers: r.leechers,
				grabs: r.grabs,
				published: r.published,
				infoHash: r.infoHash ? r.infoHash.toLowerCase() : null,
				language: r.language ?? guessLanguage(r.title),
				freeleech: r.freeleech,
				categories: [...r.categories]
			});
			continue;
		}
		existing.sources.push(r);
		if (!existing.indexers.includes(r.indexer)) existing.indexers.push(r.indexer);
		if (r.seeders > existing.seeders) {
			existing.seeders = r.seeders;
			existing.primary = r;
			existing.title = r.title;
			existing.size = r.size;
		}
		existing.leechers = Math.max(existing.leechers, r.leechers);
		existing.grabs = Math.max(existing.grabs, r.grabs);
		if (Date.parse(r.published) < Date.parse(existing.published)) existing.published = r.published;
		existing.freeleech = existing.freeleech || r.freeleech;
		if (!existing.language) existing.language = r.language ?? guessLanguage(r.title);
		for (const c of r.categories) if (!existing.categories.includes(c)) existing.categories.push(c);
	}
	return [...byKey.values()];
}

function countActiveFilters(f: Filters): number {
	let n = 0;
	if (f.minSeeders > 0) n++;
	if (f.sizeMinGb != null) n++;
	if (f.sizeMaxGb != null) n++;
	if (f.maxAgeDays != null) n++;
	if (f.languages.length) n++;
	if (f.cachedOnly) n++;
	if (f.categories.length) n++;
	if (f.indexers.length) n++;
	return n;
}

export const app = new AppState();
