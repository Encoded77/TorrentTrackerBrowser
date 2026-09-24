// localStorage helpers: every access is guarded, private mode and quota errors are silent.

const PREFIX = 'ttb.';

export function load<T>(key: string, fallback: T): T {
	try {
		const raw = localStorage.getItem(PREFIX + key);
		if (raw === null) return fallback;
		const parsed = JSON.parse(raw) as unknown;
		if (parsed === null || typeof parsed !== typeof fallback) return fallback;
		if (Array.isArray(fallback) !== Array.isArray(parsed)) return fallback;
		if (typeof fallback === 'object' && !Array.isArray(fallback)) {
			return { ...fallback, ...(parsed as object) } as T;
		}
		return parsed as T;
	} catch {
		return fallback;
	}
}

export function save(key: string, value: unknown): void {
	try {
		localStorage.setItem(PREFIX + key, JSON.stringify(value));
	} catch {
		// quota or private mode: preferences simply do not persist
	}
}
