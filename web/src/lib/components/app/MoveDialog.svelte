<script lang="ts">
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import { toast } from 'svelte-sonner';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { api, errorMessage } from '$lib/api/client';
	import type { Job } from '$lib/api/types';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	let { job, onclose }: { job: Job | null; onclose: () => void } = $props();

	let storage = $state('');
	let subdir = $state('');
	let dirs = $state<string[]>([]);
	let busy = $state(false);
	let error = $state<string | null>(null);
	const storages = $derived(app.caps?.storages ?? []);

	// Reset on each opening: same storage, its folders as suggestions.
	$effect(() => {
		if (!job) return;
		storage = job.storage ?? storages[0]?.id ?? '';
		subdir = '';
		error = null;
	});
	$effect(() => {
		const id = storage;
		dirs = [];
		if (id) api.storageDirs(id).then((r) => storage === id && (dirs = r.dirs), () => {});
	});

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!job || busy || !storage) return;
		busy = true;
		error = null;
		try {
			app.upsertJob(await api.moveJob(job.id, { storage, subdir: subdir.trim() || null }));
			toast.success(s.jobMoved);
			onclose();
		} catch (err) {
			error = errorMessage(err, s.networkError);
		} finally {
			busy = false;
		}
	}
</script>

<Dialog.Root open={!!job} onOpenChange={(o) => !o && onclose()}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{s.moveTitle}</Dialog.Title>
			<Dialog.Description class="break-words">{job?.name ?? ''}</Dialog.Description>
		</Dialog.Header>
		<form class="grid gap-4" onsubmit={submit}>
			<div class="grid gap-1.5">
				<span class="text-xs text-muted-foreground" id="move-storage-label">{s.storage}</span>
				<Select.Root type="single" bind:value={storage}>
					<Select.Trigger class="w-full" aria-labelledby="move-storage-label">{app.storageLabel(storage) || s.storage}</Select.Trigger>
					<Select.Content>
						{#each storages as st (st.id)}
							<Select.Item value={st.id} label={st.label}>{st.label}</Select.Item>
						{/each}
					</Select.Content>
				</Select.Root>
			</div>
			<label class="grid gap-1.5">
				<span class="text-xs text-muted-foreground">{s.moveFolder}</span>
				<Input name="move-subdir" bind:value={subdir} list="move-dirs" autocomplete="off" spellcheck={false} />
				<datalist id="move-dirs">
					{#each dirs as d (d)}<option value={d}></option>{/each}
				</datalist>
				<span class="text-xs text-muted-foreground">{s.moveHint}</span>
			</label>
			{#if error}
				<p class="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs break-words text-destructive" role="alert">{error}</p>
			{/if}
			<Dialog.Footer>
				<Button type="submit" disabled={busy || !storage}>
					{#if busy}<LoaderCircleIcon class="animate-spin motion-reduce:animate-none" />{/if}
					{s.moveJob}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
