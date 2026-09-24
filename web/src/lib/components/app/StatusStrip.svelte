<script lang="ts">
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import CheckIcon from '@lucide/svelte/icons/check';
	import TriangleAlertIcon from '@lucide/svelte/icons/triangle-alert';
	import XIcon from '@lucide/svelte/icons/x';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { cn } from '$lib/utils.js';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	const entries = $derived(Object.entries(app.indexerStatus));
	const summary = $derived(
		app.running
			? s.searching
			: app.searchError
				? s.searchErrorTitle
				: app.elapsedMs != null
					? s.searchDone(app.elapsedMs)
					: ''
	);
</script>

<div class="flex flex-wrap items-center gap-x-3 gap-y-2 text-xs" aria-live="polite" role="status">
	<div class="flex flex-wrap items-center gap-1.5">
		{#each entries as [id, st] (id)}
			{@const name = app.indexerName(id)}
			{#if st.state === 'failed'}
				<Tooltip.Root>
					<Tooltip.Trigger
						class="inline-flex h-6 items-center gap-1.5 rounded-full border border-destructive/40 bg-destructive/10 px-2 text-destructive"
						aria-label={s.indexerFailed(name)}
					>
						<TriangleAlertIcon class="size-3" aria-hidden="true" />
						<span>{name}</span>
						<span class="sr-only">{s.failed}</span>
					</Tooltip.Trigger>
					<Tooltip.Content>{st.error ?? s.indexerFailed(name)}</Tooltip.Content>
				</Tooltip.Root>
			{:else}
				<span
					class={cn(
						'inline-flex h-6 items-center gap-1.5 rounded-full border px-2 tabular-nums transition-colors',
						st.state === 'done' ? 'border-border bg-card text-foreground' : 'border-dashed border-border text-muted-foreground',
						st.state === 'cancelled' && 'line-through opacity-70'
					)}
				>
					{#if st.state === 'cancelled'}
						<span>{name}</span>
						<span class="sr-only">{s.cancel}</span>
					{:else if st.state === 'pending'}
						<LoaderCircleIcon class="size-3 animate-spin motion-reduce:animate-none" aria-hidden="true" />
						<span>{name}</span>
						<span class="sr-only">{s.pending}</span>
					{:else}
						<CheckIcon class="size-3 text-seed-high" aria-hidden="true" />
						<span>{name}</span>
						<span class="text-muted-foreground">{st.count}</span>
						<span class="text-muted-foreground/70">{s.elapsedMs(st.elapsedMs)}</span>
					{/if}
				</span>
			{/if}
		{/each}
	</div>
	<div class="ml-auto flex items-center gap-2 text-muted-foreground">
		<span class="tabular-nums">{s.resultsShown(app.visibleRows.length, app.rows.length)}</span>
		{#if summary}<span aria-hidden="true">·</span><span>{summary}</span>{/if}
		{#if app.running}
			<Button variant="outline" size="xs" onclick={() => app.cancel()}>
				<XIcon />
				{s.cancelSearch}
			</Button>
		{/if}
	</div>
</div>
