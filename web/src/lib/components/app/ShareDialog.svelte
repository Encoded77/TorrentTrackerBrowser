<script lang="ts">
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import { toast } from 'svelte-sonner';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { api, errorMessage } from '$lib/api/client';
	import type { Job } from '$lib/api/types';
	import { s } from '$lib/strings';

	let { job, onclose }: { job: Job | null; onclose: () => void } = $props();

	const DAYS = ['1', '7', '30', '0'];
	let days = $state('7');
	let password = $state('');
	let busy = $state(false);
	let error = $state<string | null>(null);
	let url = $state<string | null>(null);

	function close() {
		days = '7';
		password = '';
		error = null;
		url = null;
		onclose();
	}

	async function create(e: SubmitEvent) {
		e.preventDefault();
		if (!job || busy) return;
		busy = true;
		error = null;
		try {
			url = (await api.shareJob(job.id, { days: Number(days), password: password || undefined })).url;
		} catch (err) {
			error = errorMessage(err, s.networkError);
		} finally {
			busy = false;
		}
	}

	async function copy() {
		if (!url) return;
		try {
			await navigator.clipboard.writeText(url);
			toast.success(s.linkCopied);
		} catch {
			toast.error(s.copyFailed);
		}
	}
</script>

<Dialog.Root open={!!job} onOpenChange={(o) => !o && close()}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{s.shareTitle}</Dialog.Title>
			<Dialog.Description class="break-words">{job?.name ?? ''}</Dialog.Description>
		</Dialog.Header>
		{#if url}
			<div class="flex items-center gap-2">
				<Input value={url} readonly onfocus={(e) => e.currentTarget.select()} aria-label={s.shareTitle} />
				<Button variant="outline" onclick={copy}><CopyIcon />{s.copyLink}</Button>
			</div>
			<p class="text-xs text-muted-foreground">{s.shareHint}</p>
			<Dialog.Footer>
				<Button onclick={close}>{s.close}</Button>
			</Dialog.Footer>
		{:else}
			<form class="grid gap-4" onsubmit={create}>
				<div class="grid gap-1.5">
					<span class="text-xs text-muted-foreground" id="share-days-label">{s.shareExpiry}</span>
					<Select.Root type="single" bind:value={days}>
						<Select.Trigger class="w-full" aria-labelledby="share-days-label">{s.shareDays(Number(days))}</Select.Trigger>
						<Select.Content>
							{#each DAYS as d (d)}
								<Select.Item value={d} label={s.shareDays(Number(d))}>{s.shareDays(Number(d))}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				<label class="grid gap-1.5">
					<span class="text-xs text-muted-foreground">{s.sharePassword}</span>
					<Input type="password" name="share-password" bind:value={password} autocomplete="new-password" />
				</label>
				{#if error}
					<p class="rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs break-words text-destructive" role="alert">{error}</p>
				{/if}
				<Dialog.Footer>
					<Button type="submit" disabled={busy}>
						{#if busy}<LoaderCircleIcon class="animate-spin motion-reduce:animate-none" />{/if}
						{s.shareCreate}
					</Button>
				</Dialog.Footer>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>
