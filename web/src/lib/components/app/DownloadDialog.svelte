<script lang="ts">
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import DatabaseIcon from '@lucide/svelte/icons/database';
	import { toast } from 'svelte-sonner';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { cn } from '$lib/utils.js';
	import { api, errorMessage } from '$lib/api/client';
	import type { DeliveryMode, TorrentFile } from '$lib/api/types';
	import { app, type DownloadTarget } from '$lib/state/app.svelte';
	import { humanSize, sanitizeSubdir } from '$lib/format';
	import { s } from '$lib/strings';

	const targets = $derived(app.download ?? []);
	const open = $derived(targets.length > 0);
	const sendMode = $derived(targets.length === 1 && targets[0].kind === 'send');
	const single = $derived(targets.length === 1 ? targets[0] : null);

	let engine = $state('');
	let mode = $state<DeliveryMode>('copy');
	let storage = $state('');
	let subdir = $state('');
	let files = $state<number[] | null>(null);
	let busy = $state(false);
	let lastKey = $state<string | null>(null);

	const caps = $derived(app.caps);
	const engines = $derived(caps?.engines ?? []);
	const storages = $derived(caps?.storages ?? []);
	const currentEngine = $derived(app.engineById.get(engine) ?? null);
	const adoptAllowed = $derived(!!currentEngine && (currentEngine.caps.localFiles || currentEngine.caps.savePath));
	const cleanSubdir = $derived(sanitizeSubdir(subdir));

	// Files known for the single target (preview loaded, payload with a file list, or an external item).
	const knownFiles = $derived.by<TorrentFile[] | null>(() => {
		if (!single) return null;
		if (single.kind === 'result') return single.files;
		if (single.kind === 'payload') return single.payload.files;
		return single.job.files.map((f, i) => ({ path: f.path, size: f.size, index: i }));
	});

	const names = $derived(
		targets.map((t) => (t.kind === 'result' ? t.row.title : t.kind === 'payload' ? t.payload.name : t.job.name))
	);
	const hashes = $derived(
		targets
			.map((t) => (t.kind === 'result' ? t.row.infoHash : t.kind === 'payload' ? t.payload.infoHash : t.job.infoHash))
			.filter((h): h is string => !!h)
	);

	function cacheHint(engineId: string): string | null {
		const eng = app.engineById.get(engineId);
		if (!eng?.caps.cached || !hashes.length) return null;
		if (hashes.length === 1) {
			const v = app.cachedOn(engineId, hashes[0]);
			return v === null ? null : v ? s.cached : s.notCached;
		}
		const n = hashes.filter((h) => app.cachedOn(engineId, h) === true).length;
		return s.cachedCount(n, hashes.length);
	}

	const selectedSize = $derived(
		knownFiles && files ? knownFiles.filter((f) => files!.includes(f.index)).reduce((a, f) => a + f.size, 0) : null
	);
	const canLaunch = $derived(
		!busy && !!engine && (mode === 'links' || !!storage) && (files === null || files.length > 0) && targets.length > 0
	);

	// Initialise form fields once per dialog opening (keyed on the target set).
	$effect(() => {
		if (!open || !caps) return;
		const key = targets.map((t) => (t.kind === 'result' ? t.row.key : t.kind === 'payload' ? t.payload.id : t.job.id)).join('|');
		if (key === lastKey) return;
		lastKey = key;
		const first = targets[0];
		engine = first.kind === 'send' ? first.job.engine : caps.defaults.engine || engines[0]?.id || '';
		mode = first.kind === 'send' ? 'copy' : caps.defaults.mode;
		storage = caps.defaults.storage || storages[0]?.id || '';
		subdir = '';
		busy = false;
		if (first.kind === 'result' && first.preselected) files = first.preselected;
		else files = null;
	});

	function setMode(next: DeliveryMode) {
		mode = next;
	}

	function toggleFile(index: number, on: boolean) {
		const base = files ?? knownFiles?.map((f) => f.index) ?? [];
		const set = new Set(base);
		if (on) set.add(index);
		else set.delete(index);
		const next = [...set].sort((a, b) => a - b);
		files = knownFiles && next.length === knownFiles.length ? null : next;
	}

	async function launch(e: SubmitEvent) {
		e.preventDefault();
		if (!canLaunch) return;
		busy = true;
		let ok = 0;
		for (const t of targets) {
			const name = t.kind === 'result' ? t.row.title : t.kind === 'payload' ? t.payload.name : t.job.name;
			try {
				const body = {
					storage: mode === 'links' ? null : storage,
					subdir: cleanSubdir || null,
					files: targets.length === 1 ? files : null,
					mode
				};
				const job =
					t.kind === 'send'
						? await api.sendJob(t.job.id, { ...body, storage: storage })
						: await api.createJob({
								resultId: t.kind === 'result' ? t.row.primary.id : null,
								payloadId: t.kind === 'payload' ? t.payload.id : null,
								engine,
								...body
							});
				app.upsertJob(job);
				ok++;
				toast.success(s.jobCreated(job.name));
			} catch (err) {
				toast.error(`${s.jobFailed(name)} : ${errorMessage(err, s.networkError)}`);
			}
		}
		busy = false;
		if (ok) {
			app.closeDownload();
			app.selection.clear();
			app.setQueueOpen(true);
		}
	}

	const modes = $derived<{ value: DeliveryMode; label: string }[]>([
		{ value: 'copy', label: s.modeCopy },
		{ value: 'adopt', label: s.modeAdopt },
		{ value: 'links', label: s.modeLinks }
	]);
</script>

