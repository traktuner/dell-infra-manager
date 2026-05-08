<script lang="ts">
	import type { Server, FirmwareComponent, AvailableUpdate } from '$lib/types';
	import { api } from '$lib/api';
	import FirmwareTable from '$lib/components/FirmwareTable.svelte';
	import { onMount } from 'svelte';
	import {
		ChevronDown, ChevronRight, RefreshCw, ArrowUpCircle,
		Check, AlertCircle, Server as ServerIcon
	} from '@lucide/svelte';

	let servers = $state<Server[]>([]);
	let inventories = $state<Map<string, FirmwareComponent[]>>(new Map());
	let updates = $state<Map<string, AvailableUpdate[]>>(new Map());
	let loading = $state(true);
	let checking = $state(false);
	let expanded = $state<Set<string>>(new Set());
	let bulking = $state(false);
	let bulkError = $state('');
	let catalogInfo = $state<{
		available: boolean;
		date_time?: string;
		version?: string;
		fetched_at?: string;
	} | null>(null);

	async function load() {
		loading = true;
		const [serverList, info] = await Promise.all([api.servers.list(), api.catalog.info()]);
		servers = serverList;
		catalogInfo = info;
		const results = await Promise.allSettled(servers.map((s) => api.cache.firmware(s.id)));
		const inv = new Map<string, FirmwareComponent[]>();
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') inv.set(servers[i].id, r.value);
		});
		inventories = inv;
		loading = false;
	}

	// "Check for Updates" first triggers a conditional re-download of the
	// Dell catalog (cheap 304 if nothing changed) and THEN re-runs the
	// comparison for every server. The previous version compared against a
	// stale local copy, so the button could only ever return the same
	// updates over and over.
	async function checkAllUpdates() {
		checking = true;
		bulkError = '';
		try {
			const refreshed = await api.catalog.refresh();
			catalogInfo = { available: true, ...refreshed };
		} catch (e) {
			bulkError = `Catalog refresh failed: ${(e as Error).message}`;
		}
		const results = await Promise.allSettled(
			servers.map((s) => api.firmware.available(s.id))
		);
		const upd = new Map<string, AvailableUpdate[]>();
		const serverErrors: string[] = [];
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') {
				upd.set(servers[i].id, r.value);
			} else {
				serverErrors.push(`${servers[i].name}: ${(r.reason as Error).message}`);
			}
		});
		updates = upd;
		if (serverErrors.length > 0 && !bulkError) {
			bulkError = serverErrors.join(' · ');
		}
		checking = false;
	}

	function formatCatalogDate(s: string | undefined): string {
		if (!s) return '—';
		try {
			return new Date(s).toLocaleDateString();
		} catch {
			return s;
		}
	}

	function toggle(serverId: string) {
		const next = new Set(expanded);
		if (next.has(serverId)) next.delete(serverId);
		else next.add(serverId);
		expanded = next;
	}

	function expandAll() {
		expanded = new Set(servers.filter((s) => (updates.get(s.id)?.length ?? 0) > 0).map((s) => s.id));
	}

	function collapseAll() {
		expanded = new Set();
	}

	const totalOutdated = $derived(
		[...updates.values()].reduce((acc, list) => acc + list.length, 0)
	);

	const serversWithUpdates = $derived(servers.filter((s) => (updates.get(s.id)?.length ?? 0) > 0));

	// All unique components that have updates available across the fleet,
	// grouped so we can offer "Update <component> on all servers" bulk actions.
	const componentsWithUpdates = $derived.by(() => {
		const map = new Map<string, { servers: Server[]; available_version: string; catalog_path: string }>();
		for (const s of servers) {
			for (const u of updates.get(s.id) ?? []) {
				const e = map.get(u.component);
				if (e) {
					e.servers.push(s);
				} else {
					map.set(u.component, {
						servers: [s],
						available_version: u.available_version,
						catalog_path: u.catalog_path
					});
				}
			}
		}
		return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]));
	});

	async function bulkUpdateComponent(component: string, catalogPath: string, serverIds: string[]) {
		if (!confirm(`Queue ${component} update on ${serverIds.length} server(s)?`)) return;
		bulking = true;
		bulkError = '';
		try {
			await api.firmware.bulkUpdate(serverIds, component, catalogPath);
		} catch (e) {
			bulkError = (e as Error).message;
		} finally {
			bulking = false;
		}
	}

	function refreshServer(serverId: string) {
		// re-fetch inventory and updates for one server (e.g. after queueing an update)
		Promise.all([api.cache.firmware(serverId), api.firmware.available(serverId)]).then(
			([inv, upd]) => {
				inventories = new Map(inventories).set(serverId, inv);
				updates = new Map(updates).set(serverId, upd);
			}
		);
	}

	onMount(load);
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-xl font-semibold text-zinc-100">Firmware</h1>
			{#if !loading && updates.size > 0}
				<div class="text-sm text-zinc-500 mt-0.5">
					{#if totalOutdated === 0}
						<span class="text-emerald-500">All servers up to date</span>
					{:else}
						<span class="text-amber-400">{totalOutdated}</span> outdated component{totalOutdated > 1 ? 's' : ''}
						across <span class="text-zinc-300">{serversWithUpdates.length}</span> server{serversWithUpdates.length > 1 ? 's' : ''}
					{/if}
				</div>
			{/if}
			{#if catalogInfo?.available}
				<div class="text-xs text-zinc-600 mt-1">
					Dell catalog
					{#if catalogInfo.version}<span class="text-zinc-500">v{catalogInfo.version}</span>{/if}
					· dated <span class="text-zinc-500">{formatCatalogDate(catalogInfo.date_time)}</span>
					· last fetched <span class="text-zinc-500">{formatCatalogDate(catalogInfo.fetched_at)}</span>
				</div>
			{:else if catalogInfo}
				<div class="text-xs text-amber-500 mt-1">
					No Dell catalog cached yet — first "Check for Updates" will download it.
				</div>
			{/if}
		</div>
		<div class="flex gap-2">
			<button
				onclick={collapseAll}
				disabled={expanded.size === 0}
				class="px-3 py-1.5 text-sm rounded-lg text-zinc-400 hover:bg-zinc-800 disabled:opacity-30"
			>Collapse all</button>
			<button
				onclick={expandAll}
				disabled={serversWithUpdates.length === 0}
				class="px-3 py-1.5 text-sm rounded-lg text-zinc-400 hover:bg-zinc-800 disabled:opacity-30"
			>Expand outdated</button>
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

	{#if bulkError}
		<div class="flex items-center gap-2 text-red-400 bg-red-500/10 rounded-xl px-4 py-3 text-sm">
			<AlertCircle class="w-4 h-4" /> {bulkError}
		</div>
	{/if}

	<!-- Cross-server bulk update bar — only when there's something to do -->
	{#if componentsWithUpdates.length > 0}
		<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
			<h3 class="text-xs font-medium text-zinc-500 uppercase tracking-wide mb-3">
				Bulk update across fleet
			</h3>
			<div class="flex flex-wrap gap-2">
				{#each componentsWithUpdates as [component, info]}
					<button
						onclick={() => bulkUpdateComponent(component, info.catalog_path, info.servers.map((s) => s.id))}
						disabled={bulking}
						class="flex items-center gap-2 px-3 py-1.5 text-xs rounded-lg bg-blue-600/10 border border-blue-500/20
							text-blue-300 hover:bg-blue-600/20 disabled:opacity-50"
					>
						<ArrowUpCircle class="w-3.5 h-3.5" />
						<span class="font-medium">{component}</span>
						<span class="text-blue-400/60 font-mono">→ {info.available_version}</span>
						<span class="text-zinc-500">on {info.servers.length} server{info.servers.length > 1 ? 's' : ''}</span>
					</button>
				{/each}
			</div>
		</div>
	{/if}

	{#if loading}
		<div class="text-zinc-500 text-sm">Loading firmware data...</div>
	{:else if servers.length === 0}
		<div class="text-zinc-600 text-sm py-12 text-center">
			No servers configured. Add one on the <a href="/servers" class="text-blue-400 hover:underline">Servers</a> page.
		</div>
	{:else}
		<div class="space-y-2">
			{#each servers as server}
				{@const components = inventories.get(server.id) ?? []}
				{@const serverUpdates = updates.get(server.id) ?? []}
				{@const updateCount = serverUpdates.length}
				{@const isOpen = expanded.has(server.id)}
				{@const checked = updates.has(server.id)}

				<div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
					<button
						onclick={() => toggle(server.id)}
						class="w-full flex items-center justify-between px-5 py-3.5 hover:bg-zinc-800/50 transition-colors"
					>
						<div class="flex items-center gap-3">
							{#if isOpen}
								<ChevronDown class="w-4 h-4 text-zinc-500" />
							{:else}
								<ChevronRight class="w-4 h-4 text-zinc-500" />
							{/if}
							<ServerIcon class="w-4 h-4 text-zinc-500" />
							<div class="text-left">
								<div class="text-sm font-medium text-zinc-200">{server.name}</div>
								<div class="text-xs text-zinc-600">{server.hostname} · {components.length} components</div>
							</div>
						</div>
						<div class="flex items-center gap-3">
							{#if !checked}
								<span class="text-xs text-zinc-600">Run "Check All Updates" to see status</span>
							{:else if updateCount === 0}
								<span class="flex items-center gap-1.5 text-xs text-emerald-500 bg-emerald-500/10 px-2.5 py-1 rounded-full">
									<Check class="w-3.5 h-3.5" /> Up to date
								</span>
							{:else}
								<span class="text-xs text-amber-300 bg-amber-500/10 px-2.5 py-1 rounded-full font-medium">
									{updateCount} update{updateCount > 1 ? 's' : ''} available
								</span>
							{/if}
						</div>
					</button>

					{#if isOpen}
						<div class="border-t border-zinc-800 px-5 py-4">
							{#if components.length === 0}
								<div class="text-zinc-600 text-sm py-4 text-center">
									No firmware inventory yet. The poller fetches firmware once per startup
									and every 6 hours after that.
								</div>
							{:else}
								<FirmwareTable
									serverId={server.id}
									{components}
									updates={serverUpdates}
									onupdate={() => refreshServer(server.id)}
								/>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
