<script lang="ts">
	import SearchIcon from '@lucide/svelte/icons/search';
	import SearchXIcon from '@lucide/svelte/icons/search-x';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';
	import Header from '$lib/components/app/Header.svelte';
	import SearchBar from '$lib/components/app/SearchBar.svelte';
	import StatusStrip from '$lib/components/app/StatusStrip.svelte';
	import ResultsTable from '$lib/components/app/ResultsTable.svelte';
	import EmptyState from '$lib/components/app/EmptyState.svelte';
	import DownloadDialog from '$lib/components/app/DownloadDialog.svelte';
	import PayloadDialog from '$lib/components/app/PayloadDialog.svelte';
	import QueueSheet from '$lib/components/app/QueueSheet.svelte';

	const initialQuery = new URLSearchParams(location.search).get('q')?.trim() ?? '';
	if (initialQuery) app.query = initialQuery;
	void app.loadCapabilities().then(() => {
		if (initialQuery && app.caps) app.search(initialQuery);
	});

	function isEditable(target: EventTarget | null): boolean {
		if (!(target instanceof HTMLElement)) return false;
		return target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName);
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey && !isEditable(e.target)) {
			e.preventDefault();
			const input = document.getElementById('search-input') as HTMLInputElement | null;
			input?.focus();
			input?.select();
		} else if (e.key === 'Escape' && app.running && !isEditable(e.target)) {
			app.cancel();
		}
	}
</script>

<svelte:window {onkeydown} />

<a
	href="#main"
	class="sr-only z-50 rounded-md bg-background px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus-visible:ring-3 focus-visible:ring-ring/50"
>
	{s.skipToContent}
</a>

<Header />

<main id="main" class="mx-auto flex w-full max-w-[1400px] flex-col gap-3 px-4 py-4 sm:px-6">
	<SearchBar />

	{#if app.capsLoading}
		<div class="flex flex-col gap-2" aria-busy="true" aria-label={s.loadingCapabilities}>
			<Skeleton class="h-6 w-64" />
			<Skeleton class="h-9" />
			<Skeleton class="h-9" />
			<Skeleton class="h-9" />
		</div>
	{:else if app.capsError}
		<EmptyState icon={WifiOffIcon} title={s.capabilitiesErrorTitle} body={s.capabilitiesErrorBody} hint={app.capsError} tone="error">
			<Button size="sm" variant="outline" onclick={() => app.loadCapabilities()}>{s.retry}</Button>
		</EmptyState>
	{:else if app.searchedQuery === null}
		<EmptyState icon={SearchIcon} title={s.emptyTitle} body={s.emptyBody} hint={s.emptyHint} />
	{:else}
		<StatusStrip />
		{#if app.searchError}
			<p class="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-[13px] text-destructive" role="alert">
				{app.searchError}
			</p>
		{/if}
		{#if app.visibleRows.length}
			<ResultsTable />
		{:else if app.running}
			<div class="flex flex-col gap-1.5" aria-busy="true">
				{#each [1, 2, 3, 4, 5] as i (i)}
					<Skeleton class="h-9 rounded-md" />
				{/each}
			</div>
		{:else if app.rows.length}
			<EmptyState icon={SearchXIcon} title={s.noResultsTitle} body={s.noResultsFiltered(app.hiddenCount)}>
				<Button size="sm" variant="outline" onclick={() => app.resetFilters()}>{s.resetFilters}</Button>
			</EmptyState>
		{:else}
			<EmptyState icon={SearchXIcon} title={s.noResultsTitle} body={s.noResultsBody} />
		{/if}
	{/if}
</main>

<DownloadDialog />
<PayloadDialog />
<QueueSheet />
