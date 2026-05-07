<script lang="ts">
	import type { Server, ServerCache, SystemInfo, PowerInfo, ThermalInfo } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { Cpu, Thermometer, Zap, RotateCcw, Server as ServerIcon } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = { server: Server; cache: ServerCache | null };
	let { server, cache }: Props = $props();

	const system = $derived<SystemInfo | null>(
		cache?.system_json ? JSON.parse(cache.system_json) : null
	);
	const power = $derived<PowerInfo | null>(
		cache?.power_json ? JSON.parse(cache.power_json) : null
	);
	const thermal = $derived<ThermalInfo | null>(
		cache?.thermal_json ? JSON.parse(cache.thermal_json) : null
	);

	const inletTemp = $derived(
		thermal?.Temperatures.find((t) => t.Name === 'Inlet Temp')?.ReadingCelsius ?? null
	);
	const powerWatts = $derived(power?.PowerControl?.[0]?.PowerConsumedWatts ?? null);

	const lastSeen = $derived(
		cache?.last_seen ? new Date(cache.last_seen).toLocaleTimeString() : null
	);

	let rebooting = $state(false);
	async function quickReboot() {
		rebooting = true;
		try {
			await api.power.action(server.id, 'GracefulRestart');
		} catch (e) {
			alert((e as Error).message);
		} finally {
			rebooting = false;
		}
	}
</script>

<a
	href="/servers/{server.id}"
	class="block bg-zinc-900 border border-zinc-800 rounded-xl p-5 hover:border-zinc-600 transition-colors group"
>
	<div class="flex items-start justify-between mb-3">
		<div class="flex items-center gap-3">
			<div class="w-9 h-9 rounded-lg bg-zinc-800 flex items-center justify-center">
				<ServerIcon class="w-5 h-5 text-zinc-400" />
			</div>
			<div>
				<div class="font-semibold text-zinc-100 group-hover:text-white">{server.name}</div>
				<div class="text-xs text-zinc-500">{server.hostname}</div>
			</div>
		</div>
		<StatusBadge status={cache?.status ?? 'unknown'} />
	</div>

	{#if system}
		<div class="text-xs text-zinc-500 mb-3">
			{system.Model} · {system.SerialNumber}
		</div>
	{/if}

	<div class="grid grid-cols-3 gap-3 mb-4">
		<div class="flex items-center gap-1.5 text-sm">
			<Zap class="w-3.5 h-3.5 text-zinc-500" />
			<span class="text-zinc-300">
				{powerWatts != null ? `${Math.round(powerWatts)}W` : '—'}
			</span>
		</div>
		<div class="flex items-center gap-1.5 text-sm">
			<Thermometer class="w-3.5 h-3.5 text-zinc-500" />
			<span class="text-zinc-300">
				{inletTemp != null ? `${inletTemp}°C` : '—'}
			</span>
		</div>
		<div class="flex items-center gap-1.5 text-sm">
			<Cpu class="w-3.5 h-3.5 text-zinc-500" />
			<span class="text-zinc-300">
				{system ? `${system.ProcessorSummary.Count}C / ${system.MemorySummary.TotalSystemMemoryGiB}G` : '—'}
			</span>
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
			onclick={(e) => { e.preventDefault(); quickReboot(); }}
			disabled={rebooting || cache?.status !== 'online'}
			class="p-1.5 rounded-lg text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800
				disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
			title="Graceful Restart"
		>
			<RotateCcw class="w-3.5 h-3.5 {rebooting ? 'animate-spin' : ''}" />
		</button>
	</div>

	{#if cache?.status === 'offline' && lastSeen}
		<div class="mt-2 text-xs text-zinc-600">Last seen {lastSeen}</div>
	{/if}
</a>
