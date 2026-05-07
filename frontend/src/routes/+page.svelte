<script lang="ts">
	import type { Server, ServerCache, Dashboard, WSEvent } from '$lib/types';
	import { api } from '$lib/api';
	import { wsManager } from '$lib/websocket';
	import ServerCard from '$lib/components/ServerCard.svelte';
	import { onMount } from 'svelte';
	import { Server as ServerIcon, Wifi, WifiOff, ListChecks, AlertCircle } from '@lucide/svelte';

	let servers = $state<Server[]>([]);
	let caches = $state<Map<string, ServerCache>>(new Map());
	let dashboard = $state<Dashboard | null>(null);
	let loading = $state(true);

	async function load() {
		const [s, d] = await Promise.all([api.servers.list(), api.dashboard()]);
		servers = s;
		dashboard = d;
		// Load summaries in parallel
		const results = await Promise.allSettled(s.map((srv) => api.cache.summary(srv.id)));
		const next = new Map<string, ServerCache>();
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') next.set(s[i].id, r.value);
		});
		caches = next;
		loading = false;
	}

	onMount(() => {
		load();
		// Live updates
		const unsub = wsManager.on('server_status', (e: WSEvent) => {
			if (e.server_id) {
				const cache = caches.get(e.server_id);
				if (cache) {
					caches = new Map(caches).set(e.server_id, {
						...cache,
						status: (e.data.status as ServerCache['status']) ?? cache.status
					});
				}
			}
		});
		return unsub;
	});
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-zinc-100">Dashboard</h1>
		<button
			onclick={load}
			class="px-3 py-1.5 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
		>
			Refresh
		</button>
	</div>

	<!-- Summary bar -->
	{#if dashboard}
		<div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
			<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
				<div class="flex items-center gap-2 text-zinc-500 text-xs mb-1.5">
					<ServerIcon class="w-3.5 h-3.5" /> Total Servers
				</div>
				<div class="text-2xl font-bold text-zinc-100">{dashboard.total_servers}</div>
			</div>
			<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
				<div class="flex items-center gap-2 text-emerald-500 text-xs mb-1.5">
					<Wifi class="w-3.5 h-3.5" /> Online
				</div>
				<div class="text-2xl font-bold text-emerald-400">{dashboard.online}</div>
			</div>
			<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
				<div class="flex items-center gap-2 text-red-500 text-xs mb-1.5">
					<WifiOff class="w-3.5 h-3.5" /> Offline
				</div>
				<div class="text-2xl font-bold text-red-400">{dashboard.offline}</div>
			</div>
			<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
				<div class="flex items-center gap-2 text-blue-500 text-xs mb-1.5">
					<ListChecks class="w-3.5 h-3.5" /> Active Jobs
				</div>
				<div class="text-2xl font-bold text-blue-400">{dashboard.active_jobs}</div>
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each Array(3) as _}
				<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5 animate-pulse h-40"></div>
			{/each}
		</div>
	{:else if servers.length === 0}
		<div class="flex flex-col items-center justify-center py-20 text-zinc-600">
			<AlertCircle class="w-10 h-10 mb-3" />
			<p class="text-lg font-medium">No servers yet</p>
			<p class="text-sm mt-1">
				<a href="/servers" class="text-blue-400 hover:underline">Add your first server</a>
			</p>
		</div>
	{:else}
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each servers as server}
				<ServerCard {server} cache={caches.get(server.id) ?? null} />
			{/each}
		</div>
	{/if}
</div>
