<script lang="ts">
	import FunnelIcon from '@lucide/svelte/icons/funnel';
	import RotateCcwIcon from '@lucide/svelte/icons/rotate-ccw';
	import * as Popover from '$lib/components/ui/popover/index.js';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import * as ToggleGroup from '$lib/components/ui/toggle-group/index.js';
	import { cn } from '$lib/utils.js';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';
	import IndexerPicker from './IndexerPicker.svelte';

	const f = $derived(app.filters);
	const count = $derived(app.activeFilterCount);

	function num(value: string): number | null {
		const n = Number.parseFloat(value.replace(',', '.'));
		return Number.isFinite(n) && n >= 0 ? n : null;
	}
</script>

<Popover.Root>
	<Popover.Trigger
		class={cn(buttonVariants({ variant: count ? 'secondary' : 'outline', size: 'sm' }), 'font-normal')}
		aria-label={count ? s.filtersActive(count) : s.filters}
	>
		<FunnelIcon />
		<span>{s.filters}</span>
		{#if count}
			<span class="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-foreground px-1 text-[10px] font-semibold text-background">{count}</span>
		{/if}
	</Popover.Trigger>
	<Popover.Content class="w-80 gap-3 p-3" align="end">
		<div class="flex items-center justify-between">
			<span class="text-sm font-medium">{s.filters}</span>
			<Button variant="ghost" size="xs" onclick={() => app.resetFilters()} disabled={!count}>
				<RotateCcwIcon />
				{s.resetFilters}
			</Button>
		</div>

		<div class="grid grid-cols-2 gap-x-3 gap-y-2.5 text-xs">
			<label class="flex flex-col gap-1">
				<span class="text-muted-foreground">{s.minSeeders}</span>
				<Input
					type="number"
					name="minSeeders"
					autocomplete="off"
					min="0"
					inputmode="numeric"
					class="h-7 text-xs md:text-xs"
					value={f.minSeeders || ''}
					oninput={(e) => app.setFilters({ minSeeders: num(e.currentTarget.value) ?? 0 })}
				/>
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-muted-foreground">{s.maxAge}</span>
				<Input
					type="number"
					name="maxAgeDays"
					autocomplete="off"
					min="0"
					inputmode="numeric"
					class="h-7 text-xs md:text-xs"
					placeholder={s.anyValue}
					value={f.maxAgeDays ?? ''}
					oninput={(e) => app.setFilters({ maxAgeDays: num(e.currentTarget.value) })}
				/>
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-muted-foreground">{s.sizeMin}</span>
				<Input
					type="number"
					name="sizeMinGb"
					autocomplete="off"
					min="0"
					step="0.1"
					inputmode="decimal"
					class="h-7 text-xs md:text-xs"
					placeholder={s.anyValue}
					value={f.sizeMinGb ?? ''}
					oninput={(e) => app.setFilters({ sizeMinGb: num(e.currentTarget.value) })}
				/>
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-muted-foreground">{s.sizeMax}</span>
				<Input
					type="number"
					name="sizeMaxGb"
					autocomplete="off"
					min="0"
					step="0.1"
					inputmode="decimal"
					class="h-7 text-xs md:text-xs"
					placeholder={s.anyValue}
					value={f.sizeMaxGb ?? ''}
					oninput={(e) => app.setFilters({ sizeMaxGb: num(e.currentTarget.value) })}
				/>
			</label>
		</div>

		<div class="flex flex-col gap-1 text-xs">
			<span class="text-muted-foreground">{s.languages}</span>
			<ToggleGroup.Root
				type="multiple"
				variant="outline"
				size="sm"
				spacing={1}
				class="flex-wrap"
				bind:value={() => f.languages, (v) => app.setFilters({ languages: v })}
			>
				{#each app.caps?.languages ?? [] as lang (lang)}
					<ToggleGroup.Item value={lang} class="h-6 px-2 text-[11px] data-[state=on]:bg-foreground data-[state=on]:text-background" aria-label={s.languageLong[lang] ?? lang}>
						{s.language[lang] ?? lang}
					</ToggleGroup.Item>
				{/each}
			</ToggleGroup.Root>
		</div>

		<div class="flex flex-col gap-1 text-xs">
			<span class="text-muted-foreground">{s.filterCategory}</span>
			<ToggleGroup.Root
				type="multiple"
				variant="outline"
				size="sm"
				spacing={1}
				class="flex-wrap"
				bind:value={() => f.categories, (v) => app.setFilters({ categories: v })}
			>
				{#each app.caps?.categories ?? [] as cat (cat)}
					<ToggleGroup.Item value={cat} class="h-6 px-2 text-[11px] data-[state=on]:bg-foreground data-[state=on]:text-background">
						{s.category[cat] ?? cat}
					</ToggleGroup.Item>
				{/each}
			</ToggleGroup.Root>
		</div>

		<div class="flex items-center justify-between gap-2 text-xs">
			<span class="text-muted-foreground">{s.filterIndexer}</span>
			<IndexerPicker size="xs" value={f.indexers} onchange={(v) => app.setFilters({ indexers: v })} />
		</div>

		{#if app.cachedEngines.length}
			<label class="flex items-center gap-2 text-xs">
				<Checkbox checked={f.cachedOnly} onCheckedChange={(v) => app.setFilters({ cachedOnly: v === true })} />
				<span>{s.cachedOnly}</span>
			</label>
		{/if}
	</Popover.Content>
</Popover.Root>
