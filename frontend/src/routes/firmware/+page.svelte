<script lang="ts">
	import type { Server, FirmwareComponent, AvailableUpdate } from '$lib/types';
	import { api } from '$lib/api';
	import { onMount } from 'svelte';
	import { ArrowUpCircle, RefreshCw } from '@lucide/svelte';

	let servers = $state<Server[]>([]);
	let inventories = $state<Map<string, FirmwareComponent[]>>(new Map());
	let updates = $state<Map<string, AvailableUpdate[]>>(new Map());
	let loading = $state(true);
	let checking = $state(false);

	// All unique component names across all servers
	const allComponents = $derived(() => {
		const names = new Set<string>();
		for (const comps of inventories.values()) {
			comps.forEach((c) => names.add(c.Name));
		}
		return [...names].sort();
	});

	async function load() {
		loading = true;
		servers = await api.servers.list();
		const results = await Promise.allSettled(
			servers.map((s) => api.cache.firmware(s.id))
		);
		const inv = new Map<string, FirmwareComponent[]>();
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') inv.set(servers[i].id, r.value);
		});
		inventories = inv;
		loading = false;
	}

	async function checkAllUpdates() {
		checking = true;
		const results = await Promise.allSettled(
			servers.map((s) => api.firmware.available(s.id))
		);
		const upd = new Map<string, AvailableUpdate[]>();
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') upd.set(servers[i].id, r.value);
		});
		updates = upd;
		checking = false;
	}

	onMount(load);

	function getVersion(serverId: string, component: string) {
		return inventories.get(serverId)?.find((c) => c.Name === component)?.Version ?? '—';
	}

	function getUpdate(serverId: string, component: string) {
		return updates.get(serverId)?.find((u) => u.component === component);
	}

	async function bulkUpdateComponent(component: string) {
		const serverIds = servers.filter((s) => {
			const upd = getUpdate(s.id, component);
			return !!upd;
		}).map((s) => s.id);

		if (serverIds.length === 0) return;
		const upd = getUpdate(serverIds[0], component)!;
		try {
			await api.firmware.bulkUpdate(serverIds, component, upd.catalog_path);
			alert(`Queued ${component} update for ${serverIds.length} server(s)`);
		} catch (e) {
			alert((e as Error).message);
		}
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-zinc-100">Firmware Overview</h1>
		<div class="flex gap-2">
			<button onclick={load} class="px-3 py-1.5 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700">
				Refresh
			</button>
			<button
				onclick={checkAllUpdates}
				disabled={checking}
				class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700 disabled:opacity-50"
			>
				<RefreshCw class="w-4 h-4 {checking ? 'animate-spin' : ''}" />
				{checking ? 'Checking...' : 'Check All Updates'}
			</button>
		</div>
	</div>

	{#if loading}
		<div class="text-zinc-500 text-sm">Loading firmware data...</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm border border-zinc-800 rounded-xl overflow-hidden">
				<thead>
					<tr class="bg-zinc-900 border-b border-zinc-800">
						<th class="px-4 py-3 text-left text-xs font-medium text-zinc-500 uppercase tracking-wide">Component</th>
						{#each servers as server}
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-500 uppercase tracking-wide">
								<a href="/servers/{server.id}" class="hover:text-zinc-300">{server.name}</a>
							</th>
						{/each}
						<th class="px-4 py-3"></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-zinc-800 bg-zinc-900">
					{#each allComponents() as component}
						{@const hasUpdate = servers.some((s) => !!getUpdate(s.id, component))}
						<tr class="{hasUpdate ? 'bg-amber-500/5' : ''}">
							<td class="px-4 py-3 font-medium text-zinc-300">{component}</td>
							{#each servers as server}
								{@const upd = getUpdate(server.id, component)}
								{@const ver = getVersion(server.id, component)}
								<td class="px-4 py-3">
									{#if ver === '—'}
										<span class="text-zinc-700 text-xs">N/A</span>
									{:else if upd}
										<div class="text-xs">
											<div class="text-zinc-400 font-mono">{ver}</div>
											<div class="text-amber-400 font-mono">→ {upd.available_version}</div>
										</div>
									{:else}
										<span class="text-zinc-400 font-mono text-xs">{ver}</span>
									{/if}
								</td>
							{/each}
							<td class="px-4 py-3">
								{#if hasUpdate}
									<button
										onclick={() => bulkUpdateComponent(component)}
										class="flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg bg-blue-600/20
											text-blue-400 hover:bg-blue-600/30 whitespace-nowrap"
									>
										<ArrowUpCircle class="w-3.5 h-3.5" />
										Update All
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
