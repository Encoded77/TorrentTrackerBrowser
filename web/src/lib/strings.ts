// `s` is the active dictionary. Every property read goes through the `$state` locale, so a
// template, `$derived` or `$effect` that reads `s.something` re-runs when the language changes.
// Values read outside a reactive context (a stored error message, a module-level constant) are
// snapshots in the language active at that moment.

import { en } from './i18n/en';
import { fr, type Dictionary } from './i18n/fr';
import { locale, type Locale } from './i18n/locale.svelte';

const dictionaries: Record<Locale, Dictionary> = { fr, en };

export const s: Dictionary = new Proxy(fr, {
	get(_target, key) {
		return Reflect.get(dictionaries[locale.current], key);
	}
});

export type { Dictionary };
