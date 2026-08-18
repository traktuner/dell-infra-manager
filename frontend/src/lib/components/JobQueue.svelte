<script lang="ts">
	import type { Job } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { Trash2 } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = { jobs: Job[]; ondelete?: () => void };
	let { jobs, ondelete }: Props = $props();

	async function deleteJob(serverId: string, jobId: string) {
		if (!confirm('Delete this job? If it has an iDRAC job reference, the appliance also asks iDRAC to remove it. This does not reboot the server.')) return;
		try {
			await api.jobs.delete(serverId, jobId);
			ondelete?.();
		} catch (e) {
			alert((e as Error).message);
		}
	}

	function formatDate(d: string | null) {
		if (!d) return '—';
		return new Date(d).toLocaleString();
	}
</script>

<div class="hidden overflow-x-auto md:block">
	<table class="w-full text-sm">
		<thead>
			<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
				<th class="pb-3 pr-4">Type</th>
				<th class="pb-3 pr-4">Status</th>
				<th class="pb-3 pr-4">Progress</th>
				<th class="pb-3 pr-4">Created</th>
				<th class="pb-3 pr-4">Finished</th>
				<th class="pb-3"></th>
			</tr>
		</thead>
		<tbody class="divide-y divide-zinc-800">
			{#each jobs as job}
				<tr class="hover:bg-zinc-900/50">
					<td class="py-3 pr-4 text-zinc-300 font-mono text-xs">{job.type}</td>
					<td class="py-3 pr-4">
						<StatusBadge status={job.status} size="sm" />
					</td>
					<td class="py-3 pr-4 w-32">
						{#if job.status === 'running'}
							<div class="w-full bg-zinc-800 rounded-full h-1.5">
								<div class="bg-blue-500 h-1.5 rounded-full" style="width: 50%"></div>
							</div>
						{:else if job.status === 'done'}
							<div class="w-full bg-zinc-800 rounded-full h-1.5">
								<div class="bg-emerald-500 h-1.5 rounded-full w-full"></div>
							</div>
						{:else}
							<span class="text-zinc-600">—</span>
						{/if}
					</td>
					<td class="py-3 pr-4 text-zinc-500 text-xs">{formatDate(job.created_at)}</td>
					<td class="py-3 pr-4 text-zinc-500 text-xs">{formatDate(job.finished_at)}</td>
					<td class="py-3">
						{#if job.status !== 'running'}
							<button
								onclick={() => deleteJob(job.server_id, job.id)}
								class="p-1.5 text-zinc-600 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-colors"
							>
								<Trash2 class="w-3.5 h-3.5" />
							</button>
						{/if}
					</td>
				</tr>
			{/each}
			{#if jobs.length === 0}
				<tr>
					<td colspan="6" class="py-8 text-center text-zinc-600">No jobs</td>
				</tr>
			{/if}
		</tbody>
	</table>
</div>

<div class="divide-y divide-zinc-800 md:hidden">
	{#each jobs as job}
		<article class="py-4 first:pt-0 last:pb-0">
			<div class="flex items-start justify-between gap-3">
				<span class="break-all font-mono text-xs text-zinc-300">{job.type}</span>
				<StatusBadge status={job.status} size="sm" />
			</div>
			{#if job.status === 'running'}
				<div class="mt-3 h-1.5 w-full rounded-full bg-zinc-800"><div class="h-1.5 w-1/2 rounded-full bg-blue-500"></div></div>
			{:else if job.status === 'done'}
				<div class="mt-3 h-1.5 w-full rounded-full bg-emerald-500"></div>
			{/if}
			<dl class="mt-3 grid grid-cols-2 gap-3 text-xs">
				<div><dt class="text-zinc-600">Created</dt><dd class="mt-1 text-zinc-400">{formatDate(job.created_at)}</dd></div>
				<div><dt class="text-zinc-600">Finished</dt><dd class="mt-1 text-zinc-400">{formatDate(job.finished_at)}</dd></div>
			</dl>
			{#if job.status !== 'running'}
				<button onclick={() => deleteJob(job.server_id, job.id)} class="mt-3 flex min-h-11 w-full items-center justify-center gap-2 rounded-lg bg-red-500/10 text-sm text-red-400">
					<Trash2 class="h-4 w-4" /> Delete job
				</button>
			{/if}
		</article>
	{/each}
	{#if jobs.length === 0}
		<div class="py-8 text-center text-sm text-zinc-600">No jobs</div>
	{/if}
</div>
