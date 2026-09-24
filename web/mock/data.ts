// Realistic fixtures for the mock backend. Everything here mirrors API.md shapes.

export interface MockIndexer {
	id: string;
	name: string;
	private: boolean;
	latencyMs: number;
	fails?: string;
	pages?: number; // number of batches to emit (default 1)
}

export interface MockSource {
	id: string;
	name: string;
	indexers: MockIndexer[];
}

export const sources: MockSource[] = [
	{
		id: 'prowlarr',
		name: 'Prowlarr',
		indexers: [
			{ id: 'prowlarr:16', name: 'TR4KER', private: true, latencyMs: 200, pages: 2 },
			{ id: 'prowlarr:21', name: 'C411', private: true, latencyMs: 400 },
			{
				id: 'prowlarr:3',
				name: 'YTS',
				private: false,
				latencyMs: 2000,
				fails: 'upstream returned 502 Bad Gateway (cloudflare challenge)'
			}
		]
	},
	{
		id: 'torznab',
		name: 'Torznab',
		indexers: [{ id: 'torznab:nyaa', name: 'Nyaa', private: false, latencyMs: 6000 }]
	}
];

export const engines = [
	{
		id: 'torbox',
		name: 'TorBox',
		caps: { cached: true, select: false, directLinks: true, localFiles: false, savePath: false }
	},
	{
		id: 'qbit',
		name: 'qBittorrent',
		caps: { cached: false, select: false, directLinks: false, localFiles: true, savePath: true }
	}
];

export const storages = [
	{ id: 'downloads', label: 'Téléchargements', free: 9_120_000_000_000 },
	{ id: 'films', label: 'Films (NAS)', free: 1_240_000_000_000 }
];

export const categories = ['movies', 'tv', 'anime', 'music', 'books', 'games', 'software', 'other'];
export const languages = ['fr', 'multi', 'vostfr', 'en'];
export const defaults = { engine: 'torbox', storage: 'downloads', mode: 'copy' };

interface Release {
	title: string;
	size: number;
	cat: string;
	lang: string | null;
	hash: string | null;
	freeleech?: boolean;
	files?: string[];
	noPreview?: boolean;
}

const GB = 1_000_000_000;
const MB = 1_000_000;

// Deterministic 40-hex hash from a seed string.
export function fakeHash(seed: string): string {
	let h1 = 0x811c9dc5;
	let out = '';
	for (let round = 0; round < 5; round++) {
		for (let i = 0; i < seed.length; i++) {
			h1 ^= seed.charCodeAt(i) + round;
			h1 = Math.imul(h1, 0x01000193) >>> 0;
		}
		out += h1.toString(16).padStart(8, '0');
	}
	return out.slice(0, 40);
}

