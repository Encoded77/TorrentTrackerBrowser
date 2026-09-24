<script lang="ts">
	import ZapIcon from '@lucide/svelte/icons/zap';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { app, type ResultRow } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	let { row }: { row: ResultRow } = $props();

	const cachedBy = $derived(app.cachedEnginesFor(row.infoHash));
</script>

{#if row.language}
	<span
		class="inline-flex h-4.5 items-center rounded border border-border px-1 text-[10px] leading-none font-semibold tracking-wide text-muted-foreground"
		title={s.languageLong[row.language] ?? row.language}
	>
		{s.language[row.language] ?? row.language.toUpperCase()}
	</span>
{/if}
{#if row.freeleech}
	<span
		class="inline-flex h-4.5 items-center gap-0.5 rounded border border-freeleech/40 bg-freeleech/10 px-1 text-[10px] leading-none font-semibold text-freeleech"
		title={s.freeleech}
	>
		<ZapIcon class="size-2.5" aria-hidden="true" />
		FL
	</span>
{/if}
{#if cachedBy.length}
	<Tooltip.Root>
		<Tooltip.Trigger
			class="inline-flex h-4.5 items-center gap-0.5 rounded border border-cached/40 bg-cached/10 px-1 text-[10px] leading-none font-semibold text-cached"
		>
			<DatabaseIcon class="size-2.5" aria-hidden="true" />
			{s.cached}
		</Tooltip.Trigger>
		<Tooltip.Content>{cachedBy.map((e) => s.cachedOn(e.name)).join(', ')}</Tooltip.Content>
	</Tooltip.Root>
{/if}
