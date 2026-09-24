<script lang="ts">
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import BanIcon from '@lucide/svelte/icons/ban';
	import RotateCwIcon from '@lucide/svelte/icons/rotate-cw';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import SendIcon from '@lucide/svelte/icons/send';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import LoaderCircleIcon from '@lucide/svelte/icons/loader-circle';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Progress } from '$lib/components/ui/progress/index.js';
	import { cn } from '$lib/utils.js';
	import type { Job, JobState } from '$lib/api/types';
	import { app } from '$lib/state/app.svelte';
	import { humanDuration, humanSize, humanSpeed, percent } from '$lib/format';
	import { s } from '$lib/strings';

	let {
		job,
		busy = false,
		error = null,
		oncancel,
		onretry,
		ondelete
	}: {
		job: Job;
		busy?: boolean;
		error?: string | null;
		oncancel: () => void;
		onretry: () => void;
		ondelete: () => void;
	} = $props();

	let showFiles = $state(false);

	const ACTIVE = new Set<JobState>(['queued', 'adding', 'waitingSelection', 'fetching', 'copying']);
	const active = $derived(ACTIVE.has(job.state));
	const stateClass: Record<JobState, string> = {
		queued: 'border-border text-muted-foreground',
		adding: 'border-info/40 bg-info/10 text-info',
		waitingSelection: 'border-warning/50 bg-warning/15 text-warning-foreground dark:text-warning',
		fetching: 'border-info/40 bg-info/10 text-info',
		ready: 'border-cached/40 bg-cached/10 text-cached',
		copying: 'border-info/40 bg-info/10 text-info',
		done: 'border-seed-high/40 bg-seed-high/10 text-seed-high',
		failed: 'border-destructive/40 bg-destructive/10 text-destructive',
		cancelled: 'border-border bg-muted text-muted-foreground'
	};
	const barClass: Record<JobState, string> = {
		queued: '[&>[data-slot=progress-indicator]]:bg-muted-foreground',
		adding: '[&>[data-slot=progress-indicator]]:bg-info',
		waitingSelection: '[&>[data-slot=progress-indicator]]:bg-warning',
		fetching: '[&>[data-slot=progress-indicator]]:bg-info',
		ready: '[&>[data-slot=progress-indicator]]:bg-cached',
		copying: '[&>[data-slot=progress-indicator]]:bg-info',
		done: '[&>[data-slot=progress-indicator]]:bg-seed-high',
		failed: '[&>[data-slot=progress-indicator]]:bg-destructive',
		cancelled: '[&>[data-slot=progress-indicator]]:bg-muted-foreground'
	};
	const fileStateClass: Record<string, string> = {
		pending: 'text-muted-foreground',
		copying: 'text-info',
		done: 'text-seed-high',
		failed: 'text-destructive',
		skipped: 'text-muted-foreground/60'
	};

	const showProgress = $derived(active || job.state === 'failed' || (job.state === 'done' && !job.external));
	const details = $derived.by(() => {
		const parts: string[] = [];
		if (active) parts.push(percent(job.progress));
		if (active && job.speed) parts.push(humanSpeed(job.speed));
		// Engines report absurd ETAs (100 days) while a transfer has not started.
		if (active && job.eta != null && job.eta > 0 && job.eta < 30 * 86400) parts.push(s.eta(humanDuration(job.eta)));
		return parts.join(' · ');
	});
	const totalSize = $derived(job.files.reduce((a, f) => a + f.size, 0));
</script>

