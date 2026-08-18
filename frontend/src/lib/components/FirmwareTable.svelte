<script lang="ts">
	import type { FirmwareComponent, AvailableUpdate } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { ArrowUpCircle, Check, Eye, EyeOff, AlertTriangle, HelpCircle } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = {
		serverId: string;
		components: FirmwareComponent[];
		updates: AvailableUpdate[];
		checked?: boolean;
		onupdate?: () => void;
	};
	let { serverId, components, updates, checked = false, onupdate }: Props = $props();

	const updateMap = $derived(new Map(updates.map((u) => [u.inventory_id, u])));
	const outdatedCount = $derived(updates.filter((u) => u.outdated).length);
	const unknownCount = $derived(updates.filter((u) => u.comparison_status === 'unknown').length);

	let showAll = $state(false);
	let queuing = $state<Set<string>>(new Set());
	let pendingUpdate = $state<AvailableUpdate | null>(null);

	function comparisonFor(component: FirmwareComponent): AvailableUpdate | undefined {
		return updateMap.get(component.Id) ?? updates.find((u) =>
			u.software_id === component.SoftwareId &&
			u.component === component.Name &&
			u.installed_version === component.Version
		);
	}

	// "Show outdated only" filter — uses the Outdated flag, not just map presence.
	const visibleComponents = $derived(
		!checked || showAll ? components : components.filter((c) => {
			const status = comparisonFor(c)?.comparison_status;
			return status === 'outdated' || status === 'unknown';
		})
	);

	async function queueUpdate(update: AvailableUpdate) {
		const key = update.inventory_id;
		const next = new Set(queuing);
		next.add(key);
		queuing = next;
		pendingUpdate = null;
		try {
			await api.firmware.update(serverId, update);
			onupdate?.();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			const next2 = new Set(queuing);
			next2.delete(key);
			queuing = next2;
		}
	}
</script>

