<script lang="ts">
	import './layout.css';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { s } from '$lib/strings';

	let { children } = $props();
</script>

<svelte:head>
	<title>{s.pageTitle}</title>
	<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
</svelte:head>

<ModeWatcher themeColors={{ light: '#ffffff', dark: '#18181b' }} />
<Toaster richColors position="bottom-right" closeButton closeButtonAriaLabel={s.closeToast} />
<Tooltip.Provider delayDuration={250}>
	{@render children()}
</Tooltip.Provider>
