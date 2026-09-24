// Vite dev plugin: an in-memory implementation of API.md, enabled with VITE_MOCK=1
// (or `vite dev --mode mock`). Lets the UI be developed without the Go server.

import type { IncomingMessage, ServerResponse } from 'node:http';
import type { Plugin } from 'vite';
import {
	categories,
	defaults,
	engines,
	fakeHash,
	isCached,
	languages,
	resultsFor,
	seedJobs,
	sources,
	storages,
	type MockIndexer,
	type MockResult
} from './data.ts';

type Json = Record<string, unknown> | unknown[];

const REQUESTED_WITH = 'TorrentTrackerBrowser';
const RESULT_TTL_MS = 30 * 60_000;

interface StoredResult {
	result: MockResult;
	expires: number;
}

interface StoredPayload {
	id: string;
	name: string;
	infoHash: string;
	size: number | null;
	files: { path: string; size: number; index: number }[] | null;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Job = any;

const results = new Map<string, StoredResult>();
const payloads = new Map<string, StoredPayload>();
let jobs: Job[] = seedJobs(Date.now());
let counter = 100;

function send(res: ServerResponse, status: number, body: Json | null, headers: Record<string, string> = {}) {
	res.writeHead(status, { 'Content-Type': 'application/json', ...headers });
	res.end(body === null ? undefined : JSON.stringify(body));
}

function fail(res: ServerResponse, status: number, code: string, message: string) {
	send(res, status, { error: { code, message } });
}

function readBody(req: IncomingMessage): Promise<Buffer> {
	return new Promise((resolve, reject) => {
		const chunks: Buffer[] = [];
		req.on('data', (c: Buffer) => chunks.push(c));
		req.on('end', () => resolve(Buffer.concat(chunks)));
		req.on('error', reject);
	});
}

function guardMutation(req: IncomingMessage, res: ServerResponse): boolean {
	const type = req.headers['content-type'] ?? '';
	const ok =
		req.headers['x-requested-with'] === REQUESTED_WITH &&
		(type.startsWith('application/json') || type.startsWith('multipart/form-data') || req.method === 'DELETE');
	if (!ok) fail(res, 403, 'forbidden', 'missing X-Requested-With or Content-Type');
	return ok;
}

function allIndexers(): MockIndexer[] {
	return sources.flatMap((s) => s.indexers);
}

function findJob(id: string): Job | undefined {
	return jobs.find((j) => j.id === id);
}

function touch(job: Job) {
	job.updatedAt = new Date().toISOString();
}

const ACTIVE = ['queued', 'adding', 'fetching', 'copying'];

// Jobs advance on a 1 s tick so the queue visibly moves. Started by configureServer only:
// a module-level timer would keep every process that loads vite.config.ts alive (svelte-kit sync).
function tickJobs() {
	for (const job of jobs) {
		if (job.external) continue;
		if (job.id.startsWith('j_seed_') && job.state !== 'copying') continue;
		switch (job.state) {
			case 'queued':
				job.state = 'adding';
				touch(job);
				break;
			case 'adding':
				job.state = 'fetching';
				job.progress = 0;
				touch(job);
				break;
			case 'fetching':
				job.progress = Math.min(1, job.progress + 0.12);
				job.speed = 62_000_000 + Math.round(Math.random() * 20_000_000);
				job.eta = Math.round(((1 - job.progress) / 0.12) * 1);
				if (job.progress >= 1) {
					job.state = job.mode === 'links' ? 'ready' : 'copying';
					job.progress = job.mode === 'links' ? 1 : 0;
					job.speed = job.mode === 'links' ? null : job.speed;
					if (job.mode === 'links') {
						job.eta = null;
						for (const f of job.files) {
							f.done = f.size;
							f.state = 'done';
							f.url = `/api/jobs/${job.id}/files/${f.path.split('/').map(encodeURIComponent).join('/')}`;
						}
					}
				}
				touch(job);
				break;
			case 'copying': {
				const step = job.id.startsWith('j_seed_') ? 0.004 : 0.05;
				job.progress = Math.min(1, job.progress + step);
				job.speed = 38_000_000 + Math.round(Math.random() * 15_000_000);
				job.eta = Math.max(0, Math.round((1 - job.progress) / step));
				const totalSize = job.files.reduce((a: number, f: { size: number }) => a + f.size, 0);
				let budget = totalSize * job.progress;
				for (const f of job.files) {
					if (f.state === 'skipped') continue;
					const done = Math.min(f.size, Math.max(0, budget));
					budget -= f.size;
					f.done = done;
					f.state = done >= f.size ? 'done' : done > 0 ? 'copying' : 'pending';
				}
				if (job.progress >= 1) {
					job.state = 'done';
					job.speed = null;
					job.eta = null;
				}
				touch(job);
				break;
			}
		}
	}
}

function makeJob(body: Record<string, unknown>, name: string, infoHash: string | null, files: StoredPayload['files'], initial = 'queued'): Job {
	const mode = String(body.mode ?? defaults.mode);
	const wanted = Array.isArray(body.files) ? (body.files as number[]) : null;
	const jobFiles = (files ?? [{ path: name + '.mkv', size: 3_500_000_000, index: 0 }]).map((f) => ({
		path: body.subdir ? `${body.subdir}/${f.path}` : f.path,
		size: f.size,
		done: 0,
		state: wanted && !wanted.includes(f.index) ? 'skipped' : 'pending',
		url: null
	}));
	const now = new Date().toISOString();
	return {
		id: `j_${(counter++).toString(36)}${fakeHash(name + now).slice(0, 6)}`,
		createdAt: now,
		updatedAt: now,
		name,
		infoHash,
		engine: String(body.engine ?? defaults.engine),
		storage: mode === 'links' ? null : (body.storage as string | null) ?? defaults.storage,
		subdir: (body.subdir as string | null) ?? null,
		mode,
		state: initial,
		progress: 0,
		speed: null,
		eta: null,
		error: null,
		retryable: false,
		files: jobFiles,
		external: false
	};
}

function parseMultipartFilename(buf: Buffer, contentType: string): { filename: string; bytes: Buffer } | null {
	const m = /boundary=(?:"([^"]+)"|([^;]+))/.exec(contentType);
	const boundary = m?.[1] ?? m?.[2];
	if (!boundary) return null;
	const text = buf.toString('latin1');
	const start = text.indexOf('--' + boundary);
	if (start < 0) return null;
	const headEnd = text.indexOf('\r\n\r\n', start);
	if (headEnd < 0) return null;
	const head = text.slice(start, headEnd);
	const fn = /filename="([^"]*)"/.exec(head);
	const bodyStart = headEnd + 4;
	const bodyEnd = text.indexOf('\r\n--' + boundary, bodyStart);
	return {
		filename: fn?.[1] ?? 'upload.torrent',
		bytes: Buffer.from(text.slice(bodyStart, bodyEnd < 0 ? undefined : bodyEnd), 'latin1')
	};
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<boolean> {
	const url = new URL(req.url ?? '/', 'http://mock');
	const path = url.pathname;
	const method = (req.method ?? 'GET').toUpperCase();

	if (path === '/healthz') {
		send(res, 200, { ok: true });
		return true;
	}
	if (!path.startsWith('/api/')) return false;

	// Small artificial latency so loading states are visible.
	await new Promise((r) => setTimeout(r, 120));

	if (path === '/api/capabilities' && method === 'GET') {
		send(res, 200, {
			sources: sources.map((s) => ({
				id: s.id,
				name: s.name,
				indexers: s.indexers.map(({ id, name, private: p }) => ({ id, name, private: p }))
			})),
			engines,
			storages,
			categories,
			defaults,
			languages
		});
		return true;
	}

	if (path === '/api/search' && method === 'GET') {
		const q = url.searchParams.get('q') ?? '';
		if (!q.trim()) {
			fail(res, 400, 'bad_request', 'q is required');
			return true;
		}
		const cats = (url.searchParams.get('cat') ?? '').split(',').filter(Boolean);
		const wanted = (url.searchParams.get('indexers') ?? '').split(',').filter(Boolean);
		const targets = allIndexers().filter((i) => !wanted.length || wanted.includes(i.id));

		res.writeHead(200, {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache',
			Connection: 'keep-alive',
			'X-Accel-Buffering': 'no'
		});
		res.write(': connected\n\n');
		let eventId = 0;
		const started = Date.now();
		let remaining = targets.length;
		let total = 0;
		let closed = false;
		const timers: ReturnType<typeof setTimeout>[] = [];
		const emit = (event: string, data: unknown) => {
			if (closed) return;
			res.write(`id: ${++eventId}\nevent: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
		};
		const finish = () => {
			if (closed) return;
			emit('done', { total, elapsedMs: Date.now() - started });
			closed = true;
			res.end();
		};
		const ping = setInterval(() => !closed && res.write(': ping\n\n'), 15_000);
		req.on('close', () => {
			closed = true;
			clearInterval(ping);
			timers.forEach(clearTimeout);
		});

		for (const ix of targets) {
			const all = resultsFor(ix, q, cats);
			for (const r of all) results.set(r.id, { result: r, expires: Date.now() + RESULT_TTL_MS });
			const pages = ix.pages ?? 1;
			const chunk = Math.ceil(all.length / pages);
			for (let p = 0; p < pages; p++) {
				const delay = ix.latencyMs + p * 350;
				timers.push(
					setTimeout(() => {
						if (closed) return;
						const last = p === pages - 1;
						if (ix.fails && last) {
							emit('batch', { indexer: ix.id, results: [], done: false, error: ix.fails, elapsedMs: Date.now() - started });
						} else {
							const slice = all.slice(p * chunk, (p + 1) * chunk).map(({ _files, ...rest }) => rest);
							total += slice.length;
							emit('batch', { indexer: ix.id, results: slice, done: last, error: null, elapsedMs: Date.now() - started });
						}
						if (last && --remaining === 0) {
							clearInterval(ping);
							finish();
						}
					}, delay)
				);
			}
		}
		if (!targets.length) finish();
		return true;
	}

	if (path === '/api/payloads' && method === 'POST') {
		if (!guardMutation(req, res)) return true;
		const buf = await readBody(req);
		const type = req.headers['content-type'] ?? '';
		let name: string;
		let infoHash: string;
		let files: StoredPayload['files'] = null;
		let size: number | null = null;
		if (type.startsWith('multipart/form-data')) {
			const part = parseMultipartFilename(buf, type);
			if (!part) {
				fail(res, 400, 'bad_request', 'multipart body without a torrent field');
				return true;
			}
			name = part.filename.replace(/\.torrent$/i, '');
			infoHash = fakeHash(part.bytes.toString('latin1').slice(0, 512) || part.filename);
			size = 2_400_000_000;
			files = [
				{ path: `${name}/${name}.mkv`, size: 2_350_000_000, index: 0 },
				{ path: `${name}/${name}.fr.srt`, size: 50_000_000, index: 1 }
			];
		} else {
			let magnet = '';
			try {
				magnet = String((JSON.parse(buf.toString('utf8')) as { magnet?: string }).magnet ?? '');
			} catch {
				fail(res, 400, 'bad_request', 'invalid JSON');
				return true;
			}
			const m = /xt=urn:btih:([a-z0-9]{32,40})/i.exec(magnet);
			if (!magnet.startsWith('magnet:?') || !m) {
				fail(res, 400, 'invalid_magnet', 'not a magnet URI');
				return true;
			}
			infoHash = m[1].toLowerCase();
			const dn = /[?&]dn=([^&]+)/.exec(magnet);
			name = dn ? decodeURIComponent(dn[1].replace(/\+/g, ' ')) : infoHash;
		}
		const payload: StoredPayload = { id: `p_${fakeHash(infoHash).slice(0, 10)}`, name, infoHash, size, files };
		payloads.set(payload.id, payload);
		send(res, 201, payload as unknown as Json);
		return true;
	}

	const filesMatch = /^\/api\/results\/([^/]+)\/files$/.exec(path);
	if (filesMatch && method === 'GET') {
		const stored = results.get(decodeURIComponent(filesMatch[1]));
		if (!stored || stored.expires < Date.now()) {
			fail(res, 404, 'not_found', 'unknown or expired result');
			return true;
		}
		await new Promise((r) => setTimeout(r, 400));
		if (!stored.result._files) {
			fail(res, 409, 'no_preview', 'no .torrent available and the hash is not cached');
			return true;
		}
		send(res, 200, { files: stored.result._files });
		return true;
	}

	const payloadMatch = /^\/api\/results\/([^/]+)\/payload$/.exec(path);
	if (payloadMatch && method === 'GET') {
		const stored = results.get(decodeURIComponent(payloadMatch[1]));
		if (!stored) {
			fail(res, 404, 'not_found', 'unknown or expired result');
			return true;
		}
		const as = url.searchParams.get('as');
		const r = stored.result;
		if (as === 'magnet') {
			if (!r.hasMagnet) {
				fail(res, 404, 'no_magnet', 'this source cannot provide a magnet');
				return true;
			}
			const hash = r.infoHash ?? fakeHash(r.id);
			res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
			res.end(`magnet:?xt=urn:btih:${hash}&dn=${encodeURIComponent(r.title)}&tr=udp%3A%2F%2Ftracker.example.invalid%3A6969`);
			return true;
		}
		if (as === 'torrent') {
			if (!r.hasTorrent) {
				fail(res, 404, 'no_torrent', 'this source cannot provide a .torrent');
				return true;
			}
			const safe = r.title.replace(/[^\w.-]+/g, '_');
			res.writeHead(200, {
				'Content-Type': 'application/x-bittorrent',
				'Content-Disposition': `attachment; filename="${safe}.torrent"`
			});
			res.end(`d8:announce35:udp://tracker.example.invalid:69694:infod4:name${r.title.length}:${r.title}ee`);
			return true;
		}
		fail(res, 400, 'bad_request', 'as must be magnet or torrent');
		return true;
	}

	if (path === '/api/cached' && method === 'POST') {
		if (!guardMutation(req, res)) return true;
		const body = JSON.parse((await readBody(req)).toString('utf8')) as { engine?: string; hashes?: string[] };
		const engine = engines.find((e) => e.id === body.engine);
		if (!engine) {
			fail(res, 404, 'unknown_engine', `engine ${body.engine} is not configured`);
			return true;
		}
		if (!engine.caps.cached) {
			fail(res, 400, 'not_supported', `${engine.name} does not report cache status`);
			return true;
		}
		const cached: Record<string, boolean> = {};
		for (const h of body.hashes ?? []) cached[h] = isCached(h);
		await new Promise((r) => setTimeout(r, 300));
		send(res, 200, { cached });
		return true;
	}

	if (path === '/api/jobs' && method === 'GET') {
		const own = jobs.filter((j) => !j.external).sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
		const ext = jobs.filter((j) => j.external);
		send(res, 200, { jobs: [...own, ...ext] });
		return true;
	}

	if (path === '/api/jobs' && method === 'POST') {
		if (!guardMutation(req, res)) return true;
		const body = JSON.parse((await readBody(req)).toString('utf8')) as Record<string, unknown>;
		if (!!body.resultId === !!body.payloadId) {
			fail(res, 400, 'bad_request', 'exactly one of resultId or payloadId');
			return true;
		}
		if (body.mode !== 'links' && !body.storage) {
			fail(res, 400, 'bad_request', 'storage is required unless mode is links');
			return true;
		}
		if (!engines.some((e) => e.id === body.engine)) {
			fail(res, 404, 'unknown_engine', `engine ${body.engine} is not configured`);
			return true;
		}
		let job: Job;
		if (body.resultId) {
			const stored = results.get(String(body.resultId));
			if (!stored) {
				fail(res, 404, 'not_found', 'unknown or expired result');
				return true;
			}
			if (stored.result.title.includes('Affinity')) {
				fail(res, 502, 'engine_error', 'TorBox: this torrent was rejected by the engine (DMCA)');
				return true;
			}
			job = makeJob(body, stored.result.title, stored.result.infoHash, stored.result._files);
		} else {
			const p = payloads.get(String(body.payloadId));
			if (!p) {
				fail(res, 404, 'not_found', 'unknown payload');
				return true;
			}
			job = makeJob(body, p.name, p.infoHash, p.files);
		}
		jobs.unshift(job);
		send(res, 201, job);
		return true;
	}

	const jobAction = /^\/api\/jobs\/([^/]+)(?:\/(cancel|retry|send))?$/.exec(path);
	if (jobAction) {
		const id = decodeURIComponent(jobAction[1]);
		const action = jobAction[2];
		const job = findJob(id);
		if (!job) {
			fail(res, 404, 'not_found', 'unknown job');
			return true;
		}
		if (method === 'DELETE' && !action) {
			if (!guardMutation(req, res)) return true;
			jobs = jobs.filter((j) => j.id !== id);
			res.writeHead(204);
			res.end();
			return true;
		}
		if (method === 'POST' && action === 'cancel') {
			if (!guardMutation(req, res)) return true;
			if (!ACTIVE.includes(job.state)) {
				fail(res, 409, 'not_active', 'job is not running');
				return true;
			}
			job.state = 'cancelled';
			job.speed = null;
			job.eta = null;
			touch(job);
			send(res, 200, job);
			return true;
		}
		if (method === 'POST' && action === 'retry') {
			if (!guardMutation(req, res)) return true;
			if (!job.retryable) {
				fail(res, 409, 'not_retryable', 'job cannot be retried');
				return true;
			}
			job.state = 'queued';
			job.error = null;
			job.retryable = false;
			job.progress = 0;
			for (const f of job.files) if (f.state === 'failed') f.state = 'pending';
			// Seeded jobs stay still unless copying; make this one live.
			job.id = job.id.replace('j_seed_', 'j_retry_');
			touch(job);
			send(res, 200, job);
			return true;
		}
		if (method === 'POST' && action === 'send') {
			if (!guardMutation(req, res)) return true;
			if (!job.external) {
				fail(res, 409, 'not_external', 'only external items can be sent');
				return true;
			}
			const body = JSON.parse((await readBody(req)).toString('utf8')) as Record<string, unknown>;
			if (!body.storage) {
				fail(res, 400, 'bad_request', 'storage is required');
				return true;
			}
			const created = makeJob(
				{ ...body, engine: job.engine },
				job.name,
				job.infoHash,
				job.files.map((f: { path: string; size: number }, i: number) => ({ path: f.path, size: f.size, index: i })),
				'copying'
			);
			jobs.unshift(created);
			send(res, 201, created);
			return true;
		}
	}

	const fileMatch = /^\/api\/jobs\/([^/]+)\/files\/(.+)$/.exec(path);
	if (fileMatch && method === 'GET') {
		res.writeHead(200, {
			'Content-Type': 'application/octet-stream',
			'Content-Disposition': `attachment; filename="${decodeURIComponent(fileMatch[2]).split('/').pop()}"`
		});
		res.end('mock file content\n');
		return true;
	}

	fail(res, 404, 'not_found', `no mock route for ${method} ${path}`);
	return true;
}

export function mockApi(): Plugin {
	return {
		name: 'ttb-mock-api',
		configureServer(server) {
			server.config.logger.info('[mock] API.md mock backend enabled (VITE_MOCK=1)');
			const timer = setInterval(tickJobs, 1000);
			(timer as unknown as { unref?: () => void }).unref?.();
			server.httpServer?.on('close', () => clearInterval(timer));
			server.middlewares.use((req, res, next) => {
				handle(req, res)
					.then((handled) => {
						if (!handled) next();
					})
					.catch((e) => {
						fail(res, 500, 'mock_error', e instanceof Error ? e.message : String(e));
					});
			});
		}
	};
}
