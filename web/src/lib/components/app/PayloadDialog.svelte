<script lang="ts">
	import UploadIcon from '@lucide/svelte/icons/upload';
	import FileIcon from '@lucide/svelte/icons/file';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import { toast } from 'svelte-sonner';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import { cn } from '$lib/utils.js';
	import { api, errorMessage } from '$lib/api/client';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	let magnet = $state('');
	let file = $state<File | null>(null);
	let dragging = $state(false);
	let busy = $state(false);
	let error = $state<string | null>(null);
	let fileInput = $state<HTMLInputElement | null>(null);

	const magnetOk = $derived(magnet.trim().startsWith('magnet:?'));
	const canSubmit = $derived(!busy && (magnetOk || !!file));

	function reset() {
		magnet = '';
		file = null;
		error = null;
		busy = false;
		dragging = false;
	}

	function takeFile(f: File | null | undefined) {
		if (!f) return;
		if (!f.name.toLowerCase().endsWith('.torrent') && f.type !== 'application/x-bittorrent') {
			error = s.invalidTorrentFile;
			return;
		}
		error = null;
		file = f;
	}

	function ondrop(e: DragEvent) {
		e.preventDefault();
		dragging = false;
		takeFile(e.dataTransfer?.files?.[0]);
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canSubmit) return;
		if (magnet.trim() && !magnetOk) {
			error = s.invalidMagnet;
			return;
		}
		busy = true;
		error = null;
		try {
			const payload = file ? await api.postTorrent(file) : await api.postMagnet(magnet.trim());
			app.payloadDialogOpen = false;
			reset();
			toast.success(`${s.payloadAdded} : ${payload.name}`);
			app.openDownloadForPayload(payload);
		} catch (err) {
			error = errorMessage(err, s.networkError);
			busy = false;
		}
	}
</script>

<Dialog.Root
	open={app.payloadDialogOpen}
	onOpenChange={(o) => {
		app.payloadDialogOpen = o;
		if (!o) reset();
	}}
>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>{s.payloadTitle}</Dialog.Title>
			<Dialog.Description>{s.payloadDescription}</Dialog.Description>
		</Dialog.Header>
		<form class="flex flex-col gap-3" onsubmit={submit}>
			<label class="flex flex-col gap-1 text-xs">
				<span class="text-muted-foreground">{s.magnetLabel}</span>
				<Textarea
					name="magnet"
					autocomplete="off"
					bind:value={magnet}
					placeholder={s.magnetPlaceholder}
					rows={3}
					spellcheck={false}
					class="min-h-16 font-mono text-xs break-all"
					aria-invalid={magnet.trim() !== '' && !magnetOk}
					disabled={!!file}
					onkeydown={(e) => {
						// A magnet is one line: Enter submits, Shift+Enter inserts a newline.
						if (e.key === 'Enter' && !e.shiftKey) {
							e.preventDefault();
							e.currentTarget.form?.requestSubmit();
						}
					}}
				/>
			</label>
			<div
				role="group"
				aria-label={s.dropZone}
				class={cn(
					'rounded-lg border border-dashed transition-colors',
					dragging ? 'border-foreground bg-muted' : 'border-border',
					file && 'border-foreground/40 bg-muted/40'
				)}
				ondragenter={(e) => {
					e.preventDefault();
					dragging = true;
				}}
				ondragover={(e) => {
					e.preventDefault();
					dragging = true;
				}}
				ondragleave={() => (dragging = false)}
				{ondrop}
			>
				{#if file}
					<div class="flex items-center gap-2 px-4 py-3 text-xs">
						<FileIcon class="size-4 shrink-0" aria-hidden="true" />
						<span class="min-w-0 flex-1 truncate" translate="no">{s.fileChosen(file.name)}</span>
						<Button type="button" size="xs" variant="ghost" onclick={() => (file = null)}>{s.cancel}</Button>
					</div>
				{:else}
					<button
						type="button"
						class="flex w-full cursor-pointer flex-col items-center justify-center gap-1.5 rounded-lg px-4 py-6 text-center text-xs text-muted-foreground hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
						onclick={() => fileInput?.click()}
					>
						<UploadIcon class="size-5" aria-hidden="true" />
						<span>{dragging ? s.dropZoneActive : s.dropZone}</span>
					</button>
				{/if}
				<input
					bind:this={fileInput}
					type="file"
					name="torrent"
					accept=".torrent,application/x-bittorrent"
					class="sr-only"
					tabindex="-1"
					aria-hidden="true"
					onchange={(e) => takeFile(e.currentTarget.files?.[0])}
				/>
			</div>
			{#if error}
				<p class="text-xs text-destructive" role="alert">{error}</p>
			{/if}
			<Dialog.Footer>
				<Button type="submit" disabled={!canSubmit}>
					{#if busy}<LoaderCircleIcon class="animate-spin motion-reduce:animate-none" />{/if}
					{s.continueToDownload}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