const releases: Release[] = [
	{ title: 'Dune Part Two 2024 MULTI 1080p WEB H265-XYZ', size: 4.81 * GB, cat: 'movies', lang: 'multi', hash: fakeHash('dune2-1080'), files: ['Dune.Part.Two.2024.MULTI.1080p.WEB.H265-XYZ.mkv', 'Dune.Part.Two.2024.MULTI.1080p.WEB.H265-XYZ.srt'] },
	{ title: 'Dune Part Two 2024 MULTI 2160p UHD BluRay HDR x265-FraMeSToR', size: 24.3 * GB, cat: 'movies', lang: 'multi', hash: fakeHash('dune2-2160'), freeleech: true },
	{ title: 'Dune.Deuxieme.Partie.2024.FRENCH.720p.WEB.x264-FW', size: 2.1 * GB, cat: 'movies', lang: 'fr', hash: fakeHash('dune2-720-fr') },
	{ title: 'Dune (1984) VOSTFR BluRay 1080p x264-HDF', size: 8.9 * GB, cat: 'movies', lang: 'vostfr', hash: null },
	{ title: 'Dune Part Two (2024) [1080p] [WEBRip] [5.1] [YTS.MX]', size: 2.35 * GB, cat: 'movies', lang: 'en', hash: fakeHash('dune2-yts') },
	{ title: 'Le Comte de Monte-Cristo 2024 FRENCH 1080p BluRay x264-UKDHD', size: 11.2 * GB, cat: 'movies', lang: 'fr', hash: fakeHash('monte-cristo'), freeleech: true },
	{ title: 'Anatomie d’une chute 2023 FRENCH 1080p WEB H264-FW', size: 5.4 * GB, cat: 'movies', lang: 'fr', hash: fakeHash('anatomie') },
	{ title: 'Oppenheimer 2023 MULTI VFF 2160p WEB-DL DV HDR x265-Slay3R', size: 19.7 * GB, cat: 'movies', lang: 'multi', hash: fakeHash('oppenheimer'), noPreview: true },
	{ title: 'Shogun S01 MULTI 1080p WEB H264-FTMVHD', size: 28.6 * GB, cat: 'tv', lang: 'multi', hash: fakeHash('shogun-s01'), files: Array.from({ length: 10 }, (_, i) => `Shogun.S01E${String(i + 1).padStart(2, '0')}.MULTI.1080p.WEB.H264-FTMVHD.mkv`) },
	{ title: 'Shogun S01E05 VOSTFR 1080p WEB H264-FW', size: 2.9 * GB, cat: 'tv', lang: 'vostfr', hash: fakeHash('shogun-e05') },
	{ title: 'Slow Horses S04 VOSTFR 1080p WEB H264-NTb', size: 12.1 * GB, cat: 'tv', lang: 'vostfr', hash: fakeHash('slowhorses-s04') },
	{ title: 'Lupin S03 FRENCH 1080p NF WEB-DL DDP5.1 H264-TEPES', size: 9.8 * GB, cat: 'tv', lang: 'fr', hash: fakeHash('lupin-s03') },
	{ title: 'Severance S02 MULTI 2160p ATVP WEB-DL DV HDR x265-DYNASTY', size: 41.2 * GB, cat: 'tv', lang: 'multi', hash: fakeHash('severance-s02'), freeleech: true },
	{ title: 'The Bear S03 1080p HULU WEB-DL DDP5.1 H264-NTb', size: 14.3 * GB, cat: 'tv', lang: 'en', hash: fakeHash('bear-s03') },
	{ title: '[Erai-raws] Frieren - Beyond Journey’s End - 01~28 [1080p][Multiple Subtitle]', size: 38.4 * GB, cat: 'anime', lang: 'vostfr', hash: fakeHash('frieren'), files: Array.from({ length: 28 }, (_, i) => `[Erai-raws] Frieren - ${String(i + 1).padStart(2, '0')} [1080p].mkv`) },
	{ title: '[SubsPlease] Dandadan - 12 (1080p) [A1B2C3D4].mkv', size: 1.4 * GB, cat: 'anime', lang: 'en', hash: fakeHash('dandadan-12') },
	{ title: 'Dune Awakening (v1.2.5 + DLC, MULTi14) [FitGirl Repack]', size: 46.1 * GB, cat: 'games', lang: 'multi', hash: fakeHash('dune-awakening') },
	{ title: 'Baldur’s Gate 3 v4.1.1.6 (Patch 8) GOG', size: 122.8 * GB, cat: 'games', lang: 'multi', hash: fakeHash('bg3') },
	{ title: 'Hans Zimmer - Dune Part Two (Original Motion Picture Soundtrack) (2024) [FLAC 24-96]', size: 1.9 * GB, cat: 'music', lang: null, hash: fakeHash('zimmer-dune2'), files: Array.from({ length: 22 }, (_, i) => `${String(i + 1).padStart(2, '0')} - Track ${i + 1}.flac`) },
	{ title: 'Aya Nakamura - DNK (2023) [MP3 320]', size: 98 * MB, cat: 'music', lang: 'fr', hash: fakeHash('aya-dnk') },
	{ title: 'Frank Herbert - Dune (Le Cycle de Dune, tomes 1 à 6) [EPUB FR]', size: 12 * MB, cat: 'books', lang: 'fr', hash: fakeHash('dune-epub-fr') },
	{ title: 'Frank Herbert - Dune Chronicles (Books 1-6) [EPUB]', size: 9 * MB, cat: 'books', lang: 'en', hash: null },
	{ title: 'Affinity Photo 2.5.3 macOS Universal', size: 3.1 * GB, cat: 'software', lang: 'multi', hash: fakeHash('affinity'), noPreview: true },
	{ title: 'Ubuntu 24.04.2 LTS Desktop amd64', size: 5.9 * GB, cat: 'software', lang: 'en', hash: fakeHash('ubuntu-2404') },
	{ title: 'Dune 1984 Extended Cut MULTI 1080p BluRay x264-ROUGH', size: 10.6 * GB, cat: 'movies', lang: 'multi', hash: fakeHash('dune84-ext') },
	{ title: 'Dune Prophecy S01 MULTI 1080p MAX WEB-DL H264-FTMVHD', size: 16.5 * GB, cat: 'tv', lang: 'multi', hash: fakeHash('dune-prophecy') },
	{ title: 'Dune.Prophecy.S01E03.VOSTFR.720p.WEB.x264-FW', size: 1.1 * GB, cat: 'tv', lang: 'vostfr', hash: fakeHash('prophecy-e03') },
	{ title: 'Jodorowsky’s Dune 2013 VOSTFR 1080p WEB-DL x264', size: 3.3 * GB, cat: 'movies', lang: 'vostfr', hash: fakeHash('jodorowsky') },
	{ title: 'The Dune Sea Sessions - Vol. 1 (2025) [MP3 V0]', size: 145 * MB, cat: 'music', lang: 'en', hash: fakeHash('dune-sea'), freeleech: true },
	{ title: 'Kaamelott Premier Volet 2021 FRENCH 1080p BluRay x264-LOST', size: 7.7 * GB, cat: 'movies', lang: 'fr', hash: fakeHash('kaamelott') }
];

