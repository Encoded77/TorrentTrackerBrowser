// Locale-aware formatting. Units, separators and relative ages come from the active
// dictionary (`s`), numbers and dates go through Intl with the dictionary's locale tag.
import { s } from './strings';

const numberFormats = new Map<string, Intl.NumberFormat>();

function formatNumber(value: number, digits: number): string {
	const key = `${s.localeTag}|${digits}`;
	let nf = numberFormats.get(key);
	if (!nf) {
		nf = new Intl.NumberFormat(s.localeTag, { maximumFractionDigits: digits, minimumFractionDigits: digits });
		numberFormats.set(key, nf);
	}
	return nf.format(value);
}

/** Bytes to a compact unit string ("1,4 Go" in French, "1.4 GB" in English). */
export function humanSize(bytes: number | null | undefined): string {
	if (bytes == null || !Number.isFinite(bytes) || bytes < 0) return '-';
	const units = s.sizeUnits;
	let value = bytes;
	let unit = 0;
	while (value >= 1000 && unit < units.length - 1) {
		value /= 1000;
		unit++;
	}
	const digits = unit === 0 ? 0 : value < 10 ? 2 : value < 100 ? 1 : 0;
	return s.sizeWithUnit(formatNumber(value, digits), units[unit]);
}

export function humanSpeed(bytesPerSecond: number | null | undefined): string {
	if (bytesPerSecond == null) return '';
	return s.speed(humanSize(bytesPerSecond));
}

/** Seconds to "45 s", "2 min", "1 h 20", "3 j" / "3 d". */
export function humanDuration(seconds: number | null | undefined): string {
	if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '';
	if (seconds < 60) return s.durSeconds(Math.round(seconds));
	const m = Math.floor(seconds / 60);
	if (m < 60) return s.durMinutes(m);
	const h = Math.floor(m / 60);
	if (h < 24) return s.durHours(h, m % 60);
	return s.durDays(Math.floor(h / 24));
}

/** RFC 3339 date to a relative age ("3 j", "2 h", "1 an" / "1 year"). */
export function humanAge(iso: string, now: number = Date.now()): string {
	const t = Date.parse(iso);
	if (Number.isNaN(t)) return '-';
	const diff = Math.max(0, now - t) / 1000;
	if (diff < 3600) return s.durMinutes(Math.max(1, Math.floor(diff / 60)));
	if (diff < 86400) return s.durHours(Math.floor(diff / 3600), 0);
	const days = Math.floor(diff / 86400);
	if (days < 30) return s.durDays(days);
	if (days < 365) return s.ageMonths(Math.floor(days / 30));
	return s.ageYears(Math.floor(days / 365));
}

export function fullDate(iso: string): string {
	const t = Date.parse(iso);
	if (Number.isNaN(t)) return iso;
	return new Intl.DateTimeFormat(s.localeTag, { dateStyle: 'long', timeStyle: 'short' }).format(new Date(t));
}

export function percent(fraction: number | null | undefined): string {
	if (fraction == null) return '';
	return s.percent(Math.round(Math.min(1, Math.max(0, fraction)) * 100));
}

/**
 * Client-side mirror of the server rule for subfolders: clean the path, reject
 * absolute paths and `..` segments, strip characters no filesystem accepts.
 * Returns '' when nothing usable remains.
 */
export function sanitizeSubdir(input: string): string {
	const cleaned = input
		.replace(/\\/g, '/')
		// eslint-disable-next-line no-control-regex
		.replace(/[<>:"|?*\u0000-\u001f]/g, '')
		.split('/')
		.map((seg) => seg.trim())
		.filter((seg) => seg !== '' && seg !== '.' && seg !== '..')
		.join('/');
	return cleaned.slice(0, 200);
}

const LANGUAGE_PATTERNS: [RegExp, string][] = [
	[/\b(multi|multi[- ]?vf|truefrench)\b/i, 'multi'],
	[/\b(vostfr|vost)\b/i, 'vostfr'],
	[/\b(french|vff|vfq|vf2|vf|fr)\b/i, 'fr'],
	[/\b(english|eng|en)\b/i, 'en']
];

/** Fallback when the server did not tag the language. */
export function guessLanguage(title: string): string | null {
	for (const [re, lang] of LANGUAGE_PATTERNS) if (re.test(title)) return lang;
	return null;
}

export type SeedersTone = 'high' | 'mid' | 'low' | 'none';

export function seedersTone(seeders: number): SeedersTone {
	if (seeders >= 20) return 'high';
	if (seeders >= 5) return 'mid';
	if (seeders >= 1) return 'low';
	return 'none';
}

export function magnetHash(magnet: string): string | null {
	const m = /xt=urn:btih:([a-z0-9]{32,40})/i.exec(magnet);
	return m ? m[1].toLowerCase() : null;
}

export function magnetName(magnet: string): string | null {
	const m = /[?&]dn=([^&]+)/i.exec(magnet);
	if (!m) return null;
	try {
		return decodeURIComponent(m[1].replace(/\+/g, ' '));
	} catch {
		return m[1];
	}
}
