<script lang="ts">
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import { toast } from 'svelte-sonner';
	import * as Sheet from '$lib/components/ui/sheet/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import { api, errorMessage } from '$lib/api/client';
	import type { Job } from '$lib/api/types';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';
	import { humanSize } from '$lib/format';
	import JobCard from './JobCard.svelte';
	import EmptyState from './EmptyState.svelte';
	import ShareDialog from './ShareDialog.svelte';

	let busy = $state<Record<string, boolean>>({});
	let errors = $state<Record<string, string>>({});
	let toDelete = $state<Job | null>(null);
	let deleteFiles = $state(false);
	let toPurge = $state<Job | null>(null);
	let toShare = $state<Job | null>(null);
	const purgeSummary = $derived.by(() => {
		const files = (toPurge?.files ?? []).filter((f) => f.state === 'done' || f.state === 'quarantined');
		const first = files[0]?.path ?? '';
		return {
			count: files.length,
			size: humanSize(files.reduce((a, f) => a + f.size, 0)),
			folder: first.includes('/') ? first.split('/')[0] : ''
		};
	});

	const own = $derived(app.jobs.filter((j) => !j.external));
	const external = $derived(app.jobs.filter((j) => j.external));
	const deleteFilesRelevant = $derived.by(() => {
		if (!toDelete) return false;
		const eng = app.engineById.get(toDelete.engine);
		return !!eng && (eng.caps.localFiles || eng.caps.savePath);
	});

	async function run(job: Job, action: () => Promise<Job | void>, successMessage: string) {
		busy[job.id] = true;
		delete errors[job.id];
		try {
			const res = await action();
			if (res) app.upsertJob(res);
			toast.success(successMessage);
		} catch (e) {
			errors[job.id] = errorMessage(e, s.networkError);
		} finally {
			delete busy[job.id];
		}
	}

	async function confirmDelete() {
		const job = toDelete;
		if (!job) return;
		toDelete = null;
		await run(
			job,
			async () => {
				await api.deleteJob(job.id, deleteFilesRelevant && deleteFiles);
				app.removeJob(job.id);
			},
			s.jobDeleted
		);
		deleteFiles = false;
	}

	async function confirmPurge() {
		const job = toPurge;
		if (!job) return;
		toPurge = null;
		await run(
			job,
			async () => {
				await api.purgeJob(job.id);
				app.removeJob(job.id);
			},
			s.jobPurged
		);
	}
</script>

<Sheet.Root open={app.queueOpen} onOpenChange={(o) => app.setQueueOpen(o)}>
	<Sheet.Content side="right" class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl">
		<Sheet.Header class="border-b border-border px-4 py-3 pr-12">
			<Sheet.Title class="flex items-center gap-2">
				{s.queueTitle}
				{#if app.activeJobs.length}
					<span class="rounded-full bg-info/10 px-2 py-0.5 text-xs font-medium text-info">{s.active(app.activeJobs.length)}</span>
				{/if}
			</Sheet.Title>
			<Sheet.Description>{s.queueDescription}</Sheet.Description>
		</Sheet.Header>
		<div class="flex-1 overflow-y-auto overscroll-contain px-4 py-3">
			{#if app.jobsError && !app.jobsLoaded}
				<EmptyState icon={WifiOffIcon} title={s.queueError} body={app.jobsError} tone="error">
					<Button size="sm" variant="outline" onclick={() => app.refreshJobs()}>{s.retry}</Button>
				</EmptyState>
			{:else if !app.jobsLoaded}
				<div class="flex flex-col gap-2">
					<Skeleton class="h-20 rounded-lg" />
					<Skeleton class="h-20 rounded-lg" />
					<Skeleton class="h-20 rounded-lg" />
				</div>
			{:else if !app.jobs.length}
				<EmptyState icon={InboxIcon} title={s.queueEmpty} body={s.queueEmptyBody} />
			{:else}
				{#if app.jobsError}
					<p class="mb-2 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs text-destructive" role="alert">
						{s.queueError} : {app.jobsError}
					</p>
				{/if}
				<ul class="flex flex-col gap-2">
					{#each own as job (job.id)}
						<JobCard
							{job}
							busy={!!busy[job.id]}
							error={errors[job.id] ?? null}
							oncancel={() => run(job, () => api.cancelJob(job.id), s.jobCancelled)}
							onretry={() => run(job, () => api.retryJob(job.id), s.jobRetried)}
							onrescan={() => run(job, () => api.rescanJob(job.id), s.jobRescanned)}
							onpurge={() => (toPurge = job)}
							onshare={() => (toShare = job)}
							ondelete={() => (toDelete = job)}
						/>
					{/each}
				</ul>
				{#if external.length}
					<p class="mt-4 mb-2 text-xs text-muted-foreground">{s.externalHint}</p>
					<ul class="flex flex-col gap-2">
						{#each external as job (job.id)}
							<JobCard
								{job}
								busy={!!busy[job.id]}
								error={errors[job.id] ?? null}
								oncancel={() => {}}
								onretry={() => {}}
								onrescan={() => {}}
								onpurge={() => {}}
								onshare={() => {}}
								ondelete={() => (toDelete = job)}
							/>
						{/each}
					</ul>
				{/if}
			{/if}
		</div>
	</Sheet.Content>
</Sheet.Root>

<Dialog.Root open={!!toDelete} onOpenChange={(o) => !o && (toDelete = null)}>
	<Dialog.Content class="sm:max-w-sm">
		<Dialog.Header>
			<Dialog.Title>{s.confirmDeleteTitle}</Dialog.Title>
			<Dialog.Description class="break-words">{toDelete ? s.confirmDeleteBody(toDelete.name) : ''}</Dialog.Description>
		</Dialog.Header>
		{#if deleteFilesRelevant}
			<label class="flex items-center gap-2 text-[13px]">
				<Checkbox checked={deleteFiles} onCheckedChange={(v) => (deleteFiles = v === true)} />
				<span>{s.deleteFilesToo}</span>
			</label>
		{/if}
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (toDelete = null)}>{s.cancel}</Button>
			<Button variant="destructive" onclick={confirmDelete}>{s.confirmDelete}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root open={!!toPurge} onOpenChange={(o) => !o && (toPurge = null)}>
	<Dialog.Content class="sm:max-w-sm">
		<Dialog.Header>
			<Dialog.Title>{s.confirmPurgeTitle}</Dialog.Title>
			<Dialog.Description class="break-words">
				{s.confirmPurgeBody(purgeSummary.count, purgeSummary.size, purgeSummary.folder)}
			</Dialog.Description>
		</Dialog.Header>
		<Dialog.Footer>
			<Button variant="outline" onclick={() => (toPurge = null)}>{s.cancel}</Button>
			<Button variant="destructive" onclick={confirmPurge}>{s.purgeJob}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<ShareDialog job={toShare} onclose={() => (toShare = null)} />
