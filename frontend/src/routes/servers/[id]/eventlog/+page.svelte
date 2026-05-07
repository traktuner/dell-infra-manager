<script lang="ts">
	import { page } from '$app/stores';
	import type { LogEntry } from '$lib/types';
	import { api } from '$lib/api';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onMount } from 'svelte';
	import { Trash2, ChevronLeft, ChevronRight } from '@lucide/svelte';

	const id = $derived($page.params.id);

	let entries = $state<LogEntry[]>([]);
	let total = $state(0);
	let skip = $state(0);
	const top = 100;
	let loading = $state(true);

	async function load() {
		loading = true;
		try {
			const result = await api.eventlog.get(id, top, skip);
			entries = result.Members ?? [];
			total = result['Members@odata.count'] ?? entries.length;
		} catch {
			entries = [];
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function clearLog() {
		if (!confirm('Clear the entire Lifecycle Controller log?')) return;
		await api.eventlog.clear(id);
		await load();
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
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<a href="/servers/{id}" class="text-zinc-500 hover:text-zinc-300 text-sm">← Server</a>
			<h1 class="text-xl font-semibold text-zinc-100">Lifecycle Controller Log</h1>
			<span class="text-xs text-zinc-600">{total} entries</span>
		</div>
		<button
			onclick={clearLog}
			class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg text-red-400
				hover:bg-red-500/10 transition-colors"
		>
			<Trash2 class="w-4 h-4" />
			Clear Log
		</button>
	</div>

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
				{#each entries as entry}
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
				{#if !loading && entries.length === 0}
					<tr>
						<td colspan="4" class="px-5 py-10 text-center text-zinc-600">Log is empty</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>

	<!-- Pagination -->
	{#if total > top}
		<div class="flex items-center gap-3 justify-center">
			<button
				onclick={() => { skip = Math.max(0, skip - top); load(); }}
				disabled={skip === 0}
				class="p-2 rounded-lg bg-zinc-800 text-zinc-400 hover:bg-zinc-700 disabled:opacity-30"
			>
				<ChevronLeft class="w-4 h-4" />
			</button>
			<span class="text-sm text-zinc-500">
				{skip + 1}–{Math.min(skip + top, total)} of {total}
			</span>
			<button
				onclick={() => { skip = skip + top; load(); }}
				disabled={skip + top >= total}
				class="p-2 rounded-lg bg-zinc-800 text-zinc-400 hover:bg-zinc-700 disabled:opacity-30"
			>
				<ChevronRight class="w-4 h-4" />
			</button>
		</div>
	{/if}
</div>
