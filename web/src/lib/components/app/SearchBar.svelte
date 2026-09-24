<script lang="ts">
	import SearchIcon from '@lucide/svelte/icons/search';
	import XIcon from '@lucide/svelte/icons/x';
	import StarIcon from '@lucide/svelte/icons/star';
	import MagnetIcon from '@lucide/svelte/icons/magnet';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as ToggleGroup from '$lib/components/ui/toggle-group/index.js';
	import { cn } from '$lib/utils.js';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';
	import IndexerPicker from './IndexerPicker.svelte';
	import FiltersPopover from './FiltersPopover.svelte';
	import HistoryChips from './HistoryChips.svelte';

	const canSearch = $derived(app.query.trim().length > 0 && !!app.caps);
	const saved = $derived(app.isSaved(app.query));

	function submit(e: SubmitEvent) {
		e.preventDefault();
		if (app.running) app.cancel();
		app.search();
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && app.running) {
			e.preventDefault();
			app.cancel();
		}
	}
</script>

<section class="rounded-xl border border-border bg-card p-3 shadow-xs sm:p-4" aria-label={s.searchLabel}>
	<form class="flex flex-col gap-3" onsubmit={submit}>
		<div class="flex gap-2">
			<div class="relative flex-1">
				<SearchIcon class="pointer-events-none absolute top-1/2 left-3 size-4.5 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
				<input
					id="search-input"
					type="search"
					name="q"
					autocomplete="off"
					spellcheck="false"
					enterkeyhint="search"
					aria-label={s.searchLabel}
					placeholder={s.searchPlaceholder}
					bind:value={app.query}
					{onkeydown}
					class="h-11 w-full rounded-lg border border-input bg-background pr-10 pl-10 text-base outline-none transition-colors placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30 [&::-webkit-search-cancel-button]:hidden"
				/>
				<button
					type="button"
					class={cn(
						'absolute top-1/2 right-2 flex size-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
						saved && 'text-warning hover:text-warning',
						!app.query.trim() && 'invisible'
					)}
					aria-pressed={saved}
					aria-label={saved ? s.unsaveSearch : s.saveSearch}
					title={saved ? s.unsaveSearch : s.saveSearch}
					onclick={() => app.toggleSaved(app.query)}
				>
					<StarIcon class={cn('size-4', saved && 'fill-current')} />
				</button>
			</div>
			{#if app.running}
				<Button type="button" variant="outline" size="lg" class="h-11 px-3" onclick={() => app.cancel()} aria-label={s.cancelSearch}>
					<LoaderCircleIcon class="animate-spin motion-reduce:animate-none" />
					<span class="hidden sm:inline">{s.cancel}</span>
					<XIcon class="sm:hidden" />
				</Button>
			{:else}
				<Button type="submit" size="lg" class="h-11 px-4" disabled={!canSearch} aria-label={s.search}>
					<SearchIcon class="sm:hidden" />
					<span class="hidden sm:inline">{s.search}</span>
				</Button>
			{/if}
		</div>

		<div class="flex flex-wrap items-center gap-2">
			<ToggleGroup.Root
				type="multiple"
				variant="outline"
				size="sm"
				spacing={1}
				aria-label={s.categories}
				class="flex-wrap"
				bind:value={() => app.categories, (v) => app.setCategories(v)}
			>
				{#each app.caps?.categories ?? [] as cat (cat)}
					<ToggleGroup.Item value={cat} class="h-7 rounded-full px-2.5 text-xs data-[state=on]:border-foreground/60 data-[state=on]:bg-foreground data-[state=on]:text-background">
						{s.category[cat] ?? cat}
					</ToggleGroup.Item>
				{/each}
			</ToggleGroup.Root>
			<div class="ml-auto flex flex-wrap items-center gap-2">
				<IndexerPicker value={app.indexers} onchange={(v) => app.setIndexers(v)} />
				<FiltersPopover />
				<Button type="button" variant="outline" size="sm" onclick={() => (app.payloadDialogOpen = true)} title={s.pasteOrDrop}>
					<MagnetIcon />
					<span class="hidden md:inline">{s.pasteOrDrop}</span>
					<span class="md:hidden">{s.pasteShort}</span>
				</Button>
			</div>
		</div>
	</form>

	{#if !app.query.trim() && !app.running}
		<div class="mt-3 border-t border-border/70 pt-3 empty:hidden">
			<HistoryChips />
		</div>
	{/if}
</section>
