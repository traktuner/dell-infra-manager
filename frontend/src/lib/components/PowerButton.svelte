<script lang="ts">
	import type { ResetType } from '$lib/types';
	import { api } from '$lib/api';
	import {
		Power, PowerOff, RotateCcw, ZapOff, RefreshCw,
		MousePointerClick, TriangleAlert, AlertTriangle
	} from '@lucide/svelte';

	type Props = { serverId: string; powerState: string; onchange?: () => void };
	let { serverId, powerState, onchange }: Props = $props();

	let loading = $state(false);
	let pendingAction = $state<ResetType | null>(null);

	type Severity = 'safe' | 'force' | 'danger';

	type Action = {
		label: string;
		desc: string;        // tooltip — what it actually does
		action: ResetType;
		severity: Severity;
		icon: any;
		needsOn?: boolean;   // disabled when server is Off
		needsOff?: boolean;  // disabled when server is On
	};

	// Three groups by severity:
	//   safe   — graceful, no data loss
	//   force  — abrupt but normal admin action
	//   danger — system-level interrupts; can crash OS / cause data loss
	const safe: Action[] = [
		{ label: 'Power On',         desc: 'Turn the server on.',
		  action: 'On', severity: 'safe', icon: Power, needsOff: true },
		{ label: 'Graceful Shutdown', desc: 'Ask the OS to shut down cleanly via ACPI.',
		  action: 'GracefulShutdown', severity: 'safe', icon: PowerOff, needsOn: true },
		{ label: 'Graceful Restart',  desc: 'Ask the OS to reboot cleanly via ACPI.',
		  action: 'GracefulRestart', severity: 'safe', icon: RotateCcw, needsOn: true },
	];
	const force: Action[] = [
		{ label: 'Force Off',     desc: 'Cut power immediately. Same as holding the power button — OS does not get to flush data.',
		  action: 'ForceOff', severity: 'force', icon: ZapOff, needsOn: true },
		{ label: 'Force Restart', desc: 'Hard reset the CPU. OS does not shut down cleanly.',
		  action: 'ForceRestart', severity: 'force', icon: RefreshCw, needsOn: true },
		{ label: 'Push Power Button', desc: 'Simulate a single press of the chassis power button. Behaviour depends on OS settings.',
		  action: 'PushPowerButton', severity: 'force', icon: MousePointerClick },
	];
	const danger: Action[] = [
		{ label: 'NMI', desc: 'Non-Maskable Interrupt. Forces a kernel crash dump on Linux/Windows for hang debugging. Almost certainly causes data loss.',
		  action: 'Nmi', severity: 'danger', icon: TriangleAlert, needsOn: true },
	];

	function isDisabled(a: Action): boolean {
		if (loading) return true;
		if (a.needsOn  && powerState !== 'On')  return true;
		if (a.needsOff && powerState === 'On')  return true;
		return false;
	}

	async function execute(action: ResetType) {
		loading = true;
		pendingAction = null;
		try {
			await api.power.action(serverId, action);
			onchange?.();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			loading = false;
		}
	}

	function handleClick(a: Action) {
		if (a.severity === 'safe') {
			execute(a.action);
		} else {
			pendingAction = a.action;
		}
	}

	const pendingActionDef = $derived(
		[...safe, ...force, ...danger].find((a) => a.action === pendingAction)
	);
</script>

<div class="space-y-4">
	<!-- Safe actions: graceful, no confirmation needed -->
	<div class="space-y-1">
		<div class="text-[10px] uppercase tracking-wider text-zinc-600 px-1">Graceful</div>
		{#each safe as a}
			<button
				onclick={() => handleClick(a)}
				disabled={isDisabled(a)}
				title={a.desc}
				class="w-full text-left px-3 py-2 rounded-lg text-sm flex items-center gap-2.5 transition-colors
					text-zinc-300 hover:bg-zinc-800 hover:text-zinc-100
					disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent"
			>
				<a.icon class="w-4 h-4 shrink-0 text-zinc-400" />
				{a.label}
			</button>
		{/each}
	</div>

	<!-- Force actions: abrupt, confirmation required -->
	<div class="space-y-1 pt-3 border-t border-zinc-800">
		<div class="text-[10px] uppercase tracking-wider text-amber-500/80 px-1">Force</div>
		{#each force as a}
			<button
				onclick={() => handleClick(a)}
				disabled={isDisabled(a)}
				title={a.desc}
				class="w-full text-left px-3 py-2 rounded-lg text-sm flex items-center gap-2.5 transition-colors
					text-amber-300/90 hover:bg-amber-500/10 hover:text-amber-200
					disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent"
			>
				<a.icon class="w-4 h-4 shrink-0" />
				{a.label}
			</button>
		{/each}
	</div>

	<!-- Danger actions: visually separated, red, double-confirm -->
	<div class="space-y-1 pt-3 border-t border-zinc-800">
		<div class="text-[10px] uppercase tracking-wider text-red-500/80 px-1 flex items-center gap-1">
			<AlertTriangle class="w-3 h-3" /> Danger
		</div>
		{#each danger as a}
			<button
				onclick={() => handleClick(a)}
				disabled={isDisabled(a)}
				title={a.desc}
				class="w-full text-left px-3 py-2 rounded-lg text-sm flex items-center gap-2.5 transition-colors
					text-red-400 hover:bg-red-500/10 hover:text-red-300
					disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent"
			>
				<a.icon class="w-4 h-4 shrink-0" />
				{a.label}
			</button>
		{/each}
	</div>
</div>

<!-- Confirmation modal for force / danger actions -->
{#if pendingAction && pendingActionDef}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
		<div class="bg-zinc-900 border border-zinc-700 rounded-xl p-6 w-96 shadow-2xl">
			<div class="flex items-center gap-3 mb-3">
				<AlertTriangle class="w-5 h-5 {pendingActionDef.severity === 'danger' ? 'text-red-400' : 'text-amber-400'}" />
				<h3 class="font-semibold text-zinc-100">{pendingActionDef.label}</h3>
			</div>
			<p class="text-sm text-zinc-400 mb-5 leading-relaxed">{pendingActionDef.desc}</p>
			<div class="flex gap-3 justify-end">
				<button
					onclick={() => (pendingAction = null)}
					class="px-4 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
				>Cancel</button>
				<button
					onclick={() => pendingAction && execute(pendingAction)}
					class="px-4 py-2 text-sm rounded-lg text-white
						{pendingActionDef.severity === 'danger'
							? 'bg-red-600 hover:bg-red-500'
							: 'bg-amber-600 hover:bg-amber-500'}"
				>{pendingActionDef.label}</button>
			</div>
		</div>
	</div>
{/if}
