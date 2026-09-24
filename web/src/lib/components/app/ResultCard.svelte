<script lang="ts">
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { cn } from '$lib/utils.js';
	import { app, type ResultRow } from '$lib/state/app.svelte';
	import { fullDate, humanAge, humanSize, seedersTone } from '$lib/format';
	import { s } from '$lib/strings';
	import RowActions from './RowActions.svelte';
	import RowBadges from './RowBadges.svelte';
	import FilePreview from './FilePreview.svelte';

	let { row }: { row: ResultRow } = $props();

	const selected = $derived(app.selection.has(row.key));
	const expanded = $derived(app.expandedKey === row.key);
	const tone = $derived(seedersTone(row.seeders));
	const toneClass: Record<ReturnType<typeof seedersTone>, string> = {
		high: 'text-seed-high',
		mid: 'text-seed-mid',
		low: 'text-foreground',
		none: 'text-muted-foreground'
	};
</script>

<li class={cn('rounded-lg border border-border bg-card p-2.5 text-[13px]', selected && 'border-foreground/40 bg-muted/40')}>
	<div class="flex items-start gap-2">
		<Checkbox class="mt-0.5" checked={selected} onCheckedChange={(v) => app.toggleRow(row.key, v === true)} aria-label={s.selectRow} />
		<div class="min-w-0 flex-1">
			<p class="line-clamp-2 leading-snug font-medium break-words" translate="no">{row.title}</p>
			<div class="mt-1 flex flex-wrap items-center gap-1">
				<RowBadges {row} />
				{#each row.indexers as id (id)}
					<span class="inline-flex h-4.5 items-center rounded border border-border px-1 text-[10px] text-muted-foreground">{app.indexerName(id)}</span>
				{/each}
			</div>
			<dl class="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs tabular-nums text-muted-foreground">
				<div><dt class="sr-only">{s.colSize}</dt><dd class="text-foreground">{humanSize(row.size)}</dd></div>
				<div class="flex gap-1">
					<dt class="sr-only">{s.colSeeders}</dt>
					<dd class={cn('font-semibold', toneClass[tone])}>{row.seeders}</dd>
					<span aria-hidden="true">/</span>
					<dt class="sr-only">{s.colLeechers}</dt>
					<dd>{row.leechers}</dd>
				</div>
				<div><dt class="sr-only">{s.colAge}</dt><dd title={fullDate(row.published)}>{s.ago(humanAge(row.published))}</dd></div>
				<div><dt class="sr-only">{s.colGrabs}</dt><dd>{row.grabs} {s.grabs}</dd></div>
			</dl>
		</div>
		<RowActions {row} class="-mr-1 -mt-1 flex-col" />
	</div>
	<button
		type="button"
		class="mt-1.5 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
		aria-expanded={expanded}
		onclick={() => app.toggleExpanded(row)}
	>
		<ChevronRightIcon class={cn('size-3.5 transition-transform motion-reduce:transition-none', expanded && 'rotate-90')} />
		{expanded ? s.hidePreview : s.previewFiles}
	</button>
	{#if expanded}
		<div class="mt-2">
			<FilePreview {row} />
		</div>
	{/if}
</li>
