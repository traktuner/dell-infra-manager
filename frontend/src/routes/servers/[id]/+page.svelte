<script lang="ts">
	import { page } from '$app/state';
	import type { Server, ServerCache, SystemInfo, ThermalInfo, PowerInfo, WSEvent } from '$lib/types';
	import { api } from '$lib/api';
	import { wsManager } from '$lib/websocket';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import PowerButton from '$lib/components/PowerButton.svelte';
	import JobQueue from '$lib/components/JobQueue.svelte';
	import { onMount } from 'svelte';
	import { Thermometer, Zap, HardDrive, FileText, Cpu, Wind } from '@lucide/svelte';

	const id = $derived(page.params.id);

	let server = $state<Server | null>(null);
	let cache = $state<ServerCache | null>(null);
	let jobs = $state<ReturnType<typeof api.jobs.forServer> extends Promise<infer T> ? T : never>([]);
	let tab = $state<'overview' | 'storage' | 'eventlog' | 'jobs'>('overview');

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
		if (j.status === 'fulfilled') jobs = j.value;
	}

	onMount(() => {
		load();
		const unsub = wsManager.on('thermal_update', (e: WSEvent) => {
			if (e.server_id === id) load();
		});
		const unsub2 = wsManager.on('job_update', (e: WSEvent) => {
			if (e.server_id === id) api.jobs.forServer(id).then((j) => (jobs = j));
		});
		return () => { unsub(); unsub2(); };
	});

	const tabs = [
		{ key: 'overview', label: 'Overview', icon: Cpu },
		{ key: 'storage', label: 'Storage', icon: HardDrive },
		{ key: 'eventlog', label: 'Event Log', icon: FileText },
		{ key: 'jobs', label: 'iDRAC Jobs', icon: Zap }
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
			<div class="flex items-center gap-3">
				<StatusBadge status={cache?.status ?? 'unknown'} />
				{#if system}
					<StatusBadge status={system.PowerState} />
					<StatusBadge status={system.Status.Health} />
				{/if}
			</div>
		</div>

		<div class="flex gap-1 border-b border-zinc-800">
			{#each tabs as { key, label, icon: Icon }}
				<button
					onclick={() => (tab = key as typeof tab)}
					class="flex items-center gap-2 px-4 py-2.5 text-sm border-b-2 transition-colors
						{tab === key
						? 'border-blue-500 text-blue-400'
						: 'border-transparent text-zinc-500 hover:text-zinc-300'}"
				>
					<Icon class="w-4 h-4" />
					{label}
				</button>
			{/each}
			<div class="ml-auto flex gap-1 pb-1">
				<a href="/servers/{id}/firmware"
					class="flex items-center gap-1.5 px-3 py-1.5 text-sm text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded-lg">
					Firmware
				</a>
				<a href="/servers/{id}/bios"
					class="flex items-center gap-1.5 px-3 py-1.5 text-sm text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded-lg">
					BIOS
				</a>
				<a href="/servers/{id}/virtualmedia"
					class="flex items-center gap-1.5 px-3 py-1.5 text-sm text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800 rounded-lg">
					Virtual Media
				</a>
			</div>
		</div>

		<!-- Tab content -->
		{#if tab === 'overview'}
			<div class="grid grid-cols-3 gap-6">
				<!-- Left: System info + Power -->
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

				<!-- Right: Power Actions -->
				<div>
					<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
						<h3 class="text-sm font-medium text-zinc-400 mb-3">Power Actions</h3>
						<PowerButton serverId={id} powerState={system?.PowerState ?? ''} onchange={load} />
					</div>
				</div>
			</div>

		{:else if tab === 'storage'}
			<a href="/servers/{id}/storage" class="text-blue-400 hover:underline text-sm">
				→ Open Storage page
			</a>

		{:else if tab === 'eventlog'}
			<a href="/servers/{id}/eventlog" class="text-blue-400 hover:underline text-sm">
				→ Open Event Log page
			</a>

		{:else if tab === 'jobs'}
			<JobQueue {jobs} ondelete={load} />
		{/if}
	</div>
{/if}
