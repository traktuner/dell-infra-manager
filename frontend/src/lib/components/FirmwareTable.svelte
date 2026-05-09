<script lang="ts">
	import type { FirmwareComponent, AvailableUpdate } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { ArrowUpCircle, Check, Eye, EyeOff } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = {
		serverId: string;
		components: FirmwareComponent[];
		updates: AvailableUpdate[];
		onupdate?: () => void;
	};
	let { serverId, components, updates, onupdate }: Props = $props();

	const updateMap = $derived(new Map(updates.map((u) => [u.component, u])));
	const outdatedCount = $derived(updates.filter((u) => u.outdated).length);

	let showAll = $state(false);
	let queuing = $state<Set<string>>(new Set());

	// "Show outdated only" filter — uses the Outdated flag, not just map presence.
	const visibleComponents = $derived(
		showAll ? components : components.filter((c) => updateMap.get(c.Name)?.outdated)
	);

	async function queueUpdate(comp: string, catalogPath: string, version: string) {
		const next = new Set(queuing);
		next.add(comp);
		queuing = next;
		try {
			await api.firmware.update(serverId, comp, catalogPath, version);
			onupdate?.();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			const next2 = new Set(queuing);
			next2.delete(comp);
			queuing = next2;
		}
	}
</script>

<div class="space-y-3">
	<div class="flex items-center justify-between">
		<div class="text-xs text-zinc-500">
			{#if outdatedCount === 0}
				<span class="flex items-center gap-1.5 text-emerald-500">
					<Check class="w-3.5 h-3.5" /> All firmware up to date ({components.length} components)
				</span>
			{:else}
				<span class="text-amber-400">{outdatedCount} update{outdatedCount > 1 ? 's' : ''} available</span>
				<span class="text-zinc-600"> / {components.length} components total</span>
			{/if}
		</div>
		<button
			onclick={() => (showAll = !showAll)}
			class="flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800"
		>
			{#if showAll}
				<EyeOff class="w-3.5 h-3.5" /> Show outdated only
			{:else}
				<Eye class="w-3.5 h-3.5" /> Show all components
			{/if}
		</button>
	</div>

	{#if visibleComponents.length === 0}
		<div class="text-zinc-600 text-sm py-6 text-center">
			{outdatedCount === 0 ? 'No outdated components.' : 'No firmware data — run "Check Updates" first.'}
		</div>
	{:else}
		<div class="overflow-x-auto">
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
						<th class="pb-3 pr-4">Component</th>
						<th class="pb-3 pr-4">Installed</th>
						<th class="pb-3 pr-4">Available</th>
						<th class="pb-3 pr-4">Release</th>
						<th class="pb-3 pr-4">Status</th>
						<th class="pb-3"></th>
					</tr>
				</thead>
				<tbody class="divide-y divide-zinc-800">
					{#each visibleComponents as comp}
						{@const upd = updateMap.get(comp.Name)}
						{@const isOutdated = upd?.outdated ?? false}
						<tr class="hover:bg-zinc-900/50 {isOutdated ? 'bg-amber-500/5' : ''}">
							<td class="py-3 pr-4 text-zinc-200 font-medium">{comp.Name}</td>
							<td class="py-3 pr-4 text-zinc-400 font-mono text-xs">{comp.Version}</td>
							<td class="py-3 pr-4 font-mono text-xs">
								{#if upd}
									<span class={isOutdated ? 'text-amber-400' : 'text-zinc-400'}>{upd.available_version}</span>
								{:else}
									<span class="text-zinc-600" title="No matching entry in Dell catalog for this component">—</span>
								{/if}
							</td>
							<td class="py-3 pr-4 text-zinc-500 text-xs whitespace-nowrap">
								{upd?.release_date ?? '—'}
							</td>
							<td class="py-3 pr-4">
								{#if isOutdated}
									<StatusBadge status="Warning" size="sm" />
								{:else if upd}
									<span class="flex items-center gap-1 text-emerald-500 text-xs">
										<Check class="w-3 h-3" /> Up to date
									</span>
								{:else}
									<span class="text-xs text-zinc-600" title="Not in Dell catalog">Unknown</span>
								{/if}
							</td>
							<td class="py-3">
								{#if isOutdated && comp.Updateable && upd}
									<button
										onclick={() => queueUpdate(comp.Name, upd.catalog_path, upd.available_version)}
										disabled={queuing.has(comp.Name)}
										class="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg bg-blue-600
											text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
									>
										<ArrowUpCircle class="w-3.5 h-3.5" />
										{queuing.has(comp.Name) ? 'Queuing...' : 'Update'}
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
