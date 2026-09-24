<script lang="ts">
	import { toggleMode } from 'mode-watcher';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import ListIcon from '@lucide/svelte/icons/list';
	import LanguagesIcon from '@lucide/svelte/icons/languages';
	import { Button, buttonVariants } from '$lib/components/ui/button/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { app } from '$lib/state/app.svelte';
	import { locale, LOCALES, type Locale } from '$lib/i18n/locale.svelte';
	import { s } from '$lib/strings';

	const activeCount = $derived(app.activeJobs.length);
</script>

<header
	class="sticky top-0 z-30 border-b border-border/70 bg-background/85 backdrop-blur supports-[backdrop-filter]:bg-background/70"
>
	<div class="mx-auto flex h-12 w-full max-w-[1400px] items-center gap-3 px-4 sm:px-6">
		<a href="/" class="flex items-baseline gap-2 rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/50">
			<h1 class="text-[15px] font-semibold tracking-tight" translate="no">{s.appName}</h1>
			<span class="hidden text-xs text-muted-foreground sm:inline">{s.appTagline}</span>
		</a>
		<div class="ml-auto flex items-center gap-1.5">
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					class={buttonVariants({ variant: 'ghost', size: 'icon-sm' })}
					aria-label={`${s.chooseLanguage}: ${s.localeName[locale.current]}`}
					title={s.chooseLanguage}
				>
					<LanguagesIcon />
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-40">
					<DropdownMenu.RadioGroup value={locale.current} onValueChange={(v) => locale.set(v as Locale)}>
						{#each LOCALES as code (code)}
							<DropdownMenu.RadioItem value={code} lang={code}>{s.localeName[code]}</DropdownMenu.RadioItem>
						{/each}
					</DropdownMenu.RadioGroup>
				</DropdownMenu.Content>
			</DropdownMenu.Root>
			<Button variant="ghost" size="icon-sm" onclick={toggleMode} aria-label={s.toggleTheme} title={s.toggleTheme}>
				<SunIcon class="dark:hidden" />
				<MoonIcon class="hidden dark:block" />
			</Button>
			<Button
				variant={activeCount ? 'default' : 'outline'}
				size="sm"
				onclick={() => app.setQueueOpen(true)}
				aria-label={s.openQueue}
				class="relative"
			>
				<ListIcon />
				<span>{s.queue}</span>
				{#if activeCount}
					<span
						class="ml-0.5 inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-background/90 px-1 text-[10px] font-semibold text-foreground"
						aria-label={s.active(activeCount)}
					>
						{activeCount}
					</span>
				{/if}
			</Button>
		</div>
	</div>
</header>
