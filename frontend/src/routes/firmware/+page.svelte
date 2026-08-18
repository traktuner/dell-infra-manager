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
	let pendingBulk = $state<{
		component: string; catalogPath: string; version: string; servers: Server[]
	} | null>(null);
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
			if (r.status === 'fulfilled') inv.set(servers[i].id, r.value ?? []);
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
			servers.map(async (s) => {
				const comparison = await api.firmware.available(s.id);
				const inventory = await api.cache.firmware(s.id);
				return { comparison, inventory };
			})
		);
		const upd = new Map<string, AvailableUpdate[]>();
		const inv = new Map(inventories);
		const serverErrors: string[] = [];
		results.forEach((r, i) => {
			if (r.status === 'fulfilled') {
				// Backend returns null for empty Go slices — coerce to [] so
				// downstream .length / .reduce never crashes.
				upd.set(servers[i].id, r.value.comparison ?? []);
				inv.set(servers[i].id, r.value.inventory ?? []);
			} else {
				serverErrors.push(`${servers[i].name}: ${(r.reason as Error).message}`);
			}
		});
		updates = upd;
		inventories = inv;
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
		expanded = new Set(servers.filter((s) => (updates.get(s.id) ?? []).some((u) => u.outdated)).map((s) => s.id));
	}

	function collapseAll() {
		expanded = new Set();
	}

	// Backend now returns matched-AND-current entries too, so we filter by the
	// `outdated` flag instead of just counting list length.
	const totalOutdated = $derived(
		[...updates.values()].reduce(
			(acc, list) => acc + (list?.filter((u) => u.outdated).length ?? 0),
			0
		)
	);
	const totalUnknown = $derived(
		[...updates.values()].reduce(
			(acc, list) => acc + (list?.filter((u) => u.comparison_status === 'unknown').length ?? 0),
			0
		)
	);
	const totalUnchecked = $derived(servers.filter((s) => !updates.has(s.id)).length);

	const serversWithUpdates = $derived(
		servers.filter((s) => (updates.get(s.id) ?? []).some((u) => u.outdated))
	);

	// Unique outdated components across the fleet, grouped for bulk-update.
	const componentsWithUpdates = $derived.by(() => {
		const map = new Map<string, { component: string; servers: Server[]; available_version: string; catalog_path: string }>();
		for (const s of servers) {
			for (const u of updates.get(s.id) ?? []) {
				if (!u.outdated || !u.updateable) continue;
				const key = `${u.catalog_path}\u0000${u.available_version}`;
				const e = map.get(key);
				if (e) {
					if (!e.servers.some((server) => server.id === s.id)) e.servers.push(s);
				} else {
					map.set(key, {
						component: u.component,
						servers: [s],
						available_version: u.available_version,
						catalog_path: u.catalog_path
					});
				}
			}
		}
		return [...map.values()].sort((a, b) => a.component.localeCompare(b.component));
	});

	async function bulkUpdateComponent() {
		if (!pendingBulk) return;
		const request = pendingBulk;
		pendingBulk = null;
		bulking = true;
		bulkError = '';
		try {
			await api.firmware.bulkUpdate(request.servers.map((s) => s.id), request.component, request.catalogPath, request.version);
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
				inventories = new Map(inventories).set(serverId, inv ?? []);
				updates = new Map(updates).set(serverId, upd ?? []);
			}
		);
	}

	onMount(load);
</script>

