import type {
	ApiErrorBody,
	CachedResponse,
	Capabilities,
	CreateJobBody,
	FilesResponse,
	Job,
	JobsResponse,
	Payload,
	SendJobBody
} from './types';

export const REQUESTED_WITH = 'TorrentTrackerBrowser';

export class ApiError extends Error {
	constructor(
		public readonly status: number,
		public readonly code: string,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

async function parseError(res: Response): Promise<ApiError> {
	let code = `http_${res.status}`;
	let message = res.statusText || `HTTP ${res.status}`;
	try {
		const body = (await res.json()) as Partial<ApiErrorBody>;
		if (body?.error) {
			code = body.error.code ?? code;
			message = body.error.message ?? message;
		}
	} catch {
		// non-JSON error body: keep the HTTP status
	}
	return new ApiError(res.status, code, message);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const headers = new Headers(init.headers);
	headers.set('X-Requested-With', REQUESTED_WITH);
	const method = (init.method ?? 'GET').toUpperCase();
	const isForm = init.body instanceof FormData;
	if (method !== 'GET' && !isForm && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json');
	}
	let res: Response;
	try {
		res = await fetch(path, { ...init, headers });
	} catch (e) {
		throw new ApiError(0, 'network', e instanceof Error ? e.message : String(e));
	}
	if (!res.ok) throw await parseError(res);
	if (res.status === 204) return undefined as T;
	const type = res.headers.get('Content-Type') ?? '';
	if (type.includes('application/json')) return (await res.json()) as T;
	return (await res.text()) as unknown as T;
}

function json(body: unknown): RequestInit {
	return { method: 'POST', body: JSON.stringify(body) };
}

export const api = {
	healthz: () => request<{ ok: boolean }>('/healthz'),

	capabilities: () => request<Capabilities>('/api/capabilities'),

	/** The streaming search lives in sse.ts; this only builds its URL. */
	searchUrl(q: string, cat: string[] = [], indexers: string[] = []): string {
		const params = new URLSearchParams({ q });
		if (cat.length) params.set('cat', cat.join(','));
		if (indexers.length) params.set('indexers', indexers.join(','));
		return `/api/search?${params.toString()}`;
	},

	postMagnet: (magnet: string) => request<Payload>('/api/payloads', json({ magnet })),

	postTorrent(file: File) {
		const form = new FormData();
		form.append('torrent', file, file.name);
		return request<Payload>('/api/payloads', { method: 'POST', body: form });
	},

	resultFiles: (id: string) =>
		request<FilesResponse>(`/api/results/${encodeURIComponent(id)}/files`),

	payloadUrl: (id: string, as: 'magnet' | 'torrent') =>
		`/api/results/${encodeURIComponent(id)}/payload?as=${as}`,

	magnet: (id: string) => request<string>(api.payloadUrl(id, 'magnet')),

	cached: (engine: string, hashes: string[]) =>
		request<CachedResponse>('/api/cached', json({ engine, hashes })),

	createJob: (body: CreateJobBody) => request<Job>('/api/jobs', json(body)),

	jobs: () => request<JobsResponse>('/api/jobs'),

	cancelJob: (id: string) =>
		request<Job>(`/api/jobs/${encodeURIComponent(id)}/cancel`, json({})),

	retryJob: (id: string) => request<Job>(`/api/jobs/${encodeURIComponent(id)}/retry`, json({})),

	deleteJob: (id: string, deleteFiles = false) =>
		request<void>(
			`/api/jobs/${encodeURIComponent(id)}${deleteFiles ? '?deleteFiles=true' : ''}`,
			{ method: 'DELETE' }
		),

	sendJob: (id: string, body: SendJobBody) =>
		request<Job>(`/api/jobs/${encodeURIComponent(id)}/send`, json(body)),

	jobFileUrl: (jobId: string, path: string) =>
		`/api/jobs/${encodeURIComponent(jobId)}/files/${path.split('/').map(encodeURIComponent).join('/')}`
};

export function errorMessage(e: unknown, fallback: string): string {
	if (e instanceof ApiError) return e.status === 0 ? fallback : e.message;
	if (e instanceof Error) return e.message || fallback;
	return fallback;
}