<li class={cn('rounded-lg border border-border bg-card p-3 text-[13px]', busy && 'opacity-70')} aria-busy={busy}>
	<div class="flex items-start gap-2">
		<div class="min-w-0 flex-1">
			<p class="line-clamp-2 leading-snug font-medium break-words" title={job.name} translate="no">{job.name}</p>
			<div class="mt-1.5 flex flex-wrap items-center gap-1 text-[11px]">
				<span class={cn('inline-flex h-5 items-center gap-1 rounded-md border px-1.5 font-medium', stateClass[job.state])}>
					{#if active}<LoaderCircleIcon class="size-3 animate-spin motion-reduce:animate-none" aria-hidden="true" />{/if}
					{s.jobState[job.state] ?? job.state}
				</span>
				{#if job.external}
					<Tooltip.Root>
						<Tooltip.Trigger class="inline-flex h-5 items-center rounded-md border border-dashed border-border px-1.5 text-muted-foreground">{s.external}</Tooltip.Trigger>
						<Tooltip.Content>{s.externalHint}</Tooltip.Content>
					</Tooltip.Root>
				{/if}
				<span class="inline-flex h-5 items-center rounded-md border border-border px-1.5 text-muted-foreground">{app.engineName(job.engine)}</span>
				{#if job.storage}
					<span class="inline-flex h-5 items-center rounded-md border border-border px-1.5 text-muted-foreground">
						{app.storageLabel(job.storage)}{#if job.subdir}<span class="opacity-70">/{job.subdir}</span>{/if}
					</span>
				{:else}
					<span class="inline-flex h-5 items-center rounded-md border border-border px-1.5 text-muted-foreground">{s.modeLabel[job.mode] ?? job.mode}</span>
				{/if}
				<span class="ml-auto tabular-nums text-muted-foreground">{humanSize(totalSize)}</span>
			</div>
		</div>
	</div>

	{#if showProgress}
		<div class="mt-2 flex items-center gap-2">
			<Progress value={Math.round(job.progress * 100)} class={cn('h-1.5', barClass[job.state])} aria-label={s.jobState[job.state]} />
			{#if details}<span class="shrink-0 text-xs tabular-nums text-muted-foreground">{details}</span>{/if}
		</div>
	{/if}

	{#if job.error || error}
		<p class="mt-2 rounded-md border border-destructive/30 bg-destructive/5 px-2 py-1 text-xs break-words text-destructive" role="alert">
			{error ?? job.error}
		</p>
	{/if}

	<div class="mt-2 flex flex-wrap items-center gap-1">
		{#if job.files.length}
			<button
				type="button"
				class="inline-flex h-6 items-center gap-1 rounded px-1 text-xs text-muted-foreground hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
				aria-expanded={showFiles}
				onclick={() => (showFiles = !showFiles)}
			>
				<ChevronRightIcon class={cn('size-3.5 transition-transform motion-reduce:transition-none', showFiles && 'rotate-90')} />
				{s.filesCount(job.files.length)}
			</button>
		{/if}
		<div class="ml-auto flex items-center gap-1">
			{#if job.external}
				<Button size="xs" variant="outline" onclick={() => app.openSend(job)} disabled={busy}>
					<SendIcon />
					{s.sendToStorage}
				</Button>
			{/if}
			{#if active && !job.external}
				<Button size="xs" variant="outline" onclick={oncancel} disabled={busy}>
					<BanIcon />
					{s.cancelJob}
				</Button>
			{/if}
			{#if job.retryable && !job.external}
				<Button size="xs" variant="outline" onclick={onretry} disabled={busy}>
					<RotateCwIcon />
					{s.retryJob}
				</Button>
			{/if}
			<Button size="xs" variant="ghost" class="text-muted-foreground hover:text-destructive" onclick={ondelete} disabled={busy} aria-label={s.deleteJob} title={s.deleteJob}>
				<Trash2Icon />
			</Button>
		</div>
	</div>

	{#if showFiles && job.files.length}
		<ul class="mt-2 max-h-56 overflow-y-auto rounded-md border border-border/70 bg-muted/30 py-1 text-xs">
			{#each job.files as f (f.path)}
				<li class="flex items-center gap-2 px-2 py-1">
					<span class={cn('w-14 shrink-0', fileStateClass[f.state])}>{s.fileState[f.state] ?? f.state}</span>
					<span class="min-w-0 flex-1 truncate" title={f.path}>{f.path}</span>
					<span class="shrink-0 tabular-nums text-muted-foreground">
						{#if f.state === 'copying'}{humanSize(f.done)} / {/if}{humanSize(f.size)}
					</span>
					{#if f.url}
						<a
							href={f.url}
							target="_blank"
							rel="noopener"
							class="inline-flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
							aria-label={`${s.openLink} ${f.path}`}
							title={s.openLink}
						>
							<ExternalLinkIcon class="size-3.5" />
						</a>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</li>
