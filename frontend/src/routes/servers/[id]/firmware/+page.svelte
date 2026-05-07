<script lang="ts">
	import { page } from '$app/stores';
	import type { FirmwareComponent, AvailableUpdate } from '$lib/types';
	import { api } from '$lib/api';
	import FirmwareTable from '$lib/components/FirmwareTable.svelte';
	import { onMount } from 'svelte';
	import { RefreshCw, AlertCircle } from '@lucide/svelte';

	const id = $derived($page.params.id);

	let components = $state<FirmwareComponent[]>([]);
	let updates = $state<AvailableUpdate[]>([]);
	let loading = $state(true);
	let checking = $state(false);
	let error = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			components = await api.cache.firmware(id);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	async function checkUpdates() {
		checking = true;
		try {
			updates = await api.firmware.available(id);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			checking = false;
		}
	}

	onMount(load);

	const outdated = $derived(updates.length);
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="/servers/{id}" class="text-zinc-500 hover:text-zinc-300 text-sm">← Server</a>
		<h1 class="text-xl font-semibold text-zinc-100">Firmware Management</h1>
	</div>

	<div class="flex items-center gap-3">
		<button
			onclick={checkUpdates}
			disabled={checking}
			class="flex items-center gap-2 px-4 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300
				hover:bg-zinc-700 disabled:opacity-50"
		>
			<RefreshCw class="w-4 h-4 {checking ? 'animate-spin' : ''}" />
			{checking ? 'Checking...' : 'Check Updates'}
		</button>
		{#if outdated > 0}
			<span class="text-sm text-amber-400">
				{outdated} update{outdated > 1 ? 's' : ''} available
			</span>
		{/if}
	</div>

	{#if error}
		<div class="flex items-center gap-2 text-red-400 bg-red-500/10 rounded-xl px-4 py-3 text-sm">
			<AlertCircle class="w-4 h-4" /> {error}
		</div>
	{/if}

	{#if loading}
		<div class="text-zinc-500 text-sm">Loading firmware inventory...</div>
	{:else}
		<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
			<FirmwareTable serverId={id} {components} {updates} onupdate={load} />
		</div>
	{/if}
</div>