// Which releases each indexer lists. Overlaps are deliberate: same hash on several indexers.
const listing: Record<string, number[]> = {
	'prowlarr:16': [0, 1, 2, 5, 6, 7, 8, 9, 10, 11, 12, 16, 18, 19, 20, 24, 25, 26, 29],
	'prowlarr:21': [0, 1, 3, 5, 6, 8, 11, 12, 13, 17, 18, 20, 21, 22, 23, 24, 25, 27, 28],
	'prowlarr:3': [4],
	'torznab:nyaa': [14, 15, 0, 8, 12, 16, 22, 23, 25, 26]
};

function hashToInt(hash: string): number {
	return parseInt(hash.slice(0, 6), 16);
}

function stableRand(seed: string, salt: string): number {
	return (hashToInt(fakeHash(seed + salt)) % 10_000) / 10_000;
}

export interface MockResult {
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
	categories: string[];
	language: string | null;
	infoUrl: string | null;
	// mock-only, never serialised to the client
	_files: { path: string; size: number; index: number }[] | null;
}

function tokens(q: string): string[] {
	return q
		.toLowerCase()
		.split(/\s+/)
		.map((t) => t.replace(/[^a-z0-9àâçéèêëîïôûùüÿœ]/g, ''))
		.filter((t) => t.length > 1);
}

function matches(release: Release, q: string, cats: string[]): boolean {
	if (cats.length && !cats.includes(release.cat)) return false;
	const toks = tokens(q);
	if (!toks.length) return true;
	const hay = release.title.toLowerCase();
	return toks.some((t) => hay.includes(t));
}

function filesFor(release: Release, idx: number): MockResult['_files'] {
	if (release.noPreview) return null;
	const names = release.files ?? [
		release.title.replace(/[^\w.-]+/g, '.').replace(/\.+/g, '.') +
			(release.cat === 'books' ? '.epub' : release.cat === 'music' ? '.flac' : '.mkv')
	];
	const folder = release.title.split(' ').slice(0, 3).join('.');
	const per = Math.floor(release.size / names.length);
	return names.map((n, i) => ({
		path: names.length > 1 ? `${folder}/${n}` : n,
		size: i === names.length - 1 ? release.size - per * (names.length - 1) : per,
		index: i + (idx % 1 === 0 ? 0 : 0)
	}));
}

