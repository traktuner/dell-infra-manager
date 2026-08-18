<script lang="ts">
	import type { Server, ResetType } from '$lib/types';
	import { api } from '$lib/api';
	import { CheckSquare, Square, Zap, AlertTriangle } from '@lucide/svelte';

	type Props = { servers: Server[]; onaction?: () => void };
	let { servers, onaction }: Props = $props();

	let selected = $state<Set<string>>(new Set());
	let loading = $state(false);
	let showActions = $state(false);
	let pendingAction = $state<ResetType | null>(null);

	const actionDetails: Record<string, { label: string; description: string; force: boolean }> = {
		GracefulShutdown: { label: 'Graceful Shutdown', description: 'Sends an ACPI shutdown request to every selected server. Each operating system should close applications and power off cleanly.', force: false },
		GracefulRestart: { label: 'Graceful Restart', description: 'Sends an ACPI restart request to every selected server. Active services and sessions will be interrupted.', force: false },
		ForceOff: { label: 'Force Off', description: 'Cuts power immediately on every selected server. Open files, transactions, and unwritten caches can be corrupted.', force: true },
		ForceRestart: { label: 'Force Restart', description: 'Hard-resets every selected server without an operating-system shutdown. Data corruption is possible.', force: true }
	};

	const allSelected = $derived(selected.size === servers.length && servers.length > 0);

	function toggleAll() {
		if (allSelected) {
			selected = new Set();
		} else {
			selected = new Set(servers.map((s) => s.id));
		}
	}

	function toggle(id: string) {
		const next = new Set(selected);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selected = next;
	}

	async function bulkAction(action: ResetType) {
		if (selected.size === 0) return;
		loading = true;
		pendingAction = null;
		showActions = false;
		try {
			await api.power.bulk([...selected], action);
			onaction?.();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			loading = false;
		}
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	<button
		onclick={toggleAll}
		class="flex min-h-11 items-center gap-2 text-sm text-zinc-400 hover:text-zinc-200"
	>
		{#if allSelected}
			<CheckSquare class="w-4 h-4" />
		{:else}
			<Square class="w-4 h-4" />
		{/if}
		{selected.size > 0 ? `${selected.size} selected` : 'Select all'}
	</button>

	{#if selected.size > 0}
		<div class="relative">
			<button
				onclick={() => (showActions = !showActions)}
				disabled={loading}
				class="flex min-h-11 items-center gap-2 rounded-lg bg-zinc-800 px-3 py-1.5 text-zinc-300
					hover:bg-zinc-700 text-sm disabled:opacity-50"
			>
				<Zap class="w-4 h-4" />
				Power Actions
			</button>
			{#if showActions}
				<div class="absolute top-full left-0 mt-1 w-52 bg-zinc-900 border border-zinc-700
					rounded-xl shadow-2xl z-20 py-1">
					{#each ['GracefulShutdown', 'GracefulRestart', 'ForceOff', 'ForceRestart'] as action}
						<button
							onclick={() => { pendingAction = action as ResetType; showActions = false; }}
							class="w-full text-left px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-800
								{action.startsWith('Force') ? 'text-red-400' : ''}"
						>
							{action}
						</button>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>

{#if pendingAction}
	{@const detail = actionDetails[pendingAction]}
	<div role="dialog" aria-modal="true" tabindex="-1" class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm sm:p-4"
		onclick={(e) => { if (e.target === e.currentTarget) pendingAction = null; }} onkeydown={(e) => { if (e.key === 'Escape') pendingAction = null; }}>
		<div role="document" class="max-h-[calc(100dvh-1.5rem)] w-full max-w-lg overflow-y-auto rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl"
		>
			<div class="flex items-start gap-3 border-b border-zinc-800 px-4 py-5 sm:px-6">
				<AlertTriangle class="w-5 h-5 {detail.force ? 'text-red-400' : 'text-amber-400'} shrink-0 mt-0.5" />
				<div><h3 class="font-semibold text-zinc-100">{detail.label} on {selected.size} servers?</h3><p class="text-xs text-zinc-500 mt-1">One Redfish power command will be sent to each selected iDRAC.</p></div>
			</div>
			<div class="space-y-3 px-4 py-4 text-sm text-zinc-300 sm:px-6">
				<p>{detail.description}</p>
				<p class="text-zinc-500">Targets: {servers.filter((s) => selected.has(s.id)).map((s) => s.name).join(', ')}</p>
				{#if detail.force}<p class="text-red-300">This bypasses every operating-system shutdown safeguard.</p>{/if}
			</div>
			<div class="flex flex-col-reverse gap-2 rounded-b-xl border-t border-zinc-800 bg-zinc-950/40 px-4 py-3 sm:flex-row sm:justify-end sm:px-6">
				<button onclick={() => (pendingAction = null)} class="min-h-11 w-full rounded-lg bg-zinc-800 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-700 sm:w-auto">Cancel</button>
				<button onclick={() => pendingAction && bulkAction(pendingAction)} class="min-h-11 w-full rounded-lg px-4 py-2 text-sm text-white sm:w-auto {detail.force ? 'bg-red-600 hover:bg-red-500' : 'bg-amber-600 hover:bg-amber-500'}">{detail.label}</button>
			</div>
		</div>
	</div>
{/if}

<!-- Export selected IDs for parent to use -->
<div class="hidden" data-selected={JSON.stringify([...selected])}></div>
