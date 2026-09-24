<script lang="ts">
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import LockIcon from '@lucide/svelte/icons/lock';
	import GlobeIcon from '@lucide/svelte/icons/globe';
	import CheckIcon from '@lucide/svelte/icons/check';
	import * as Popover from '$lib/components/ui/popover/index.js';
	import * as Command from '$lib/components/ui/command/index.js';
	import { buttonVariants } from '$lib/components/ui/button/index.js';
	import { cn } from '$lib/utils.js';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	let {
		value = [],
		onchange,
		size = 'sm',
		label
	}: { value?: string[]; onchange: (next: string[]) => void; size?: 'sm' | 'xs'; label?: string } = $props();

	let open = $state(false);
	const sources = $derived(app.caps?.sources ?? []);
	const total = $derived(app.allIndexers.length);
	const summary = $derived(
		value.length === 0 || value.length === total ? (label ?? s.allIndexers) : s.indexersSelected(value.length)
	);

	function toggle(id: string) {
		onchange(value.includes(id) ? value.filter((v) => v !== id) : [...value, id]);
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		class={cn(buttonVariants({ variant: 'outline', size }), 'font-normal')}
		aria-label={s.indexers}
	>
		<span class="truncate">{summary}</span>
		<ChevronDownIcon class="text-muted-foreground" />
	</Popover.Trigger>
	<Popover.Content class="w-72 p-0" align="start">
		<Command.Root>
			<Command.Input placeholder={s.indexerSearch} />
			<Command.List class="max-h-72">
				<Command.Empty>{s.noIndexer}</Command.Empty>
				{#each sources as source (source.id)}
					<Command.Group heading={source.name}>
						{#each source.indexers as ix (ix.id)}
							{@const on = value.includes(ix.id)}
							<Command.Item value={`${source.name} ${ix.name} ${ix.id}`} onSelect={() => toggle(ix.id)} data-checked={on}>
								<span
									class={cn(
										'flex size-4 items-center justify-center rounded-[4px] border',
										on ? 'border-primary bg-primary text-primary-foreground' : 'border-input'
									)}
									aria-hidden="true"
								>
									{#if on}<CheckIcon class="size-3" />{/if}
								</span>
								<span class="flex-1 truncate">{ix.name}</span>
								<span class="flex items-center gap-1 text-[11px] text-muted-foreground">
									{#if ix.private}<LockIcon class="size-3" />{s.private}{:else}<GlobeIcon class="size-3" />{s.public}{/if}
								</span>
							</Command.Item>
						{/each}
					</Command.Group>
				{/each}
			</Command.List>
			<div class="flex items-center justify-between border-t px-2 py-1.5 text-xs">
				<button type="button" class="rounded px-1 text-muted-foreground hover:text-foreground" onclick={() => onchange([])}>
					{s.selectAll}
				</button>
				<span class="text-muted-foreground">{value.length ? s.indexersSelected(value.length) : s.allIndexers}</span>
			</div>
		</Command.Root>
	</Popover.Content>
</Popover.Root>