<div class="space-y-3">
	<div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
		<div class="text-xs text-zinc-500">
			{#if !checked}
				<span>Firmware has not been compared with the Dell catalog.</span>
			{:else if outdatedCount === 0 && unknownCount === 0}
					<span class="flex items-center gap-1.5 text-emerald-500">
						<Check class="w-3.5 h-3.5" /> All firmware up to date ({components.length} components)
					</span>
				{:else}
					{#if outdatedCount > 0}<span class="text-amber-400">{outdatedCount} update{outdatedCount > 1 ? 's' : ''} available</span>{/if}
					{#if outdatedCount > 0 && unknownCount > 0}<span class="text-zinc-600"> · </span>{/if}
					{#if unknownCount > 0}<span class="text-zinc-400">{unknownCount} unknown</span>{/if}
					<span class="text-zinc-600"> / {components.length} components total</span>
			{/if}
		</div>
		<button
			onclick={() => (showAll = !showAll)}
			disabled={!checked}
			class="flex min-h-11 items-center justify-center gap-1.5 rounded-lg px-2.5 py-1 text-xs text-zinc-500 hover:bg-zinc-800 hover:text-zinc-300 disabled:opacity-30 sm:min-h-0"
		>
			{#if showAll}
				<EyeOff class="w-3.5 h-3.5" /> Show outdated only
			{:else}
				<Eye class="w-3.5 h-3.5" /> Show all components
			{/if}
		</button>
	</div>

	{#if checked && visibleComponents.length === 0}
		<div class="text-zinc-600 text-sm py-6 text-center">
				No outdated or unknown components.
		</div>
	{:else}
		<div class="hidden overflow-x-auto md:block">
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
							{@const upd = comparisonFor(comp)}
						{@const isOutdated = upd?.outdated ?? false}
						<tr class="hover:bg-zinc-900/50 {isOutdated ? 'bg-amber-500/5' : ''}">
							<td class="py-3 pr-4 text-zinc-200 font-medium">{comp.Name}</td>
							<td class="py-3 pr-4 text-zinc-400 font-mono text-xs">{comp.Version}</td>
							<td class="py-3 pr-4 font-mono text-xs">
									{#if upd?.matched}
									<span class={isOutdated ? 'text-amber-400' : 'text-zinc-400'}>{upd.available_version}</span>
									{:else}
										<span class="text-zinc-600" title={upd?.reason ?? 'No comparison result'}>—</span>
								{/if}
							</td>
							<td class="py-3 pr-4 text-zinc-500 text-xs whitespace-nowrap">
								{upd?.release_date ?? '—'}
							</td>
							<td class="py-3 pr-4">
								{#if isOutdated}
									<StatusBadge status="Warning" size="sm" />
									{:else if upd?.comparison_status === 'current'}
										<span class="flex items-center gap-1 text-emerald-500 text-xs">
											<Check class="w-3 h-3" /> Up to date
										</span>
									{:else if upd?.comparison_status === 'newer'}
										<span class="text-xs text-blue-400" title={upd.reason}>Newer than catalog</span>
									{:else}
										<span class="flex items-center gap-1 text-xs text-zinc-500" title={upd?.reason ?? 'Not compared'}>
											<HelpCircle class="w-3 h-3" /> Unknown
										</span>
								{/if}
							</td>
							<td class="py-3">
								{#if isOutdated && comp.Updateable && upd}
									<button
										onclick={() => (pendingUpdate = upd)}
										disabled={queuing.has(upd.inventory_id)}
										class="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg bg-blue-600
											text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
									>
										<ArrowUpCircle class="w-3.5 h-3.5" />
										{queuing.has(upd.inventory_id) ? 'Queuing...' : 'Update'}
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>

		<div class="divide-y divide-zinc-800 md:hidden">
			{#each visibleComponents as comp}
				{@const upd = comparisonFor(comp)}
				{@const isOutdated = upd?.outdated ?? false}
				<article class="py-4 first:pt-1 {isOutdated ? 'bg-amber-500/5' : ''}">
					<div class="flex items-start justify-between gap-3">
						<h4 class="min-w-0 break-words text-sm font-medium text-zinc-200">{comp.Name}</h4>
						<div class="shrink-0">
							{#if isOutdated}
								<StatusBadge status="Warning" size="sm" />
							{:else if upd?.comparison_status === 'current'}
								<span class="flex items-center gap-1 text-xs text-emerald-500"><Check class="h-3 w-3" /> Up to date</span>
							{:else if upd?.comparison_status === 'newer'}
								<span class="text-xs text-blue-400" title={upd.reason}>Newer than catalog</span>
							{:else}
								<span class="flex items-center gap-1 text-xs text-zinc-500" title={upd?.reason ?? 'Not compared'}><HelpCircle class="h-3 w-3" /> Unknown</span>
							{/if}
						</div>
					</div>
					<dl class="mt-3 grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
						<div><dt class="text-zinc-600">Installed</dt><dd class="mt-0.5 break-all font-mono text-zinc-400">{comp.Version}</dd></div>
						<div><dt class="text-zinc-600">Available</dt><dd class="mt-0.5 break-all font-mono {isOutdated ? 'text-amber-400' : 'text-zinc-400'}">{upd?.matched ? upd.available_version : '—'}</dd></div>
						<div class="col-span-2"><dt class="text-zinc-600">Release</dt><dd class="mt-0.5 text-zinc-500">{upd?.release_date ?? '—'}</dd></div>
					</dl>
					{#if isOutdated && comp.Updateable && upd}
						<button onclick={() => (pendingUpdate = upd)} disabled={queuing.has(upd.inventory_id)} class="mt-4 flex min-h-11 w-full items-center justify-center gap-2 rounded-lg bg-blue-600 px-3 text-sm text-white disabled:opacity-50">
							<ArrowUpCircle class="h-4 w-4" /> {queuing.has(upd.inventory_id) ? 'Queuing...' : 'Update'}
						</button>
					{/if}
				</article>
			{/each}
		</div>
	{/if}
</div>

{#if pendingUpdate}
	<div role="dialog" aria-modal="true" tabindex="-1"
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm sm:p-4"
		onclick={(e) => { if (e.target === e.currentTarget) pendingUpdate = null; }}
		onkeydown={(e) => { if (e.key === 'Escape') pendingUpdate = null; }}>
		<div role="document" class="max-h-[calc(100dvh-1.5rem)] w-full max-w-lg overflow-y-auto rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl"
		>
			<div class="flex items-start gap-3 border-b border-zinc-800 px-4 py-5 sm:px-6">
				<AlertTriangle class="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
				<div><h3 class="font-semibold text-zinc-100">Stage firmware update?</h3>
					<p class="text-xs text-zinc-500 mt-1">{pendingUpdate.component} · {pendingUpdate.installed_version} → {pendingUpdate.available_version}</p></div>
			</div>
			<div class="space-y-3 px-4 py-4 text-sm text-zinc-300 sm:px-6">
				<p>The appliance downloads the Dell package and uploads it to this iDRAC.</p>
				<p class="text-amber-300">Apply time is fixed to OnReset. This action does not restart, power off, or reboot the managed server.</p>
				<p class="text-zinc-500">The staged firmware applies only when you later reset the applicable system or service during your maintenance window.</p>
			</div>
			<div class="flex flex-col-reverse gap-2 rounded-b-xl border-t border-zinc-800 bg-zinc-950/40 px-4 py-3 sm:flex-row sm:justify-end sm:px-6">
				<button onclick={() => (pendingUpdate = null)} class="min-h-11 w-full rounded-lg bg-zinc-800 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-700 sm:w-auto">Cancel</button>
				<button onclick={() => pendingUpdate && queueUpdate(pendingUpdate)} class="min-h-11 w-full rounded-lg bg-amber-600 px-4 py-2 text-sm text-white hover:bg-amber-500 sm:w-auto">Stage with OnReset</button>
			</div>
		</div>
	</div>
{/if}
