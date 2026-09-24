<script lang="ts">
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { app, type ResultRow } from '$lib/state/app.svelte';
	import { humanSize } from '$lib/format';
	import { s } from '$lib/strings';

	let { row }: { row: ResultRow } = $props();

	const id = $derived(row.primary.id);
	const state = $derived(app.filesByResult[id]);
	const selected = $derived(app.fileSelection[id] ?? []);
	const files = $derived(state?.state === 'ok' ? state.files : []);
	const allOn = $derived(files.length > 0 && selected.length === files.length);
	const someOn = $derived(selected.length > 0 && !allOn);
	const selectedSize = $derived(files.filter((f) => selected.includes(f.index)).reduce((a, f) => a + f.size, 0));

	function toggleAll(on: boolean) {
		app.fileSelection[id] = on ? files.map((f) => f.index) : [];
	}
</script>

<div class="rounded-lg border border-border bg-muted/30 text-[13px]">
	{#if !state || state.state === 'loading'}
		<div class="flex items-center gap-2 px-3 py-2 text-muted-foreground">
			<LoaderCircleIcon class="size-3.5 animate-spin motion-reduce:animate-none" aria-hidden="true" />
			{s.filesLoading}
		</div>
	{:else if state.state === 'error'}
		<div class="flex items-center gap-2 px-3 py-2 text-muted-foreground">
			<CircleAlertIcon class="size-3.5 text-destructive" aria-hidden="true" />
			{state.message.includes('no_preview') || state.message.includes('cached') ? s.noPreview : state.message}
		</div>
	{:else}
		<div class="flex items-center gap-3 border-b border-border/70 px-3 py-1.5">
			<label class="flex items-center gap-2">
				<Checkbox checked={allOn} indeterminate={someOn} onCheckedChange={(v) => toggleAll(v === true)} aria-label={s.allFiles} />
				<span class="text-xs text-muted-foreground">{s.filesSelected(selected.length, files.length)}, {humanSize(selectedSize)}</span>
			</label>
			<Button size="xs" class="ml-auto" onclick={() => app.openDownload([row])} disabled={!selected.length}>
				<DownloadIcon />
				{s.download}
			</Button>
		</div>
		<ul class="max-h-64 overflow-y-auto py-1">
			{#each files as f (f.index)}
				<li>
					<label class="flex cursor-pointer items-center gap-2 px-3 py-1 hover:bg-muted/60">
						<Checkbox
							checked={selected.includes(f.index)}
							onCheckedChange={(v) => app.setFileSelected(id, f.index, v === true)}
							aria-label={s.selectFile}
						/>
						<span class="min-w-0 flex-1 truncate" title={f.path}>{f.path}</span>
						<span class="shrink-0 tabular-nums text-muted-foreground">{humanSize(f.size)}</span>
					</label>
				</li>
			{/each}
		</ul>
	{/if}
</div>
