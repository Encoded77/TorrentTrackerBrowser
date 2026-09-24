import { api } from './client';
import type { BatchEvent, DoneEvent } from './types';

export interface SearchHandlers {
	onBatch: (batch: BatchEvent) => void;
	onDone: (done: DoneEvent) => void;
	/** Transport failure before `done` (the server dropped, or the network did). */
	onError: () => void;
}

export interface SearchStream {
	cancel: () => void;
}

/**
 * Opens the /api/search event stream. The stream is closed on `done`, on
 * error and on cancel(); closing it cancels the fan-out server-side.
 */
export function openSearch(
	q: string,
	cat: string[],
	indexers: string[],
	handlers: SearchHandlers
): SearchStream {
	const es = new EventSource(api.searchUrl(q, cat, indexers));
	let finished = false;

	const close = () => {
		if (finished) return;
		finished = true;
		es.close();
	};

	es.addEventListener('batch', (ev) => {
		if (finished) return;
		try {
			handlers.onBatch(JSON.parse((ev as MessageEvent).data) as BatchEvent);
		} catch {
			handlers.onError();
			close();
		}
	});

	es.addEventListener('done', (ev) => {
		if (finished) return;
		try {
			handlers.onDone(JSON.parse((ev as MessageEvent).data) as DoneEvent);
		} finally {
			close();
		}
	});

	es.onerror = () => {
		if (finished) return;
		// EventSource would reconnect and restart the search server-side; stop instead.
		handlers.onError();
		close();
	};

	return { cancel: close };
}
