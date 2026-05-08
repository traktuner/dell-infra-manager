<script lang="ts">
	import type { Server, ServerCache, SystemInfo, PowerInfo, ThermalInfo, TempSensor } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { Cpu, Thermometer, Zap, Server as ServerIcon, MemoryStick, RefreshCw } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = { server: Server; cache: ServerCache | null; onrefresh?: () => void };
	let { server, cache, onrefresh }: Props = $props();

	let refreshing = $state(false);

	async function refreshData(e: MouseEvent) {
		e.preventDefault(); // don't navigate to /servers/:id
		if (refreshing) return;
		refreshing = true;
		try {
			await Promise.all([
				api.cache.summary(server.id),
				api.cache.thermal(server.id),
				api.cache.power(server.id),
			]);
		} catch {
			// ignore — stale data is fine
		} finally {
			refreshing = false;
		}
		onrefresh?.();
	}

	const system = $derived<SystemInfo | null>(
		cache?.system_json ? JSON.parse(cache.system_json) : null
	);
	const power = $derived<PowerInfo | null>(
		cache?.power_json ? JSON.parse(cache.power_json) : null
	);
	const thermal = $derived<ThermalInfo | null>(
		cache?.thermal_json ? JSON.parse(cache.thermal_json) : null
	);

	// iDRAC sensor names vary across firmware versions / chassis: "Inlet Temp",
	// "System Inlet Temp", "Inlet Temperature", "Ambient Temp", etc. Match
	// exact first, then case-insensitive substrings.
	function findInletTemp(temps: TempSensor[] | undefined): number | null {
		if (!temps?.length) return null;
		const exact = temps.find((t) => t.Name === 'Inlet Temp');
		if (exact) return exact.ReadingCelsius;
		const inlet = temps.find((t) => t.Name?.toLowerCase().includes('inlet'));
		if (inlet) return inlet.ReadingCelsius;
		const ambient = temps.find((t) => t.Name?.toLowerCase().includes('ambient'));
		if (ambient) return ambient.ReadingCelsius;
		return null;
	}

	const inletTemp = $derived(findInletTemp(thermal?.Temperatures));
	const powerWatts = $derived(power?.PowerControl?.[0]?.PowerConsumedWatts ?? null);

	// Compact CPU summary: "2× Intel Xeon Gold 6248R · 48C/96T"
	// Falls back to just the count if we don't have the model yet.
	const cpuSummary = $derived.by(() => {
		if (!system) return null;
		const count = system.ProcessorSummary.Count ?? 0;
		const model = (system.ProcessorSummary.Model ?? '')
			.replace(/\(R\)|\(TM\)/g, '')
			.replace(/CPU\s*@.*$/, '')
			.trim();
		const cores = (system.CoresPerCPU ?? 0) * count;
		const threads =
			(system.ThreadsPerCPU ?? 0) * count || system.ProcessorSummary.LogicalProcessorCount || 0;
		const head = count > 1 ? `${count}× ${model}` : model;
		const detail = cores && threads ? `${cores}C / ${threads}T` : '';
		return { head: head || `${count} CPU`, detail };
	});

	// Compact RAM summary: "256 GB DDR4 2933 MHz"
	const ramSummary = $derived.by(() => {
		if (!system) return null;
		const gb = system.MemorySummary.TotalSystemMemoryGiB ?? 0;
		const type = system.MemoryType ?? '';
		const speed = system.MemorySpeedMHz ?? 0;
		const detail = [type, speed ? `${speed} MHz` : ''].filter(Boolean).join(' ');
		return { head: `${Math.round(gb)} GB`, detail };
	});

	const lastSeen = $derived(
		cache?.last_seen ? new Date(cache.last_seen).toLocaleTimeString() : null
	);

</script>

<a
	href="/servers/{server.id}"
	class="block bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-600 transition-colors group"
>
	<div class="flex items-start justify-between mb-3">
		<div class="flex items-center gap-3 min-w-0">
			<div class="w-9 h-9 rounded-lg bg-zinc-800 flex items-center justify-center shrink-0">
				<ServerIcon class="w-5 h-5 text-zinc-400" />
			</div>
			<div class="min-w-0">
				<div class="font-semibold text-zinc-100 group-hover:text-white truncate">{server.name}</div>
				<div class="text-xs text-zinc-500 truncate">{server.hostname}</div>
			</div>
		</div>
		<StatusBadge status={cache?.status ?? 'unknown'} />
	</div>

	{#if system}
		<div class="text-xs text-zinc-500 mb-4 truncate">
			{system.Model} · {system.SerialNumber}
		</div>
	{/if}

	<!-- Power + Inlet temp row -->
	<div class="grid grid-cols-2 gap-3 mb-3">
		<div class="flex items-center gap-2 text-sm">
			<Zap class="w-4 h-4 text-amber-400/70 shrink-0" />
			<span class="text-zinc-200 font-medium tabular-nums">
				{powerWatts != null ? `${Math.round(powerWatts)} W` : '—'}
			</span>
		</div>
		<div class="flex items-center gap-2 text-sm">
			<Thermometer class="w-4 h-4 text-sky-400/70 shrink-0" />
			<span class="text-zinc-200 font-medium tabular-nums">
				{inletTemp != null ? `${inletTemp}°C` : '—'}
			</span>
		</div>
	</div>

	<!-- CPU + RAM rows (more detail than the previous "2C / 256G" mash-up) -->
	<div class="space-y-1.5 mb-4">
		<div class="flex items-start gap-2 text-xs">
			<Cpu class="w-3.5 h-3.5 text-zinc-500 shrink-0 mt-0.5" />
			<div class="min-w-0">
				<div class="text-zinc-300 truncate">{cpuSummary?.head ?? '—'}</div>
				{#if cpuSummary?.detail}
					<div class="text-zinc-500">{cpuSummary.detail}</div>
				{/if}
			</div>
		</div>
		<div class="flex items-start gap-2 text-xs">
			<MemoryStick class="w-3.5 h-3.5 text-zinc-500 shrink-0 mt-0.5" />
			<div class="min-w-0">
				<div class="text-zinc-300 truncate">{ramSummary?.head ?? '—'}</div>
				{#if ramSummary?.detail}
					<div class="text-zinc-500">{ramSummary.detail}</div>
				{/if}
			</div>
		</div>
	</div>

	<div class="flex items-center justify-between">
		<div class="flex items-center gap-2">
			<StatusBadge status={system?.PowerState ?? 'unknown'} size="sm" />
			{#if system?.Status.Health}
				<StatusBadge status={system.Status.Health} size="sm" />
			{/if}
		</div>
		<button
			onclick={refreshData}
			disabled={refreshing}
			title="Kartendaten aktualisieren"
			class="p-1.5 rounded-lg text-zinc-600 hover:text-zinc-300 hover:bg-zinc-800
				disabled:opacity-30 transition-colors"
		>
			<RefreshCw class="w-3.5 h-3.5 {refreshing ? 'animate-spin' : ''}" />
		</button>
	</div>

	{#if cache?.status === 'offline' && lastSeen}
		<div class="mt-2 text-xs text-zinc-600">Last seen {lastSeen}</div>
	{/if}
</a>
