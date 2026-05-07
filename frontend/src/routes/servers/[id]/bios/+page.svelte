<script lang="ts">
	import { page } from '$app/stores';
	import type { BiosRegistryEntry, Job } from '$lib/types';
	import { api } from '$lib/api';
	import BiosAttributeEditor from '$lib/components/BiosAttributeEditor.svelte';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onMount } from 'svelte';
	import { Search, AlertCircle, Clock } from '@lucide/svelte';

	const id = $derived($page.params.id);

	let attributes = $state<Record<string, unknown>>({});
	let registryMap = $state<Map<string, BiosRegistryEntry>>(new Map());
	let pendingJobs = $state<Job[]>([]);
	let loading = $state(true);
	let error = $state('');
	let search = $state('');
	let editing = $state<BiosRegistryEntry | null>(null);

	async function load() {
		loading = true;
		error = '';
		try {
			const [bios, registry, pending] = await Promise.all([
				api.bios.get(id),
				api.bios.registry(id),
				api.bios.pending(id)
			]);
			attributes = bios.Attributes;
			pendingJobs = pending;

			const map = new Map<string, BiosRegistryEntry>();
			for (const entry of registry.RegistryEntries ?? []) {
				map.set(entry.AttributeName, {
					...entry,
					current_value: bios.Attributes[entry.AttributeName],
					allowed_values: entry.Value?.map((v) => v.ValueName) ?? []
				} as BiosRegistryEntry);
			}
			registryMap = map;
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	onMount(load);

	const filtered = $derived(
		Object.entries(attributes).filter(
			([k]) =>
				!search ||
				k.toLowerCase().includes(search.toLowerCase()) ||
				registryMap.get(k)?.DisplayName.toLowerCase().includes(search.toLowerCase())
		)
	);

	function openEdit(attrName: string) {
		const entry = registryMap.get(attrName);
		if (!entry || entry.ReadOnly) return;
		editing = { ...entry, current_value: attributes[attrName] };
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="/servers/{id}" class="text-zinc-500 hover:text-zinc-300 text-sm">← Server</a>
		<h1 class="text-xl font-semibold text-zinc-100">BIOS Configuration</h1>
	</div>

	{#if pendingJobs.length > 0}
		<div class="flex items-center gap-3 bg-amber-500/10 border border-amber-500/20 rounded-xl px-4 py-3">
			<Clock class="w-4 h-4 text-amber-400 shrink-0" />
			<div class="text-sm text-amber-300">
				<strong>{pendingJobs.length}</strong> pending BIOS change{pendingJobs.length > 1 ? 's' : ''} —
				will be applied at next reboot.
			</div>
		</div>
	{/if}

	{#if error}
		<div class="flex items-center gap-2 text-red-400 bg-red-500/10 rounded-xl px-4 py-3">
			<AlertCircle class="w-4 h-4" /> {error}
		</div>
	{:else if loading}
		<div class="text-zinc-500 text-sm">Loading BIOS attributes...</div>
	{:else}
		<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-5">
			<div class="flex items-center gap-3 mb-4">
				<div class="relative flex-1 max-w-sm">
					<Search class="absolute left-3 top-2.5 w-4 h-4 text-zinc-500" />
					<input
						bind:value={search}
						placeholder="Search attributes..."
						class="w-full pl-9 pr-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm
							text-zinc-200 placeholder:text-zinc-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
					/>
				</div>
				<span class="text-xs text-zinc-600">{filtered.length} / {Object.keys(attributes).length} attributes</span>
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
						{#each filtered as [key, val]}
							{@const entry = registryMap.get(key)}
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
											onclick={() => openEdit(key)}
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

{#if editing}
	<BiosAttributeEditor
		serverId={id}
		entry={editing}
		onclose={() => (editing = null)}
		onsave={load}
	/>
{/if}
