<script lang="ts">
	import StarIcon from '@lucide/svelte/icons/star';
	import XIcon from '@lucide/svelte/icons/x';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import { app } from '$lib/state/app.svelte';
	import { s } from '$lib/strings';

	const recent = $derived(app.history.filter((h) => !app.isSaved(h)));
</script>

{#if app.saved.length || recent.length}
	<div class="flex flex-col gap-2 text-[13px]">
		{#if app.saved.length}
			<div class="flex flex-wrap items-center gap-1.5">
				<span class="mr-1 inline-flex items-center gap-1 text-xs text-muted-foreground">
					<StarIcon class="size-3.5 fill-current" aria-hidden="true" />{s.savedSearches}
				</span>
				{#each app.saved as q (q)}
					<span class="group inline-flex h-7 items-center overflow-hidden rounded-full border border-border bg-card">
						<button type="button" class="h-full pr-1.5 pl-2.5 hover:bg-muted" onclick={() => app.search(q)}>{q}</button>
						<button
							type="button"
							class="flex h-full items-center pr-2 pl-1 text-muted-foreground hover:text-foreground"
							aria-label={`${s.unsaveSearch} : ${q}`}
							onclick={() => app.toggleSaved(q)}
						>
							<XIcon class="size-3" />
						</button>
					</span>
				{/each}
			</div>
		{/if}
		{#if recent.length}
			<div class="flex flex-wrap items-center gap-1.5">
				<span class="mr-1 inline-flex items-center gap-1 text-xs text-muted-foreground">
					<HistoryIcon class="size-3.5" aria-hidden="true" />{s.history}
				</span>
				{#each recent as q (q)}
					<span class="inline-flex h-7 items-center overflow-hidden rounded-full border border-dashed border-border">
						<button type="button" class="h-full pr-1.5 pl-2.5 hover:bg-muted" onclick={() => app.search(q)}>{q}</button>
						<button
							type="button"
							class="flex h-full items-center pr-2 pl-1 text-muted-foreground hover:text-foreground"
							aria-label={`${s.removeFromHistory} : ${q}`}
							onclick={() => app.removeHistory(q)}
						>
							<XIcon class="size-3" />
						</button>
					</span>
				{/each}
				<button type="button" class="ml-1 text-xs text-muted-foreground underline-offset-2 hover:underline" onclick={() => app.clearHistory()}>
					{s.clearHistory}
				</button>
			</div>
		{/if}
	</div>
{/if}
