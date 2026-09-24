<script lang="ts">
	import DownloadIcon from '@lucide/svelte/icons/download';
	import EllipsisVerticalIcon from '@lucide/svelte/icons/ellipsis-vertical';
	import MagnetIcon from '@lucide/svelte/icons/magnet';
	import FileDownIcon from '@lucide/svelte/icons/file-down';
	import ListTreeIcon from '@lucide/svelte/icons/list-tree';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import { cn } from '$lib/utils.js';
	import { app, type ResultRow } from '$lib/state/app.svelte';
	import { copyMagnet, downloadTorrent, magnetSource, openInfoPage, torrentSource } from '$lib/state/actions';
	import { s } from '$lib/strings';

	let { row, class: className }: { row: ResultRow; class?: string } = $props();

	const expanded = $derived(app.expandedKey === row.key);
	const hasInfo = $derived(row.sources.some((r) => r.infoUrl));
</script>

<div class={cn('flex items-center justify-end gap-0.5', className)}>
	<Button size="icon-sm" variant="ghost" onclick={() => app.openDownload([row])} aria-label={s.download} title={s.download}>
		<DownloadIcon />
	</Button>
	<DropdownMenu.Root>
		<DropdownMenu.Trigger class={buttonVariants({ variant: 'ghost', size: 'icon-sm' })} aria-label={s.moreActions} title={s.moreActions}>
			<EllipsisVerticalIcon />
		</DropdownMenu.Trigger>
		<DropdownMenu.Content align="end" class="w-56">
			<DropdownMenu.Item onSelect={() => app.openDownload([row])}>
				<DownloadIcon />
				{s.download}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={() => copyMagnet(row)} disabled={!magnetSource(row)}>
				<MagnetIcon />
				{s.copyMagnet}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={() => downloadTorrent(row)} disabled={!torrentSource(row)}>
				<FileDownIcon />
				{s.downloadTorrent}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={() => app.toggleExpanded(row)}>
				<ListTreeIcon />
				{expanded ? s.hidePreview : s.previewFiles}
			</DropdownMenu.Item>
			{#if hasInfo}
				<DropdownMenu.Separator />
				<DropdownMenu.Item onSelect={() => openInfoPage(row)}>
					<ExternalLinkIcon />
					{s.openInfoPage}
				</DropdownMenu.Item>
			{/if}
		</DropdownMenu.Content>
	</DropdownMenu.Root>
</div>