<Dialog.Root {open} onOpenChange={(o) => !o && app.closeDownload()}>
	<Dialog.Content class="max-h-[calc(100dvh-2rem)] overflow-y-auto overscroll-contain sm:max-w-lg">
		<Dialog.Header>
			<Dialog.Title>{sendMode ? s.sendTitle : s.downloadTitle}</Dialog.Title>
			<Dialog.Description>{s.downloadItems(targets.length)}</Dialog.Description>
		</Dialog.Header>

		<ul class="max-h-28 overflow-y-auto rounded-lg border border-border bg-muted/40 px-3 py-2 text-[13px]">
			{#each names as name, i (i)}
				<li class="truncate" title={name}>{name}</li>
			{/each}
		</ul>

		<form class="flex flex-col gap-4" onsubmit={launch}>
			{#if !engines.length}
				<p class="text-sm text-destructive">{s.noEngine}</p>
			{:else}
				<div class="grid gap-1.5">
					<span class="text-xs text-muted-foreground" id="engine-label">{s.engine}</span>
					<Select.Root type="single" bind:value={engine} disabled={sendMode}>
						<Select.Trigger class="w-full" aria-labelledby="engine-label">
							<span class="flex items-center gap-2">
								{currentEngine?.name ?? engine}
								{#if currentEngine && cacheHint(currentEngine.id)}
									<span class={cn('inline-flex items-center gap-1 text-xs', cacheHint(currentEngine.id) === s.notCached ? 'text-muted-foreground' : 'text-cached')}>
										<DatabaseIcon class="size-3" />{cacheHint(currentEngine.id)}
									</span>
								{/if}
							</span>
						</Select.Trigger>
						<Select.Content>
							{#each engines as e (e.id)}
								{@const hint = cacheHint(e.id)}
								<Select.Item value={e.id} label={e.name}>
									<span>{e.name}</span>
									{#if hint}
										<span class={cn('ml-auto text-xs', hint === s.notCached ? 'text-muted-foreground' : 'text-cached')}>{hint}</span>
									{/if}
								</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
			{/if}

			<fieldset class="grid gap-1.5">
				<legend class="mb-1.5 text-xs text-muted-foreground">{s.mode}</legend>
				<div class="grid gap-1">
					{#each modes as m (m.value)}
						{@const disabled = (m.value === 'adopt' && !adoptAllowed) || (m.value === 'links' && sendMode)}
						<label
							class={cn(
								'flex cursor-pointer items-start gap-2.5 rounded-lg border px-3 py-2 text-[13px] transition-colors',
								mode === m.value ? 'border-foreground/50 bg-muted/60' : 'border-border hover:bg-muted/40',
								disabled && 'cursor-not-allowed opacity-50'
							)}
						>
							<input
								type="radio"
								name="mode"
								value={m.value}
								checked={mode === m.value}
								aria-label={m.label}
								{disabled}
								class="mt-0.5 accent-foreground"
								onchange={() => setMode(m.value)}
							/>
							<span class="flex flex-col gap-0.5">
								<span class="font-medium">{m.label}</span>
								<span class="text-xs text-muted-foreground">{s.modeDescription[m.value]}</span>
							</span>
						</label>
					{/each}
				</div>
			</fieldset>

			{#if mode !== 'links'}
				<div class="grid gap-1.5">
					<span class="text-xs text-muted-foreground" id="storage-label">{s.storage}</span>
					{#if !storages.length}
						<p class="text-sm text-destructive">{s.noStorage}</p>
					{:else}
						<Select.Root type="single" bind:value={storage}>
							<Select.Trigger class="w-full" aria-labelledby="storage-label">
								{app.storageLabel(storage) || s.storage}
							</Select.Trigger>
							<Select.Content>
								{#each storages as st (st.id)}
									<Select.Item value={st.id} label={st.label}>
										<span>{st.label}</span>
										<span class="ml-auto text-xs text-muted-foreground">{s.free(humanSize(st.free))}</span>
									</Select.Item>
								{/each}
							</Select.Content>
						</Select.Root>
					{/if}
				</div>
				<label class="grid gap-1.5">
					<span class="text-xs text-muted-foreground">{s.subdir}</span>
					<Input name="subdir" bind:value={subdir} placeholder={s.subdirPlaceholder} autocomplete="off" spellcheck={false} />
					<span class="text-xs text-muted-foreground">
						{#if subdir && cleanSubdir !== subdir.trim()}
							{s.subdirHint} <span class="font-medium text-foreground">{cleanSubdir || '/'}</span>
						{:else}
							{s.subdirHint}
						{/if}
					</span>
				</label>
			{/if}

			{#if knownFiles && knownFiles.length}
				<div class="grid gap-1.5">
					<div class="flex items-center justify-between text-xs text-muted-foreground">
						<span>{s.files}</span>
						<span class="tabular-nums">
							{s.filesSelected(files ? files.length : knownFiles.length, knownFiles.length)}{#if selectedSize != null}, {humanSize(selectedSize)}{/if}
						</span>
					</div>
					<ul class="max-h-44 overflow-y-auto rounded-lg border border-border py-1 text-[13px]">
						{#each knownFiles as f (f.index)}
							{@const on = files === null || files.includes(f.index)}
							<li>
								<label class="flex cursor-pointer items-center gap-2 px-2.5 py-1 hover:bg-muted/60">
									<Checkbox checked={on} onCheckedChange={(v) => toggleFile(f.index, v === true)} aria-label={s.selectFile} />
									<span class="min-w-0 flex-1 truncate" title={f.path}>{f.path}</span>
									<span class="shrink-0 text-xs tabular-nums text-muted-foreground">{humanSize(f.size)}</span>
								</label>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<Dialog.Footer>
				<Button type="submit" disabled={!canLaunch}>
					{#if busy}<LoaderCircleIcon class="animate-spin motion-reduce:animate-none" />{/if}
					{busy ? s.launching : s.launch}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
