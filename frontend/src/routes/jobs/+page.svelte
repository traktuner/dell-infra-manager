<script lang="ts">
	import type { Job, Server, WSEvent } from '$lib/types';
	import { api } from '$lib/api';
	import { wsManager } from '$lib/websocket';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onMount } from 'svelte';
	import { Trash2, RefreshCw } from '@lucide/svelte';

	let jobs = $state<Job[]>([]);
	let servers = $state<Map<string, Server>>(new Map());
	let loading = $state(true);

	// Track per-job progress from WebSocket
	let progress = $state<Map<string, { percent: number; message: string }>>(new Map());

	async function load() {
		loading = true;
		const [j, s] = await Promise.all([api.jobs.all(), api.servers.list()]);
		jobs = j;
		servers = new Map(s.map((srv) => [srv.id, srv]));
		loading = false;
	}

	onMount(() => {
		load();
		const unsub = wsManager.on('job_update', (e: WSEvent) => {
			const d = e.data as { job_id?: string; percent?: number; message?: string };
			if (d.job_id) {
				const next = new Map(progress);
				next.set(d.job_id, { percent: d.percent ?? 0, message: d.message ?? '' });
				progress = next;
			}
		});
		return unsub;
	});

	async function deleteJob(serverId: string, jobId: string) {
		await api.jobs.delete(serverId, jobId);
		await load();
	}

	function formatDate(d: string | null) {
		if (!d) return '—';
		return new Date(d).toLocaleString();
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-zinc-100">Job Queue</h1>
		<button
			onclick={load}
			class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
		>
			<RefreshCw class="w-4 h-4" />
			Refresh
		</button>
	</div>

	<div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
					<th class="px-5 py-3">Server</th>
					<th class="px-5 py-3">Type</th>
					<th class="px-5 py-3">Status</th>
					<th class="px-5 py-3">Progress</th>
					<th class="px-5 py-3">Created</th>
					<th class="px-5 py-3">Finished</th>
					<th class="px-5 py-3"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-zinc-800">
				{#each jobs as job}
					{@const server = servers.get(job.server_id)}
					{@const prog = progress.get(job.id)}
					<tr class="hover:bg-zinc-800/30">
						<td class="px-5 py-3">
							{#if server}
								<a href="/servers/{server.id}" class="text-zinc-300 hover:text-white">
									{server.name}
								</a>
							{:else}
								<span class="text-zinc-600 font-mono text-xs">{job.server_id.slice(0, 8)}</span>
							{/if}
						</td>
						<td class="px-5 py-3 text-zinc-400 font-mono text-xs">{job.type}</td>
						<td class="px-5 py-3">
							<StatusBadge status={job.status} size="sm" />
						</td>
						<td class="px-5 py-3 w-36">
							{#if job.status === 'running' && prog}
								<div class="space-y-1">
									<div class="w-full bg-zinc-800 rounded-full h-1.5">
										<div class="bg-blue-500 h-1.5 rounded-full transition-all"
											style="width: {prog.percent}%"></div>
									</div>
									<div class="text-xs text-zinc-600 truncate">{prog.message}</div>
								</div>
							{:else if job.status === 'done'}
								<div class="w-full bg-zinc-800 rounded-full h-1.5">
									<div class="bg-emerald-500 h-1.5 rounded-full w-full"></div>
								</div>
							{:else}
								<span class="text-zinc-700">—</span>
							{/if}
						</td>
						<td class="px-5 py-3 text-zinc-500 text-xs">{formatDate(job.created_at)}</td>
						<td class="px-5 py-3 text-zinc-500 text-xs">{formatDate(job.finished_at)}</td>
						<td class="px-5 py-3">
							{#if job.status !== 'running'}
								<button
									onclick={() => deleteJob(job.server_id, job.id)}
									class="p-1.5 text-zinc-600 hover:text-red-400 hover:bg-red-500/10 rounded-lg"
								>
									<Trash2 class="w-3.5 h-3.5" />
								</button>
							{/if}
						</td>
					</tr>
				{/each}
				{#if !loading && jobs.length === 0}
					<tr>
						<td colspan="7" class="px-5 py-10 text-center text-zinc-600">No jobs</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
