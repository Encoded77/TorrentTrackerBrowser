// Active UI locale: persisted in localStorage under `locale`, initialised from the stored
// value, else from the browser language (fr* -> fr, anything else -> en).

export type Locale = 'fr' | 'en';

export const LOCALES: Locale[] = ['fr', 'en'];
const KEY = 'locale';

function isLocale(v: unknown): v is Locale {
	return v === 'fr' || v === 'en';
}

function initial(): Locale {
	try {
		const stored = localStorage.getItem(KEY);
		if (isLocale(stored)) return stored;
	} catch {
		// storage unavailable: fall through to the browser language
	}
	const lang = typeof navigator !== 'undefined' ? (navigator.language ?? '') : '';
	return lang.toLowerCase().startsWith('fr') ? 'fr' : 'en';
}

function applyLang(next: Locale) {
	if (typeof document !== 'undefined') document.documentElement.lang = next;
}

class LocaleState {
	current = $state<Locale>(initial());

	constructor() {
		applyLang(this.current);
	}

	set(next: Locale) {
		if (!isLocale(next) || next === this.current) return;
		this.current = next;
		applyLang(next);
		try {
			localStorage.setItem(KEY, next);
		} catch {
			// private mode or quota: the choice lasts for the session only
		}
	}
}

export const locale = new LocaleState();
