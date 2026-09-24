<script lang="ts">
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpDownIcon from '@lucide/svelte/icons/arrow-up-down';
	import LockIcon from '@lucide/svelte/icons/lock';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import XIcon from '@lucide/svelte/icons/x';
	import * as Table from '$lib/components/ui/table/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { cn } from '$lib/utils.js';
	import { app, type ResultRow, type SortKey } from '$lib/state/app.svelte';
	import { fullDate, humanAge, humanSize, seedersTone } from '$lib/format';
	import { s } from '$lib/strings';
	import RowActions from './RowActions.svelte';
	import RowBadges from './RowBadges.svelte';
	import FilePreview from './FilePreview.svelte';
	import ResultCard from './ResultCard.svelte';

	const rows = $derived(app.visibleRows);
	const allSelected = $derived(rows.length > 0 && rows.every((r) => app.selection.has(r.key)));
	const someSelected = $derived(!allSelected && rows.some((r) => app.selection.has(r.key)));
	const selectedCount = $derived(app.selectedRows.length);

	// Derived so the headers re-render when the language changes.
	const columns = $derived<{ key: SortKey | null; label: string; class: string }[]>([
		{ key: 'title', label: s.colTitle, class: 'w-full text-left' },
		{ key: null, label: s.colIndexer, class: 'text-left' },
		{ key: 'size', label: s.colSize, class: 'text-right' },
		{ key: 'seeders', label: s.colSeeders, class: 'text-right' },
		{ key: null, label: s.colLeechers, class: 'text-right' },
		{ key: 'published', label: s.colAge, class: 'text-right' },
		{ key: null, label: s.colGrabs, class: 'text-right' }
	]);

	const toneClass: Record<ReturnType<typeof seedersTone>, string> = {
		high: 'text-seed-high',
		mid: 'text-seed-mid',
		low: 'text-foreground',
		none: 'text-muted-foreground'
	};

	function ariaSort(key: SortKey | null): 'ascending' | 'descending' | 'none' {
		if (!key || app.sort.key !== key) return 'none';
		return app.sort.dir === 'asc' ? 'ascending' : 'descending';
	}

	function onRowKeydown(e: KeyboardEvent, row: ResultRow) {
		if (e.target !== e.currentTarget) return;
		if (e.key === 'Enter') {
			e.preventDefault();
			app.openDownload([row]);
		} else if (e.key === ' ') {
			e.preventDefault();
			app.toggleRow(row.key);
		}
	}
</script>

