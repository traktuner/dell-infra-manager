<script lang="ts">
	import type { Server, ResetType } from '$lib/types';
	import { api } from '$lib/api';
	import { CheckSquare, Square, Zap } from '@lucide/svelte';

	type Props = { servers: Server[]; onaction?: () => void };
	let { servers, onaction }: Props = $props();

	let selected = $state<Set<string>>(new Set());
	let loading = $state(false);
	let showActions = $state(false);

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

<div class="flex items-center gap-3">
	<button
		onclick={toggleAll}
		class="flex items-center gap-2 text-sm text-zinc-400 hover:text-zinc-200"
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
				class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-zinc-800 text-zinc-300
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
							onclick={() => bulkAction(action as ResetType)}
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

<!-- Export selected IDs for parent to use -->
<div class="hidden" data-selected={JSON.stringify([...selected])}></div>