/** Results for one indexer and one query, deterministic across calls. */
export function resultsFor(indexer: MockIndexer, q: string, cats: string[]): MockResult[] {
	const out: MockResult[] = [];
	for (const idx of listing[indexer.id] ?? []) {
		const r = releases[idx];
		if (!matches(r, q, cats)) continue;
		const seed = `${indexer.id}|${idx}`;
		const private_ = indexer.private;
		const seeders = Math.floor(stableRand(seed, 's') * (private_ ? 260 : 60)) + (idx % 7 === 3 ? 0 : 1);
		const ageDays = Math.floor(stableRand(seed, 'a') * 700);
		const published = new Date(Date.now() - ageDays * 86_400_000 - 3_600_000 * (idx % 24)).toISOString();
		out.push({
			id: `r_${fakeHash(seed + q).slice(0, 12)}`,
			indexer: indexer.id,
			title: r.title,
			size: Math.round(r.size * (1 + (stableRand(seed, 'z') - 0.5) * 0.002)),
			seeders,
			leechers: Math.floor(stableRand(seed, 'l') * 40),
			grabs: Math.floor(stableRand(seed, 'g') * 900),
			published,
			infoHash: r.hash,
			hasMagnet: !private_ || idx % 5 !== 0,
			hasTorrent: private_ || idx % 3 !== 0,
			freeleech: !!r.freeleech && private_,
			categories: [r.cat],
			language: r.lang,
			infoUrl: idx % 4 === 0 ? null : `https://example.invalid/${indexer.name.toLowerCase()}/torrent/${idx}`,
			_files: filesFor(r, idx)
		});
	}
	return out;
}

/** The mock says a hash is cached when its first hex digit is even. */
export function isCached(hash: string): boolean {
	const d = parseInt(hash[0], 16);
	return Number.isFinite(d) && d % 2 === 0;
}

