<script lang="ts">
	import type { FirmwareComponent, AvailableUpdate } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { ArrowUpCircle, Check } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = {
		serverId: string;
		components: FirmwareComponent[];
		updates: AvailableUpdate[];
		onupdate?: () => void;
	};
	let { serverId, components, updates, onupdate }: Props = $props();

	const updateMap = $derived(
		new Map(updates.map((u) => [u.component, u]))
	);

	let queuing = $state<Set<string>>(new Set());

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

<div class="overflow-x-auto">
	<table class="w-full text-sm">
		<thead>
			<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
				<th class="pb-3 pr-4">Component</th>
				<th class="pb-3 pr-4">Installed</th>
				<th class="pb-3 pr-4">Available</th>
				<th class="pb-3 pr-4">Status</th>
				<th class="pb-3"></th>
			</tr>
		</thead>
		<tbody class="divide-y divide-zinc-800">
			{#each components as comp}
				{@const upd = updateMap.get(comp.Name)}
				<tr class="hover:bg-zinc-900/50">
					<td class="py-3 pr-4 text-zinc-200 font-medium">{comp.Name}</td>
					<td class="py-3 pr-4 text-zinc-400 font-mono text-xs">{comp.Version}</td>
					<td class="py-3 pr-4 text-zinc-400 font-mono text-xs">
						{upd ? upd.available_version : '—'}
					</td>
					<td class="py-3 pr-4">
						{#if upd}
							<StatusBadge status="Warning" size="sm" />
						{:else}
							<span class="flex items-center gap-1 text-emerald-500 text-xs">
								<Check class="w-3 h-3" /> Up to date
							</span>
						{/if}
					</td>
					<td class="py-3">
						{#if upd && comp.Updateable}
							<button
								onclick={() => queueUpdate(comp.Name, upd.catalog_path, upd.available_version)}
								disabled={queuing.has(comp.Name)}
								class="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg bg-blue-600
									text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
							>
								<ArrowUpCircle class="w-3.5 h-3.5" />
								{queuing.has(comp.Name) ? 'Queuing...' : 'Queue Update'}
							</button>
						{/if}
					</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
