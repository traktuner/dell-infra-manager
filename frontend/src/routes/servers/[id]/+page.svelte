<script lang="ts">
	import { page } from '$app/state';
	import type {
		Server,
		ServerCache,
		SystemInfo,
		ThermalInfo,
		PowerInfo,
		StorageDetail,
		LogEntry,
		FirmwareComponent,
		AvailableUpdate,
		BiosRegistryEntry,
		IDRACJob,
		Job,
		WSEvent
	} from '$lib/types';
	import { api } from '$lib/api';
	import { wsManager } from '$lib/websocket';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import PowerButton from '$lib/components/PowerButton.svelte';
	import JobQueue from '$lib/components/JobQueue.svelte';
	import FirmwareTable from '$lib/components/FirmwareTable.svelte';
	import BiosAttributeEditor from '$lib/components/BiosAttributeEditor.svelte';
	import VirtualMediaPanel from '$lib/components/VirtualMediaPanel.svelte';
	import ConsolePanel from '$lib/components/ConsolePanel.svelte';
	import { onMount } from 'svelte';
	import {
		Thermometer, Zap, HardDrive, FileText, Cpu, Wind, Database,
		Trash2, ChevronLeft, ChevronRight, RefreshCw, Search, Clock,
		AlertCircle, Settings, Disc, ListChecks, SquareTerminal
	} from '@lucide/svelte';

	const id = $derived(page.params.id);

	type TabKey = 'overview' | 'storage' | 'eventlog' | 'firmware' | 'bios' | 'virtualmedia' | 'jobs' | 'console';

	let server = $state<Server | null>(null);
	let cache = $state<ServerCache | null>(null);
	let localJobs = $state<Job[]>([]);
	let tab = $state<TabKey>('overview');

	// Lazy-loaded tab state. All "loaded" flags are $state too so we never mix
	// reactive and non-reactive booleans for the same render path.
	let storage = $state<StorageDetail[]>([]);
	let storageLoading = $state(false);
	let storageError = $state('');
	let storageLoaded = $state(false);

	let logEntries = $state<LogEntry[]>([]);
	let logTotal = $state(0);
	let logSkip = $state(0);
	let logLoading = $state(false);
	let logError = $state('');
	const logTop = 100;
	let logLoaded = $state(false);

	let fwComponents = $state<FirmwareComponent[]>([]);
	let fwUpdates = $state<AvailableUpdate[]>([]);
	let fwLoading = $state(false);
	let fwError = $state('');
	let fwChecking = $state(false);
	let fwLoaded = $state(false);

	let biosAttributes = $state<Record<string, unknown>>({});
	let biosRegistry = $state<Map<string, BiosRegistryEntry>>(new Map());
	let biosPending = $state<Job[]>([]);
	let biosLoading = $state(false);
	let biosError = $state('');
	let biosSearch = $state('');
	let biosEditing = $state<BiosRegistryEntry | null>(null);
	let biosLoaded = $state(false);

	let idracJobs = $state<IDRACJob[]>([]);
	let idracJobsLoading = $state(false);
	let idracJobsError = $state('');
	let idracJobsLoaded = $state(false);

	const system = $derived<SystemInfo | null>(
		cache?.system_json ? JSON.parse(cache.system_json) : null
	);
	const thermal = $derived<ThermalInfo | null>(
		cache?.thermal_json ? JSON.parse(cache.thermal_json) : null
	);
	const power = $derived<PowerInfo | null>(
		cache?.power_json ? JSON.parse(cache.power_json) : null
	);

	async function load() {
		const [s, c, j] = await Promise.allSettled([
			api.servers.get(id),
			api.cache.summary(id),
			api.jobs.forServer(id)
		]);
		if (s.status === 'fulfilled') server = s.value;
		if (c.status === 'fulfilled') cache = c.value;
		if (j.status === 'fulfilled') localJobs = j.value ?? [];
	}

	async function loadStorage() {
		storageLoading = true;
		storageError = '';
		try {
			storage = (await api.cache.storage(id)) ?? [];
			storageLoaded = true;
		} catch (e) {
			storageError = (e as Error).message;
		} finally {
			storageLoading = false;
		}
	}

	async function loadEventLog() {
		logLoading = true;
		logError = '';
		try {
			const result = await api.eventlog.get(id, logTop, logSkip);
			logEntries = result.Members ?? [];
			logTotal = result['Members@odata.count'] ?? logEntries.length;
			logLoaded = true;
		} catch (e) {
			logError = (e as Error).message;
			logEntries = [];
		} finally {
			logLoading = false;
		}
	}

	async function loadFirmware() {
		fwLoading = true;
		fwError = '';
		try {
			// Load inventory + available updates in parallel so the user sees
			// outdated components immediately, without a separate "Check Updates" click.
			const [comps, upds] = await Promise.all([
				api.cache.firmware(id),
				api.firmware.available(id).catch(() => [])
			]);
			// Backend may return null for empty Go slices — coerce to [].
			fwComponents = comps ?? [];
			fwUpdates = upds ?? [];
			fwLoaded = true;
		} catch (e) {
			fwError = (e as Error).message;
		} finally {
			fwLoading = false;
		}
	}

	async function checkFirmwareUpdates() {
		fwChecking = true;
		fwError = '';
		try {
			fwUpdates = (await api.firmware.available(id, true)) ?? [];
		} catch (e) {
			fwError = (e as Error).message;
		} finally {
			fwChecking = false;
		}
	}

	// loadBios: schneller Pfad — Attribute + pending Jobs sofort.
	// Die Registry (~2 MB) wird im Hintergrund nachgeladen; sobald sie da ist
	// erscheinen die Edit/Read-only-Labels. Edit-Klicks vorher warten kurz auf
	// die Registry und öffnen dann das Modal.
	async function loadBios() {
		biosLoading = true;
		biosError = '';
		try {
			const [biosResp, pending] = await Promise.all([
				api.bios.get(id),
				api.bios.pending(id)
			]);
			// Defensive: backend may return null for missing/empty fields.
			// Object.keys(null) throws TypeError, so always coerce to {}/[].
			biosAttributes = biosResp?.Attributes ?? {};
			biosPending = pending ?? [];
			biosLoaded = true;
			// Fire-and-forget — UI updates reactively when registry arrives.
			ensureBiosRegistry().catch(() => {});
		} catch (e) {
			biosError = (e as Error).message;
		} finally {
			biosLoading = false;
		}
	}

	// Registry wird einmal geladen; mehrfache Aufrufe teilen sich den Promise.
	let biosRegistryLoaded = $state(false);
	let biosRegistryPromise: Promise<void> | null = null;
	function ensureBiosRegistry(): Promise<void> {
		if (biosRegistryLoaded) return Promise.resolve();
		if (biosRegistryPromise) return biosRegistryPromise;
		biosRegistryPromise = (async () => {
			const registry = await api.bios.registry(id);
			const map = new Map<string, BiosRegistryEntry>();
			for (const entry of registry.RegistryEntries?.Attributes ?? []) {
				map.set(entry.AttributeName, {
					...entry,
					current_value: biosAttributes[entry.AttributeName],
					AllowedValues: entry.Value?.map((v) => v.ValueName) ?? []
				});
			}
			biosRegistry = map;
			biosRegistryLoaded = true;
		})();
		return biosRegistryPromise;
	}

	async function loadIdracJobs() {
		idracJobsLoading = true;
		idracJobsError = '';
		try {
			idracJobs = (await api.jobs.idrac(id)) ?? [];
			idracJobsLoaded = true;
		} catch (e) {
			idracJobsError = (e as Error).message;
		} finally {
			idracJobsLoading = false;
		}
	}

	async function clearLog() {
		if (!confirm('Clear the entire Lifecycle Controller log?')) return;
		await api.eventlog.clear(id);
		await loadEventLog();
	}

	function selectTab(key: TabKey) {
		tab = key;
		if (key === 'storage' && !storageLoaded) loadStorage();
		if (key === 'eventlog' && !logLoaded) loadEventLog();
		if (key === 'firmware' && !fwLoaded) loadFirmware();
		if (key === 'bios' && !biosLoaded) loadBios();
		if (key === 'jobs' && !idracJobsLoaded) loadIdracJobs();
	}

	async function openBiosEdit(attrName: string) {
		await ensureBiosRegistry();
		const entry = biosRegistry.get(attrName);
		if (!entry || entry.ReadOnly) return;
		biosEditing = { ...entry, current_value: biosAttributes[attrName] };
	}

	const filteredBiosEntries = $derived(
		Object.entries(biosAttributes).filter(
			([k]) =>
				!biosSearch ||
				k.toLowerCase().includes(biosSearch.toLowerCase()) ||
				biosRegistry.get(k)?.DisplayName.toLowerCase().includes(biosSearch.toLowerCase())
		)
	);

	function formatBytes(bytes: number) {
		const gb = bytes / 1e9;
		return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(bytes / 1e6).toFixed(0)} MB`;
	}

	function severityColor(s: string) {
		const m: Record<string, string> = {
			Critical: 'text-red-400',
			Warning: 'text-yellow-400',
			OK: 'text-zinc-500',
			Informational: 'text-zinc-500'
		};
		return m[s] ?? 'text-zinc-500';
	}

	function formatJobTime(s: string) {
		if (!s || s === '0001-01-01T00:00:00Z' || s === 'TIME_NA') return '—';
		try {
			return new Date(s).toLocaleString();
		} catch {
			return s;
		}
	}

	onMount(() => {
		load();
		const unsub = wsManager.on('thermal_update', (e: WSEvent) => {
			if (e.server_id === id) load();
		});
		const unsub2 = wsManager.on('job_update', (e: WSEvent) => {
			if (e.server_id === id) {
				api.jobs.forServer(id).then((j) => (localJobs = j ?? []));
				if (tab === 'jobs') loadIdracJobs();
			}
		});
		return () => { unsub(); unsub2(); };
	});

	// Tab ordering by usage frequency:
	//   1. Overview          — landing tab, all-at-a-glance
	//   2. Console           — KVM/SOL, used a lot during incidents → keep close
	//   3-6. Configuration   — hardware-config screens (Storage / Firmware / BIOS / Virtual Media)
	//   7-8. Forensics       — historical data (Event Log / Jobs)
	const tabs: { key: TabKey; label: string; icon: typeof Cpu; group?: 'config' | 'log' }[] = [
		{ key: 'overview',     label: 'Overview',      icon: Cpu },
		{ key: 'console',      label: 'Console',       icon: SquareTerminal },
		{ key: 'storage',      label: 'Storage',       icon: HardDrive,  group: 'config' },
		{ key: 'firmware',     label: 'Firmware',      icon: RefreshCw,  group: 'config' },
		{ key: 'bios',         label: 'BIOS',          icon: Settings,   group: 'config' },
		{ key: 'virtualmedia', label: 'Virtual Media', icon: Disc,       group: 'config' },
		{ key: 'eventlog',     label: 'Event Log',     icon: FileText,   group: 'log' },
		{ key: 'jobs',         label: 'Jobs',          icon: ListChecks, group: 'log' }
	];
</script>

{#if !server}
	<div class="text-zinc-500">Loading...</div>
{:else}
	<div class="space-y-6">
		<!-- Header -->
		<div class="flex items-start justify-between">
			<div>
				<h1 class="text-xl font-semibold text-zinc-100">{server.name}</h1>
				<div class="text-sm text-zinc-500 mt-0.5">{server.hostname}:{server.port}</div>
				{#if system}
					<div class="text-xs text-zinc-600 mt-0.5">{system.Model} · {system.SerialNumber}</div>
				{/if}
			</div>
			<!-- Status badges with explicit labels so it's obvious which is which.
			     Reachability is implied by Power/Health when system data is fresh — only
			     show the offline pill if we explicitly can't reach the BMC. -->
			<div class="flex items-center gap-3 text-xs">
				{#if cache?.status === 'offline' || cache?.status === 'unknown'}
					<span class="flex items-center gap-1.5">
						<span class="text-zinc-600">BMC</span>
						<StatusBadge status={cache?.status ?? 'unknown'} size="sm" />
					</span>
				{/if}
				{#if system}
					<span class="flex items-center gap-1.5" title="Server power state">
						<span class="text-zinc-600">Power</span>
						<StatusBadge status={system.PowerState} size="sm" />
					</span>
					<span class="flex items-center gap-1.5" title="Hardware health (sensors / drives / fans)">
						<span class="text-zinc-600">Health</span>
						<StatusBadge status={system.Status.Health} size="sm" />
					</span>
				{/if}
			</div>
		</div>

		<div class="flex items-end gap-1 border-b border-zinc-800 overflow-x-auto">
			{#each tabs as t, i}
				{@const prev = tabs[i - 1]}
				{#if i > 0 && prev?.group !== t.group}
					<!-- visual divider between Overview/Console · Config · Logs groups -->
					<span class="w-px h-5 bg-zinc-800 mx-2 self-center"></span>
				{/if}
				<button
					onclick={() => selectTab(t.key)}
					class="flex items-center gap-2 px-4 py-2.5 text-sm border-b-2 transition-colors whitespace-nowrap
						{tab === t.key
						? 'border-blue-500 text-blue-400'
						: 'border-transparent text-zinc-500 hover:text-zinc-300'}"
				>
					<t.icon class="w-4 h-4" />
					{t.label}
				</button>
			{/each}
		</div>

		<!-- Tab content — one independent if-block per tab so a render fault
		     in one can't break the chain for the others. -->
		{#if tab === 'overview'}
			<div class="grid grid-cols-3 gap-6">
				<div class="col-span-2 space-y-4">
					{#if system}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<h3 class="text-sm font-medium text-zinc-400 mb-3 flex items-center gap-2">
								<Cpu class="w-4 h-4" /> System
							</h3>
							<div class="grid grid-cols-2 gap-3 text-sm">
								<div class="text-zinc-500">Model</div><div class="text-zinc-200">{system.Model}</div>
								<div class="text-zinc-500">Service Tag</div><div class="text-zinc-200 font-mono">{system.SerialNumber}</div>
								<div class="text-zinc-500">BIOS</div><div class="text-zinc-200 font-mono">{system.BiosVersion}</div>
								<div class="text-zinc-500">CPUs</div><div class="text-zinc-200">{system.ProcessorSummary.Count}</div>
								<div class="text-zinc-500">RAM</div><div class="text-zinc-200">{system.MemorySummary.TotalSystemMemoryGiB} GiB</div>
							</div>
						</div>
					{/if}

					{#if power}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<h3 class="text-sm font-medium text-zinc-400 mb-3 flex items-center gap-2">
								<Zap class="w-4 h-4" /> Power
							</h3>
							{#each power.PowerControl as pc}
								<div class="mb-2">
									<div class="flex justify-between text-sm mb-1">
										<span class="text-zinc-400">{pc.Name}</span>
										<span class="text-zinc-200 font-mono">{Math.round(pc.PowerConsumedWatts)}W / {Math.round(pc.PowerCapacityWatts)}W</span>
									</div>
									<div class="w-full bg-zinc-800 rounded-full h-1.5">
										<div class="bg-blue-500 h-1.5 rounded-full"
											style="width: {Math.min(100, (pc.PowerConsumedWatts / pc.PowerCapacityWatts) * 100).toFixed(1)}%">
										</div>
									</div>
								</div>
							{/each}
							<div class="mt-3 space-y-2">
								{#each power.PowerSupplies as psu}
									<div class="flex items-center justify-between text-xs text-zinc-500">
										<span>{psu.Name}</span>
										<div class="flex items-center gap-3">
											<span>{psu.LineInputVoltage}V input · {Math.round(psu.LastPowerOutputWatts)}W out</span>
											<StatusBadge status={psu.Status.Health} size="sm" />
										</div>
									</div>
								{/each}
							</div>
						</div>
					{/if}

					{#if thermal}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<h3 class="text-sm font-medium text-zinc-400 mb-3 flex items-center gap-2">
								<Thermometer class="w-4 h-4" /> Temperature Sensors
							</h3>
							<div class="grid grid-cols-2 gap-2">
								{#each thermal.Temperatures.filter(t => t.ReadingCelsius > 0) as sensor}
									<div class="flex items-center justify-between text-sm bg-zinc-800/50 rounded-lg px-3 py-2">
										<span class="text-zinc-400 text-xs">{sensor.Name}</span>
										<span class="font-mono text-zinc-200">{sensor.ReadingCelsius}°C</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}

					{#if thermal?.Fans}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<h3 class="text-sm font-medium text-zinc-400 mb-3 flex items-center gap-2">
								<Wind class="w-4 h-4" /> Fans
							</h3>
							<div class="grid grid-cols-2 gap-2">
								{#each thermal.Fans.filter(f => f.Status.State !== 'Absent') as fan}
									<div class="flex items-center justify-between text-sm bg-zinc-800/50 rounded-lg px-3 py-2">
										<span class="text-zinc-400 text-xs">{fan.Name}</span>
										<span class="font-mono text-zinc-200">{Math.round(fan.Reading)} {fan.ReadingUnits}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>

				<div>
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
						<h3 class="text-sm font-medium text-zinc-400 mb-3">Power Actions</h3>
						<PowerButton serverId={id} powerState={system?.PowerState ?? ''} onchange={load} />
					</div>
				</div>
			</div>

		{/if}

		{#if tab === 'storage'}
			{#if storageLoading}
				<div class="text-zinc-500 text-sm">Loading storage...</div>
			{:else if storageError}
				<div class="text-red-400 text-sm">{storageError}</div>
			{:else if storage.length === 0}
				<div class="text-zinc-600 text-sm py-8 text-center">No storage data yet.</div>
			{:else}
				<div class="space-y-4">
					{#each storage as detail}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
							<div class="flex items-center gap-2 mb-4">
								<Database class="w-4 h-4 text-zinc-400" />
								<h3 class="font-medium text-zinc-200">{detail.Controller.Name}</h3>
								<StatusBadge status={detail.Controller.Status.Health} size="sm" />
							</div>

							{#if detail.Volumes?.length}
								<h4 class="text-xs font-medium text-zinc-500 uppercase tracking-wide mb-2">Volumes (RAID)</h4>
								<div class="space-y-2 mb-4">
									{#each detail.Volumes as vol}
										<div class="bg-zinc-800/50 rounded-lg px-4 py-3 flex items-center justify-between">
											<div>
												<div class="text-sm text-zinc-200">{vol.Name}</div>
												<div class="text-xs text-zinc-500">{vol.RAIDType} · {vol.VolumeType}</div>
											</div>
											<div class="flex items-center gap-3">
												<span class="text-sm text-zinc-400">{formatBytes(vol.CapacityBytes)}</span>
												<StatusBadge status={vol.Status.Health} size="sm" />
											</div>
										</div>
									{/each}
								</div>
							{/if}

							{#if detail.Drives?.length}
								<h4 class="text-xs font-medium text-zinc-500 uppercase tracking-wide mb-2">Physical Drives</h4>
								<div class="overflow-x-auto">
									<table class="w-full text-sm">
										<thead>
											<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs">
												<th class="pb-2 pr-4">Name</th>
												<th class="pb-2 pr-4">Type</th>
												<th class="pb-2 pr-4">Size</th>
												<th class="pb-2 pr-4">Protocol</th>
												<th class="pb-2 pr-4">Failure</th>
												<th class="pb-2">Health</th>
											</tr>
										</thead>
										<tbody class="divide-y divide-zinc-800/50">
											{#each detail.Drives as drive}
												<tr class={drive.FailurePredicted ? 'bg-red-500/5' : ''}>
													<td class="py-2 pr-4 text-zinc-300">{drive.Name}</td>
													<td class="py-2 pr-4 text-zinc-500">{drive.MediaType}</td>
													<td class="py-2 pr-4 text-zinc-400">{formatBytes(drive.CapacityBytes)}</td>
													<td class="py-2 pr-4 text-zinc-500">{drive.Protocol}</td>
													<td class="py-2 pr-4">
														{#if drive.FailurePredicted}
															<span class="text-red-400 text-xs font-medium">PREDICTED</span>
														{:else}
															<span class="text-zinc-700 text-xs">No</span>
														{/if}
													</td>
													<td class="py-2">
														<StatusBadge status={drive.Status.Health} size="sm" />
													</td>
												</tr>
											{/each}
										</tbody>
									</table>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}

		{/if}

		{#if tab === 'firmware'}
			<div class="space-y-4">
				<div class="flex items-center justify-end">
					<button
						onclick={checkFirmwareUpdates}
						disabled={fwChecking}
						title="Refresh Dell catalog and re-compare against installed firmware"
						class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg text-zinc-400 hover:bg-zinc-800 disabled:opacity-50"
					>
						<RefreshCw class="w-4 h-4 {fwChecking ? 'animate-spin' : ''}" />
						{fwChecking ? 'Checking...' : 'Re-check Updates'}
					</button>
				</div>

				{#if fwError}
					<div class="flex items-center gap-2 text-red-400 bg-red-500/10 rounded-xl px-4 py-3 text-sm">
						<AlertCircle class="w-4 h-4" /> {fwError}
					</div>
				{/if}

				{#if fwLoading}
					<div class="text-zinc-500 text-sm">Loading firmware inventory...</div>
				{:else}
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
						<FirmwareTable serverId={id} components={fwComponents} updates={fwUpdates} onupdate={loadFirmware} />
					</div>
				{/if}
			</div>

		{/if}

		{#if tab === 'bios'}
			<div class="space-y-4">
				{#if biosPending.length > 0}
					<div class="flex items-center gap-3 bg-amber-500/10 border border-amber-500/20 rounded-xl px-4 py-3">
						<Clock class="w-4 h-4 text-amber-400 shrink-0" />
						<div class="text-sm text-amber-300">
							<strong>{biosPending.length}</strong> pending BIOS change{biosPending.length > 1 ? 's' : ''} —
							will be applied at next reboot.
						</div>
					</div>
				{/if}

				{#if biosError}
					<div class="flex items-center gap-2 text-red-400 bg-red-500/10 rounded-xl px-4 py-3">
						<AlertCircle class="w-4 h-4" /> {biosError}
					</div>
				{:else if biosLoading}
					<div class="text-zinc-500 text-sm">Loading BIOS attributes...</div>
				{:else}
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
						<div class="flex items-center gap-3 mb-4">
							<div class="relative flex-1 max-w-sm">
								<Search class="absolute left-3 top-2.5 w-4 h-4 text-zinc-500" />
								<input
									bind:value={biosSearch}
									placeholder="Search attributes..."
									class="w-full pl-9 pr-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm
										text-zinc-200 placeholder:text-zinc-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
								/>
							</div>
							<span class="text-xs text-zinc-600">{filteredBiosEntries.length} / {Object.keys(biosAttributes).length} attributes</span>
						</div>

						<div class="overflow-x-auto">
							<table class="w-full text-sm">
								<thead>
									<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
										<th class="pb-3 pr-4">Attribute</th>
										<th class="pb-3 pr-4">Display Name</th>
										<th class="pb-3 pr-4">Value</th>
										<th class="pb-3 pr-4">Type</th>
										<th class="pb-3"></th>
									</tr>
								</thead>
								<tbody class="divide-y divide-zinc-800/50">
									{#each filteredBiosEntries as [key, val]}
										{@const entry = biosRegistry.get(key)}
										<tr class="hover:bg-zinc-800/20 {entry && !entry.ReadOnly ? 'cursor-pointer' : ''}">
											<td class="py-2.5 pr-4 font-mono text-xs text-zinc-500">{key}</td>
											<td class="py-2.5 pr-4 text-zinc-300">{entry?.DisplayName ?? key}</td>
											<td class="py-2.5 pr-4 font-mono text-xs text-zinc-200">{String(val)}</td>
											<td class="py-2.5 pr-4">
												{#if entry}
													<span class="text-xs text-zinc-600">{entry.Type}</span>
												{/if}
											</td>
											<td class="py-2.5">
												{#if entry && !entry.ReadOnly}
													<button
														onclick={() => openBiosEdit(key)}
														class="px-2.5 py-1 text-xs rounded-lg bg-zinc-800 text-zinc-400 hover:bg-zinc-700 hover:text-zinc-200"
													>
														Edit
													</button>
												{:else if entry?.ReadOnly}
													<span class="text-xs text-zinc-700">Read-only</span>
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				{/if}
			</div>

		{/if}

		{#if tab === 'virtualmedia'}
			<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-6 max-w-xl">
				<VirtualMediaPanel serverId={id} />
			</div>

		{/if}

		{#if tab === 'eventlog'}
			<div class="space-y-4">
				<div class="flex items-center justify-between">
					<span class="text-xs text-zinc-600">{logTotal} entries</span>
					<button
						onclick={clearLog}
						title="Permanently clear the iDRAC Lifecycle Controller log on this server. Cannot be undone."
						class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg text-red-400 hover:bg-red-500/10 transition-colors"
					>
						<Trash2 class="w-4 h-4" />
						Clear Log
					</button>
				</div>
				{#if logLoading}
					<div class="text-zinc-500 text-sm">Loading log...</div>
				{:else if logError}
					<div class="text-red-400 text-sm">{logError}</div>
				{:else}
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
						<table class="w-full text-sm">
							<thead>
								<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
									<th class="px-5 py-3">Time</th>
									<th class="px-5 py-3">Severity</th>
									<th class="px-5 py-3">Message</th>
									<th class="px-5 py-3">ID</th>
								</tr>
							</thead>
							<tbody class="divide-y divide-zinc-800">
								{#each logEntries as entry}
									<tr class="hover:bg-zinc-800/30">
										<td class="px-5 py-3 text-zinc-500 text-xs whitespace-nowrap">
											{new Date(entry.created).toLocaleString()}
										</td>
										<td class="px-5 py-3">
											<span class="text-xs font-medium {severityColor(entry.severity)}">{entry.severity}</span>
										</td>
										<td class="px-5 py-3 text-zinc-300 text-xs">{entry.message}</td>
										<td class="px-5 py-3 text-zinc-600 font-mono text-xs">{entry.message_id}</td>
									</tr>
								{/each}
								{#if logEntries.length === 0}
									<tr><td colspan="4" class="px-5 py-10 text-center text-zinc-600">Log is empty</td></tr>
								{/if}
							</tbody>
						</table>
					</div>
					{#if logTotal > logTop}
						<div class="flex items-center gap-3 justify-center">
							<button
								onclick={() => { logSkip = Math.max(0, logSkip - logTop); loadEventLog(); }}
								disabled={logSkip === 0}
								class="p-2 rounded-lg bg-zinc-800 text-zinc-400 hover:bg-zinc-700 disabled:opacity-30"
							>
								<ChevronLeft class="w-4 h-4" />
							</button>
							<span class="text-sm text-zinc-500">
								{logSkip + 1}–{Math.min(logSkip + logTop, logTotal)} of {logTotal}
							</span>
							<button
								onclick={() => { logSkip = logSkip + logTop; loadEventLog(); }}
								disabled={logSkip + logTop >= logTotal}
								class="p-2 rounded-lg bg-zinc-800 text-zinc-400 hover:bg-zinc-700 disabled:opacity-30"
							>
								<ChevronRight class="w-4 h-4" />
							</button>
						</div>
					{/if}
				{/if}
			</div>

		{/if}

		{#if tab === 'jobs'}
			<div class="space-y-6">
				<!-- Live iDRAC job queue (from BMC) -->
				<div>
					<div class="flex items-center justify-between mb-3">
						<h3 class="text-sm font-medium text-zinc-400">iDRAC Job Queue (live)</h3>
						<button
							onclick={loadIdracJobs}
							disabled={idracJobsLoading}
							class="flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-300 disabled:opacity-50"
						>
							<RefreshCw class="w-3 h-3 {idracJobsLoading ? 'animate-spin' : ''}" />
							Refresh
						</button>
					</div>
					{#if idracJobsLoading && !idracJobsLoaded}
						<div class="text-zinc-500 text-sm">Loading...</div>
					{:else if idracJobsError}
						<div class="text-red-400 text-sm">{idracJobsError}</div>
					{:else}
						<div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
							<table class="w-full text-sm">
								<thead>
									<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
										<th class="px-5 py-3">ID</th>
										<th class="px-5 py-3">Name</th>
										<th class="px-5 py-3">State</th>
										<th class="px-5 py-3">Progress</th>
										<th class="px-5 py-3">Started</th>
										<th class="px-5 py-3">Message</th>
									</tr>
								</thead>
								<tbody class="divide-y divide-zinc-800">
									{#each idracJobs as j}
										<tr class="hover:bg-zinc-800/30">
											<td class="px-5 py-3 font-mono text-xs text-zinc-500">{j.Id}</td>
											<td class="px-5 py-3 text-zinc-300 text-xs">{j.Name}</td>
											<td class="px-5 py-3"><StatusBadge status={j.JobState} size="sm" /></td>
											<td class="px-5 py-3 w-32">
												<div class="flex items-center gap-2">
													<div class="flex-1 bg-zinc-800 rounded-full h-1.5">
														<div class="bg-blue-500 h-1.5 rounded-full" style="width: {j.PercentComplete}%"></div>
													</div>
													<span class="text-xs text-zinc-500">{j.PercentComplete}%</span>
												</div>
											</td>
											<td class="px-5 py-3 text-zinc-500 text-xs whitespace-nowrap">{formatJobTime(j.StartTime)}</td>
											<td class="px-5 py-3 text-zinc-400 text-xs">{j.Message}</td>
										</tr>
									{/each}
									{#if idracJobs.length === 0}
										<tr><td colspan="6" class="py-8 text-center text-zinc-600">No iDRAC jobs</td></tr>
									{/if}
								</tbody>
							</table>
						</div>
					{/if}
				</div>

				<!-- Local jobs we queued (firmware updates, BIOS config) -->
				<div>
					<h3 class="text-sm font-medium text-zinc-400 mb-3">Locally Queued Jobs</h3>
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-4">
						<JobQueue jobs={localJobs} ondelete={load} />
					</div>
				</div>
			</div>
		{/if}

		<!-- Console tab — fills remaining viewport height -->
		{#if tab === 'console'}
			<div
				class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden"
				style="height: calc(100vh - 220px); min-height: 480px;"
			>
				<ConsolePanel serverId={id} />
			</div>
		{/if}

		<!-- BIOS edit modal — fullscreen overlay, lives outside the tab if-chain
		     so it can be triggered/closed without depending on the active tab. -->
		{#if biosEditing}
			<BiosAttributeEditor
				serverId={id}
				entry={biosEditing}
				onclose={() => (biosEditing = null)}
				onsave={loadBios}
			/>
		{/if}
	</div>
{/if}