export const seedJobs = (now: number) => [
	{
		id: 'j_seed_done',
		createdAt: new Date(now - 3_600_000).toISOString(),
		updatedAt: new Date(now - 3_000_000).toISOString(),
		name: 'Le Comte de Monte-Cristo 2024 FRENCH 1080p BluRay x264-UKDHD',
		infoHash: fakeHash('monte-cristo'),
		engine: 'torbox',
		storage: 'films',
		subdir: 'Le Comte de Monte-Cristo (2024)',
		mode: 'copy',
		state: 'done',
		progress: 1,
		speed: null,
		eta: null,
		error: null,
		retryable: false,
		files: [
			{ path: 'Le Comte de Monte-Cristo (2024)/Le.Comte.de.Monte-Cristo.2024.FRENCH.1080p.BluRay.x264-UKDHD.mkv', size: 11.2 * GB, done: 11.2 * GB, state: 'done', url: null }
		],
		external: false
	},
	{
		id: 'j_seed_copying',
		createdAt: new Date(now - 600_000).toISOString(),
		updatedAt: new Date(now).toISOString(),
		name: 'Shogun S01 MULTI 1080p WEB H264-FTMVHD',
		infoHash: fakeHash('shogun-s01'),
		engine: 'torbox',
		storage: 'downloads',
		subdir: null,
		mode: 'copy',
		state: 'copying',
		progress: 0.31,
		speed: 48_000_000,
		eta: 410,
		error: null,
		retryable: false,
		files: Array.from({ length: 10 }, (_, i) => ({
			path: `Shogun.S01/Shogun.S01E${String(i + 1).padStart(2, '0')}.MULTI.1080p.WEB.H264-FTMVHD.mkv`,
			size: 2.86 * GB,
			done: i < 3 ? 2.86 * GB : i === 3 ? 0.4 * GB : 0,
			state: i < 3 ? 'done' : i === 3 ? 'copying' : 'pending',
			url: null
		})),
		external: false
	},
	{
		id: 'j_seed_failed',
		createdAt: new Date(now - 7_200_000).toISOString(),
		updatedAt: new Date(now - 7_000_000).toISOString(),
		name: 'Severance S02 MULTI 2160p ATVP WEB-DL DV HDR x265-DYNASTY',
		infoHash: fakeHash('severance-s02'),
		engine: 'torbox',
		storage: 'downloads',
		subdir: 'Severance',
		mode: 'copy',
		state: 'failed',
		progress: 0.12,
		speed: null,
		eta: null,
		error: 'copy: write /mnt/media/downloads/manual/Severance/.part: no space left on device',
		retryable: true,
		files: [
			{ path: 'Severance/Severance.S02E01.mkv', size: 4.1 * GB, done: 4.1 * GB, state: 'done', url: null },
			{ path: 'Severance/Severance.S02E02.mkv', size: 4.1 * GB, done: 0.9 * GB, state: 'failed', url: null },
			{ path: 'Severance/Severance.S02E03.mkv', size: 4.1 * GB, done: 0, state: 'pending', url: null }
		],
		external: false
	},
	{
		id: 'j_seed_links',
		createdAt: new Date(now - 86_400_000).toISOString(),
		updatedAt: new Date(now - 86_000_000).toISOString(),
		name: 'Hans Zimmer - Dune Part Two (Original Motion Picture Soundtrack) (2024) [FLAC 24-96]',
		infoHash: fakeHash('zimmer-dune2'),
		engine: 'torbox',
		storage: null,
		subdir: null,
		mode: 'links',
		state: 'ready',
		progress: 1,
		speed: null,
		eta: null,
		error: null,
		retryable: false,
		files: Array.from({ length: 4 }, (_, i) => ({
			path: `Dune Part Two OST/${String(i + 1).padStart(2, '0')} - Track ${i + 1}.flac`,
			size: 86 * MB,
			done: 86 * MB,
			state: 'done',
			url: `/api/jobs/j_seed_links/files/Dune%20Part%20Two%20OST/${String(i + 1).padStart(2, '0')}%20-%20Track%20${i + 1}.flac`
		})),
		external: false
	},
	{
		id: 'x_torbox_5521',
		createdAt: new Date(now - 4 * 86_400_000).toISOString(),
		updatedAt: new Date(now - 4 * 86_400_000).toISOString(),
		name: 'Kaamelott Premier Volet 2021 FRENCH 1080p BluRay x264-LOST',
		infoHash: fakeHash('kaamelott'),
		engine: 'torbox',
		storage: null,
		subdir: null,
		mode: 'links',
		state: 'ready',
		progress: 1,
		speed: null,
		eta: null,
		error: null,
		retryable: false,
		files: [
			{ path: 'Kaamelott.Premier.Volet.2021.FRENCH.1080p.BluRay.x264-LOST.mkv', size: 7.7 * GB, done: 7.7 * GB, state: 'done', url: '/api/jobs/x_torbox_5521/files/Kaamelott.Premier.Volet.2021.FRENCH.1080p.BluRay.x264-LOST.mkv' }
		],
		external: true
	},
	{
		id: 'x_qbit_a1f',
		createdAt: new Date(now - 2 * 86_400_000).toISOString(),
		updatedAt: new Date(now - 60_000).toISOString(),
		name: 'Ubuntu 24.04.2 LTS Desktop amd64',
		infoHash: fakeHash('ubuntu-2404'),
		engine: 'qbit',
		storage: null,
		subdir: null,
		mode: 'adopt',
		state: 'fetching',
		progress: 0.67,
		speed: 12_500_000,
		eta: 160,
		error: null,
		retryable: false,
		files: [
			{ path: 'ubuntu-24.04.2-desktop-amd64.iso', size: 5.9 * GB, done: 3.95 * GB, state: 'copying', url: null }
		],
		external: true
	}
];