<div class="space-y-6">
	<div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
		<div class="min-w-0">
			<h1 class="text-xl font-semibold text-zinc-100">Firmware</h1>
			{#if !loading && updates.size > 0}
				<div class="text-sm text-zinc-500 mt-0.5">
						{#if totalOutdated === 0 && totalUnknown === 0 && totalUnchecked === 0}
							<span class="text-emerald-500">All servers up to date</span>
						{:else}
							{#if totalOutdated > 0}<span class="text-amber-400">{totalOutdated} outdated</span>{/if}
							{#if totalOutdated > 0 && (totalUnknown > 0 || totalUnchecked > 0)}<span class="text-zinc-600"> · </span>{/if}
							{#if totalUnknown > 0}<span class="text-zinc-300">{totalUnknown} unknown</span>{/if}
							{#if totalUnknown > 0 && totalUnchecked > 0}<span class="text-zinc-600"> · </span>{/if}
							{#if totalUnchecked > 0}<span class="text-zinc-400">{totalUnchecked} server{totalUnchecked > 1 ? 's' : ''} not checked</span>{/if}
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
		<div class="grid grid-cols-2 gap-2 sm:flex">
			<button
				onclick={collapseAll}
				disabled={expanded.size === 0}
				class="min-h-11 rounded-lg px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 disabled:opacity-30"
			>Collapse all</button>
			<button
				onclick={expandAll}
				disabled={serversWithUpdates.length === 0}
				class="min-h-11 rounded-lg px-3 py-1.5 text-sm text-zinc-400 hover:bg-zinc-800 disabled:opacity-30"
			>Expand outdated</button>
			<button
				onclick={checkAllUpdates}
				disabled={checking}
				class="col-span-2 flex min-h-11 items-center justify-center gap-2 rounded-lg bg-zinc-800 px-3 py-1.5 text-sm text-zinc-300 hover:bg-zinc-700 disabled:opacity-50 sm:col-span-1"
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
					{#each componentsWithUpdates as info}
						<button
							onclick={() => (pendingBulk = { component: info.component, catalogPath: info.catalog_path, version: info.available_version, servers: info.servers })}
						disabled={bulking}
						class="flex min-h-11 items-center gap-2 rounded-lg border border-blue-500/20 bg-blue-600/10 px-3 py-1.5 text-left text-xs
							text-blue-300 hover:bg-blue-600/20 disabled:opacity-50"
					>
						<ArrowUpCircle class="w-3.5 h-3.5" />
							<span class="font-medium">{info.component}</span>
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
					{@const updateCount = serverUpdates.filter((u) => u.outdated).length}
					{@const unknownCount = serverUpdates.filter((u) => u.comparison_status === 'unknown').length}
				{@const isOpen = expanded.has(server.id)}
				{@const checked = updates.has(server.id)}

				<div class="overflow-hidden rounded-xl border border-zinc-800 bg-zinc-900">
					<button
						onclick={() => toggle(server.id)}
						class="flex min-h-16 w-full flex-col items-stretch justify-between gap-3 px-4 py-3.5 text-left transition-colors hover:bg-zinc-800/50 sm:flex-row sm:items-center sm:px-5"
					>
						<div class="flex min-w-0 items-center gap-3">
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
						<div class="flex flex-wrap items-center gap-3 pl-10 sm:justify-end sm:pl-0">
							{#if !checked}
								<span class="text-xs text-zinc-600">Run "Check All Updates" to see status</span>
								{:else if updateCount === 0 && unknownCount === 0}
								<span class="flex items-center gap-1.5 text-xs text-emerald-500 bg-emerald-500/10 px-2.5 py-1 rounded-full">
									<Check class="w-3.5 h-3.5" /> Up to date
								</span>
								{:else if updateCount > 0}
									<span class="text-xs text-amber-300 bg-amber-500/10 px-2.5 py-1 rounded-full font-medium">
										{updateCount} update{updateCount > 1 ? 's' : ''} available
									</span>
								{:else}
									<span class="text-xs text-zinc-400 bg-zinc-800 px-2.5 py-1 rounded-full font-medium">
										{unknownCount} unknown
									</span>
							{/if}
						</div>
					</button>

					{#if isOpen}
						<div class="border-t border-zinc-800 px-4 py-4 sm:px-5">
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
									{checked}
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

{#if pendingBulk}
	<div role="dialog" aria-modal="true" tabindex="-1" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm sm:p-4"
		onclick={(e) => { if (e.target === e.currentTarget) pendingBulk = null; }} onkeydown={(e) => { if (e.key === 'Escape') pendingBulk = null; }}>
		<div role="document" class="max-h-[calc(100dvh-1.5rem)] w-full max-w-lg overflow-y-auto rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl"
		>
			<div class="border-b border-zinc-800 px-4 py-5 sm:px-6">
				<h3 class="font-semibold text-zinc-100">Stage firmware on {pendingBulk.servers.length} servers?</h3>
				<p class="text-xs text-zinc-500 mt-1">{pendingBulk.component} → {pendingBulk.version}</p>
			</div>
			<div class="space-y-3 px-4 py-4 text-sm text-zinc-300 sm:px-6">
				<p>Targets: {pendingBulk.servers.map((s) => s.name).join(', ')}</p>
				<p class="text-amber-300">Each package uses OnReset. This action does not restart, power off, or reboot any managed server.</p>
				<p class="text-zinc-500">The appliance validates the current Dell match again for every server before it creates a job.</p>
			</div>
			<div class="flex flex-col-reverse gap-2 rounded-b-xl border-t border-zinc-800 bg-zinc-950/40 px-4 py-3 sm:flex-row sm:justify-end sm:px-6">
				<button onclick={() => (pendingBulk = null)} class="min-h-11 w-full rounded-lg bg-zinc-800 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-700 sm:w-auto">Cancel</button>
				<button onclick={bulkUpdateComponent} class="min-h-11 w-full rounded-lg bg-amber-600 px-4 py-2 text-sm text-white hover:bg-amber-500 sm:w-auto">Stage all with OnReset</button>
			</div>
		</div>
	</div>
{/if}
