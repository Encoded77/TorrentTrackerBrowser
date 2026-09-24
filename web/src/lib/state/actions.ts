// Row actions shared by the table (desktop) and the card list (phone).

import { toast } from 'svelte-sonner';
import { api, errorMessage } from '$lib/api/client';
import type { Result } from '$lib/api/types';
import { s } from '$lib/strings';
import type { ResultRow } from './app.svelte';

function pick(row: ResultRow, cap: 'hasMagnet' | 'hasTorrent'): Result | null {
	if (row.primary[cap]) return row.primary;
	return row.sources.find((r) => r[cap]) ?? null;
}

export function magnetSource(row: ResultRow): Result | null {
	return pick(row, 'hasMagnet');
}

export function torrentSource(row: ResultRow): Result | null {
	return pick(row, 'hasTorrent');
}

export async function copyMagnet(row: ResultRow): Promise<void> {
	const src = magnetSource(row);
	if (!src) return;
	try {
		const magnet = await api.magnet(src.id);
		await navigator.clipboard.writeText(magnet.trim());
		toast.success(s.magnetCopied);
	} catch (e) {
		toast.error(errorMessage(e, s.copyFailed));
	}
}

export function downloadTorrent(row: ResultRow): void {
	const src = torrentSource(row);
	if (!src) return;
	const a = document.createElement('a');
	a.href = api.payloadUrl(src.id, 'torrent');
	a.download = '';
	a.rel = 'noopener';
	document.body.appendChild(a);
	a.click();
	a.remove();
}

export function openInfoPage(row: ResultRow): void {
	const url = row.sources.find((r) => r.infoUrl)?.infoUrl;
	if (url) window.open(url, '_blank', 'noopener');
}
