<script lang="ts">
	import { page } from '$app/state';
	import type { StorageDetail } from '$lib/types';
	import { api } from '$lib/api';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onMount } from 'svelte';
	import { HardDrive, Database } from '@lucide/svelte';

	const id = $derived(page.params.id);

	let storage = $state<StorageDetail[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			storage = await api.cache.storage(id);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	});

	function formatBytes(bytes: number) {
		const gb = bytes / 1e9;
		return gb >= 1 ? `${gb.toFixed(1)} GB` : `${(bytes / 1e6).toFixed(0)} MB`;
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="/servers/{id}" class="text-zinc-500 hover:text-zinc-300 text-sm">← Server</a>
		<h1 class="text-xl font-semibold text-zinc-100">Storage</h1>
	</div>

	{#if loading}
		<div class="text-zinc-500 text-sm">Loading storage...</div>
	{:else if error}
		<div class="text-red-400 text-sm">{error}</div>
	{:else}
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
									<tr class="{drive.FailurePredicted ? 'bg-red-500/5' : ''}">
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
	{/if}
</div>
