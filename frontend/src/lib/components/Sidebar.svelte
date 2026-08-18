<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import {
		LayoutDashboard,
		Server,
		HardDrive,
		ListChecks,
		Cpu,
		Settings,
		ChevronRight,
		User as UserIcon
	} from '@lucide/svelte';

	const nav = [
		{ href: '/',         label: 'Dashboard', icon: LayoutDashboard },
		{ href: '/servers',  label: 'Servers',   icon: Server },
		{ href: '/firmware', label: 'Firmware',  icon: HardDrive },
		{ href: '/jobs',     label: 'Jobs',      icon: ListChecks },
		{ href: '/settings', label: 'Settings',  icon: Settings }
	];

	const active = $derived(page.url.pathname);

	let userEmail = $state<string | null>(null);
	let authEnabled = $state(false);

	onMount(async () => {
		try {
			const me = await api.auth.me();
			authEnabled = me.auth_enabled;
			userEmail = me.user.email;
		} catch {
			// /me failure shouldn't block UI
		}
	});
</script>

<aside class="sticky top-0 hidden h-screen w-56 shrink-0 flex-col border-r border-zinc-800 bg-zinc-900 md:flex">
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

	<!-- User badge -->
	<div class="px-3 py-3 border-t border-zinc-800">
		{#if authEnabled && userEmail && userEmail !== 'anonymous'}
			<div class="flex items-center gap-2 text-xs px-2 py-1.5 rounded-lg bg-zinc-800/50" title={userEmail}>
				<UserIcon class="w-3.5 h-3.5 text-zinc-500 shrink-0" />
				<span class="text-zinc-400 truncate">{userEmail}</span>
			</div>
		{:else}
			<div class="text-xs text-zinc-600 px-2">
				{authEnabled ? 'Not authenticated' : 'Auth disabled'}
			</div>
		{/if}
	</div>
</aside>

<header class="mobile-topbar fixed inset-x-0 top-0 z-40 flex items-end justify-between border-b border-zinc-800 bg-zinc-900/95 px-4 pb-3 backdrop-blur md:hidden">
	<a href="/" class="flex min-h-11 items-center gap-2.5" aria-label="iDRAC Manager dashboard">
		<div class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-600">
			<Cpu class="h-4 w-4 text-white" />
		</div>
		<span class="text-sm font-semibold text-zinc-100">iDRAC Manager</span>
	</a>
	{#if authEnabled && userEmail && userEmail !== 'anonymous'}
		<div class="flex max-w-32 items-center gap-1.5 text-xs text-zinc-400" title={userEmail}>
			<UserIcon class="h-4 w-4 shrink-0" />
			<span class="truncate">{userEmail}</span>
		</div>
	{:else}
		<span class="text-xs text-zinc-500">{authEnabled ? 'Not authenticated' : 'Auth disabled'}</span>
	{/if}
</header>

<nav class="mobile-bottom-nav fixed inset-x-0 bottom-0 z-40 grid grid-cols-5 border-t border-zinc-800 bg-zinc-900/95 backdrop-blur md:hidden" aria-label="Primary navigation">
	{#each nav as { href, label, icon: Icon }}
		<a
			{href}
			class="flex min-h-16 min-w-0 flex-col items-center justify-center gap-1 px-1 text-[11px] transition-colors
				{active === href || (href !== '/' && active.startsWith(href))
				? 'text-blue-400'
				: 'text-zinc-500 active:bg-zinc-800'}"
		>
			<Icon class="h-5 w-5" />
			<span class="max-w-full truncate">{label}</span>
		</a>
	{/each}
</nav>
