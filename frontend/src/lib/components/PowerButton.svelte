<script lang="ts">
	import type { ResetType } from '$lib/types';
	import { api } from '$lib/api';
	import { Power, RotateCcw, Zap, AlertTriangle } from '@lucide/svelte';

	type Props = { serverId: string; powerState: string; onchange?: () => void };
	let { serverId, powerState, onchange }: Props = $props();

	let loading = $state(false);
	let showConfirm = $state<ResetType | null>(null);

	const actions: { label: string; action: ResetType; destructive?: boolean; icon?: unknown }[] = [
		{ label: 'Power On', action: 'On', icon: Power },
		{ label: 'Graceful Shutdown', action: 'GracefulShutdown', icon: Power },
		{ label: 'Graceful Restart', action: 'GracefulRestart', icon: RotateCcw },
		{ label: 'Force Off', action: 'ForceOff', destructive: true, icon: Zap },
		{ label: 'Force Restart', action: 'ForceRestart', destructive: true, icon: RotateCcw },
		{ label: 'Push Power Button', action: 'PushPowerButton', icon: Power },
		{ label: 'NMI', action: 'Nmi', destructive: true, icon: AlertTriangle }
	];

	async function execute(action: ResetType) {
		loading = true;
		showConfirm = null;
		try {
			await api.power.action(serverId, action);
			onchange?.();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			loading = false;
		}
	}

	function handleClick(action: ResetType, destructive?: boolean) {
		if (destructive) {
			showConfirm = action;
		} else {
			execute(action);
		}
	}
</script>

<div class="space-y-1">
	{#each actions as { label, action, destructive, icon: Icon }}
		<button
			onclick={() => handleClick(action, destructive)}
			disabled={loading}
			class="w-full text-left px-3 py-2 rounded-lg text-sm flex items-center gap-2 transition-colors
				{destructive
				? 'text-red-400 hover:bg-red-500/10 hover:text-red-300'
				: 'text-zinc-300 hover:bg-zinc-800 hover:text-zinc-100'}
				disabled:opacity-50 disabled:cursor-not-allowed"
		>
			{#if Icon}
				<Icon class="w-4 h-4 shrink-0" />
			{/if}
			{label}
		</button>
	{/each}
</div>

{#if showConfirm}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
		<div class="bg-zinc-900 border border-zinc-700 rounded-xl p-6 w-80 shadow-2xl">
			<div class="flex items-center gap-3 mb-4">
				<AlertTriangle class="w-5 h-5 text-red-400" />
				<h3 class="font-semibold text-zinc-100">Confirm {showConfirm}</h3>
			</div>
			<p class="text-sm text-zinc-400 mb-6">
				This is a destructive action and may cause data loss or service interruption.
			</p>
			<div class="flex gap-3 justify-end">
				<button
					onclick={() => (showConfirm = null)}
					class="px-4 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
				>
					Cancel
				</button>
				<button
					onclick={() => showConfirm && execute(showConfirm)}
					class="px-4 py-2 text-sm rounded-lg bg-red-600 text-white hover:bg-red-500"
				>
					Confirm
				</button>
			</div>
		</div>
	</div>
{/if}
