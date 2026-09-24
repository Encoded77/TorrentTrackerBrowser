<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { Component } from 'svelte';

	let {
		icon: Icon,
		title,
		body,
		hint,
		tone = 'neutral',
		children
	}: {
		icon: Component<{ class?: string }>;
		title: string;
		body?: string;
		hint?: string;
		tone?: 'neutral' | 'error';
		children?: Snippet;
	} = $props();
</script>

<div class="flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border px-6 py-14 text-center">
	<div class={tone === 'error' ? 'text-destructive' : 'text-muted-foreground'}>
		<Icon class="size-7" />
	</div>
	<h2 class="text-sm font-medium text-balance">{title}</h2>
	{#if body}<p class="max-w-md text-[13px] text-pretty text-muted-foreground">{body}</p>{/if}
	{#if hint}<p class="text-xs text-muted-foreground/80">{hint}</p>{/if}
	{#if children}<div class="mt-2">{@render children()}</div>{/if}
</div>
