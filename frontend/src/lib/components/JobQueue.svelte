<script lang="ts">
	import type { Job } from '$lib/types';
	import StatusBadge from './StatusBadge.svelte';
	import { Trash2 } from '@lucide/svelte';
	import { api } from '$lib/api';

	type Props = { jobs: Job[]; ondelete?: () => void };
	let { jobs, ondelete }: Props = $props();

	async function deleteJob(serverId: string, jobId: string) {
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

<div class="overflow-x-auto">
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
