// Mirrors API.md exactly. Change API.md first, then this file.

export type Category =
	| 'movies'
	| 'tv'
	| 'anime'
	| 'music'
	| 'books'
	| 'games'
	| 'software'
	| 'other';

export type Language = 'fr' | 'multi' | 'vostfr' | 'en';

export type DeliveryMode = 'copy' | 'adopt' | 'links';

export interface ApiErrorBody {
	error: { code: string; message: string };
}

export interface Indexer {
	id: string; // "<sourceId>:<upstreamId>"
	name: string;
	private: boolean;
}

export interface Source {
	id: string;
	name: string;
	indexers: Indexer[];
}

export interface EngineCaps {
	cached: boolean;
	select: boolean;
	directLinks: boolean;
	localFiles: boolean;
	savePath: boolean;
}

export interface Engine {
	id: string;
	name: string;
	caps: EngineCaps;
}

export interface Storage {
	id: string;
	label: string;
	free: number;
	scanned: boolean; // a malware scanner covers this storage
}

export interface Capabilities {
	sources: Source[];
	engines: Engine[];
	storages: Storage[];
	categories: Category[];
	defaults: { engine: string; storage: string; mode: DeliveryMode };
	languages: Language[];
	scanner: boolean;
	/** Name from the proxy's user header; null = the operator, who sees every job. */
	user: string | null;
	/** Storage whose finished jobs can be shared; null = sharing off. */
	shareStorage: string | null;
}

export interface Result {
	id: string;
	indexer: string;
	title: string;
	size: number;
	seeders: number;
	leechers: number;
	grabs: number;
	published: string;
	infoHash: string | null;
	hasMagnet: boolean;
	hasTorrent: boolean;
	freeleech: boolean;
	categories: Category[];
	language: Language | null;
	infoUrl: string | null;
}

export interface BatchEvent {
	indexer: string;
	results: Result[];
	done: boolean;
	error: string | null;
	elapsedMs: number;
}

export interface DoneEvent {
	total: number;
	elapsedMs: number;
}

export interface TorrentFile {
	path: string;
	size: number;
	index: number;
}

export interface Payload {
	id: string;
	name: string;
	infoHash: string;
	size: number | null;
	files: TorrentFile[] | null;
}

export interface FilesResponse {
	files: TorrentFile[];
}

export interface CachedResponse {
	cached: Record<string, boolean>;
}

export type JobState =
	| 'queued'
	| 'adding'
	| 'waitingSelection'
	| 'fetching'
	| 'ready'
	| 'copying'
	| 'scanning'
	| 'done'
	| 'infected'
	| 'failed'
	| 'cancelled';

export type JobFileState = 'pending' | 'copying' | 'done' | 'failed' | 'skipped' | 'quarantined';

export type ScanStatus = 'clean' | 'infected' | 'skipped' | 'error';

export interface ScanFinding {
	path: string;
	status: ScanStatus;
	signature?: string;
	reason?: string;
	quarantine?: string;
}

export interface ScanResult {
	status: ScanStatus;
	findings: ScanFinding[];
	scannedAt: string;
}

export interface JobFile {
	path: string;
	size: number;
	done: number;
	state: JobFileState;
	url: string | null;
}

export interface Job {
	id: string;
	createdAt: string;
	updatedAt: string;
	name: string;
	infoHash: string | null;
	engine: string;
	storage: string | null;
	subdir: string | null;
	mode: DeliveryMode;
	state: JobState;
	progress: number;
	speed: number | null;
	eta: number | null;
	error: string | null;
	retryable: boolean;
	files: JobFile[];
	external: boolean;
	scan: ScanResult | null;
	owner: string | null;
}

export interface CreateJobBody {
	resultId: string | null;
	payloadId: string | null;
	engine: string;
	storage: string | null;
	subdir: string | null;
	files: number[] | null;
	mode: DeliveryMode;
}

export interface SendJobBody {
	storage: string;
	subdir: string | null;
	files: number[] | null;
	mode: DeliveryMode;
}

export interface JobsResponse {
	jobs: Job[];
}