{#if selectedCount}
	<div
		class="sticky top-12 z-20 -mx-1 mb-2 flex items-center gap-2 rounded-lg border border-border bg-popover/95 px-3 py-1.5 text-[13px] shadow-sm backdrop-blur"
		role="region"
		aria-label={s.downloadSelection(selectedCount)}
	>
		<span class="tabular-nums">{selectedCount}</span>
		<Button size="sm" onclick={() => app.openDownload(app.selectedRows)}>
			<DownloadIcon />
			{s.downloadSelection(selectedCount)}
		</Button>
		<Button size="sm" variant="ghost" onclick={() => app.toggleAllVisible(false)}>
			<XIcon />
			{s.clearSelection}
		</Button>
	</div>
{/if}

<!-- Desktop: dense table -->
<div class="hidden md:block">
	<table class="w-full caption-bottom text-[13px]">
		<Table.Header class="sticky top-12 z-10 bg-background shadow-[inset_0_-1px_0_var(--border)]">
			<Table.Row class="hover:bg-transparent">
				<Table.Head class="w-8 pr-0 pl-1">
					<Checkbox
						checked={allSelected}
						indeterminate={someSelected}
						onCheckedChange={(v) => app.toggleAllVisible(v === true)}
						aria-label={s.selectAllRows}
					/>
				</Table.Head>
				<Table.Head class="w-6 px-0"><span class="sr-only">{s.previewFiles}</span></Table.Head>
				{#each columns as col, i (i)}
					<Table.Head class={cn('h-9 text-xs font-medium text-muted-foreground', col.class)} aria-sort={col.key ? ariaSort(col.key) : undefined}>
						{#if col.key}
							{@const active = app.sort.key === col.key}
							<button
								type="button"
								class={cn(
									'inline-flex items-center gap-1 rounded px-1 -mx-1 hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
									active && 'text-foreground'
								)}
								onclick={() => app.setSort(col.key!)}
								aria-label={s.sortBy(col.label)}
							>
								{col.label}
								{#if active}
									{#if app.sort.dir === 'asc'}<ArrowUpIcon class="size-3" />{:else}<ArrowDownIcon class="size-3" />{/if}
								{:else}
									<ArrowUpDownIcon class="size-3 opacity-40" />
								{/if}
							</button>
						{:else}
							{col.label}
						{/if}
					</Table.Head>
				{/each}
				<Table.Head class="w-20 text-right"><span class="sr-only">{s.colActions}</span></Table.Head>
			</Table.Row>
		</Table.Header>
		<Table.Body>
			{#each rows as row (row.key)}
				{@const selected = app.selection.has(row.key)}
				{@const expanded = app.expandedKey === row.key}
				{@const tone = seedersTone(row.seeders)}
				<Table.Row
					class={cn('group h-9 scroll-mt-24 border-border/70', expanded && 'border-b-0 bg-muted/40')}
					data-state={selected ? 'selected' : undefined}
					tabindex={0}
					onkeydown={(e) => onRowKeydown(e, row)}
				>
					<Table.Cell class="pr-0 pl-1 py-0">
						<Checkbox checked={selected} onCheckedChange={(v) => app.toggleRow(row.key, v === true)} aria-label={s.selectRow} />
					</Table.Cell>
					<Table.Cell class="px-0 py-0">
						<button
							type="button"
							class="flex size-6 items-center justify-center rounded text-muted-foreground hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
							aria-expanded={expanded}
							aria-label={expanded ? s.hidePreview : s.previewFiles}
							onclick={() => app.toggleExpanded(row)}
						>
							<ChevronRightIcon class={cn('size-3.5 transition-transform motion-reduce:transition-none', expanded && 'rotate-90')} />
						</button>
					</Table.Cell>
					<Table.Cell class="max-w-0 py-1">
						<div class="flex min-w-0 items-center gap-1.5">
							<span class="min-w-0 truncate font-medium" title={row.title} translate="no">{row.title}</span>
							<RowBadges {row} />
						</div>
					</Table.Cell>
					<Table.Cell class="py-1">
						<div class="flex items-center gap-1">
							{#each row.indexers as id (id)}
								{@const ix = app.indexerById.get(id)}
								<span class="inline-flex h-5 items-center gap-0.5 rounded-md border border-border bg-card px-1.5 text-[11px] text-muted-foreground">
									{#if ix?.private}<LockIcon class="size-2.5" aria-label={s.private} />{/if}
									{app.indexerName(id)}
								</span>
							{/each}
						</div>
					</Table.Cell>
					<Table.Cell class="py-1 text-right tabular-nums">{humanSize(row.size)}</Table.Cell>
					<Table.Cell class={cn('py-1 text-right font-semibold tabular-nums', toneClass[tone])}>{row.seeders}</Table.Cell>
					<Table.Cell class="py-1 text-right tabular-nums text-muted-foreground">{row.leechers}</Table.Cell>
					<Table.Cell class="py-1 text-right tabular-nums text-muted-foreground">
						<Tooltip.Root>
							<Tooltip.Trigger class="rounded px-0.5 underline decoration-dotted decoration-border underline-offset-2">{humanAge(row.published)}</Tooltip.Trigger>
							<Tooltip.Content>{s.publishedOn} {fullDate(row.published)}</Tooltip.Content>
						</Tooltip.Root>
					</Table.Cell>
					<Table.Cell class="py-1 text-right tabular-nums text-muted-foreground">{row.grabs}</Table.Cell>
					<Table.Cell class="py-0 pr-1">
						<RowActions {row} class="opacity-60 group-hover:opacity-100 group-focus-within:opacity-100 has-[[aria-expanded=true]]:opacity-100" />
					</Table.Cell>
				</Table.Row>
				{#if expanded}
					<Table.Row class="bg-muted/40 hover:bg-muted/40">
						<Table.Cell colspan={10} class="py-2 pr-3 pl-14 whitespace-normal">
							<FilePreview {row} />
						</Table.Cell>
					</Table.Row>
				{/if}
			{/each}
		</Table.Body>
	</table>
</div>

<!-- Phone: card list -->
<ul class="flex flex-col gap-2 md:hidden">
	{#each rows as row (row.key)}
		<ResultCard {row} />
	{/each}
</ul>
