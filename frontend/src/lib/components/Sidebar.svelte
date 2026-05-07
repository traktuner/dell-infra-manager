<script lang="ts">
	import { page } from '$app/state';
	import {
		LayoutDashboard,
		Server,
		HardDrive,
		ListChecks,
		Cpu,
		ChevronRight
	} from '@lucide/svelte';

	const nav = [
		{ href: '/', label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/servers', label: 'Servers', icon: Server },
		{ href: '/firmware', label: 'Firmware', icon: HardDrive },
		{ href: '/jobs', label: 'Jobs', icon: ListChecks }
	];

	const active = $derived(page.url.pathname);
</script>

<aside class="w-56 shrink-0 flex flex-col bg-zinc-900 border-r border-zinc-800 min-h-screen">
	<!-- Logo -->
	<div class="flex items-center gap-2.5 px-5 py-4 border-b border-zinc-800">
		<div class="w-7 h-7 bg-blue-600 rounded-lg flex items-center justify-center">
			<Cpu class="w-4 h-4 text-white" />
		</div>
		<span class="font-semibold text-zinc-100 text-sm">iDRAC Manager</span>
	</div>

	<!-- Navigation -->
	<nav class="flex-1 px-3 py-4 space-y-0.5">
		{#each nav as { href, label, icon: Icon }}
			<a
				{href}
				class="flex items-center justify-between px-3 py-2 rounded-lg text-sm transition-colors
					{active === href || (href !== '/' && active.startsWith(href))
					? 'bg-zinc-800 text-zinc-100 font-medium'
					: 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50'}"
			>
				<span class="flex items-center gap-2.5">
					<Icon class="w-4 h-4" />
					{label}
				</span>
				{#if active === href || (href !== '/' && active.startsWith(href))}
					<ChevronRight class="w-3 h-3 text-zinc-500" />
				{/if}
			</a>
		{/each}
	</nav>

	<div class="px-5 py-3 border-t border-zinc-800 text-xs text-zinc-600">
		Dell iDRAC Manager
	</div>
</aside>
