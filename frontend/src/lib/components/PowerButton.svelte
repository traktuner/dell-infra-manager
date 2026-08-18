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
		short: string;       // one-liner for confirmation header (e.g. "Reboot the server?")
		desc: string;        // longer explanation shown in the modal body
		action: ResetType;
		severity: Severity;
		icon: any;
		needsOn?: boolean;
		needsOff?: boolean;
	};

	// Three groups by severity:
	//   safe   — graceful, no data loss
	//   force  — abrupt but normal admin action
	//   danger — system-level interrupts; will crash OS / cause data loss
	const safe: Action[] = [
		{
			label: 'Power On',
			short: 'Power on the server?',
			desc: 'Boot the server. The OS will start normally as if you had pressed the physical power button.',
			action: 'On', severity: 'safe', icon: Power, needsOff: true
		},
		{
			label: 'Graceful Shutdown',
			short: 'Shut down the server cleanly?',
			desc: 'Send an ACPI shutdown signal. The OS will close applications, flush caches, and power off — same as choosing "Shut Down" from the OS menu. Safe; no data loss.',
			action: 'GracefulShutdown', severity: 'safe', icon: PowerOff, needsOn: true
		},
		{
			label: 'Graceful Restart',
			short: 'Restart the server cleanly?',
			desc: 'Send an ACPI restart signal. The OS will close applications, flush caches, then reboot — same as choosing "Restart" from the OS menu. Safe; no data loss.',
			action: 'GracefulRestart', severity: 'safe', icon: RotateCcw, needsOn: true
		},
	];
	const force: Action[] = [
		{
			label: 'Force Off',
			short: 'Force the server off?',
			desc: 'Cuts power immediately — same as holding the physical power button for 5 seconds. The OS does NOT get to flush data: open files, in-flight database transactions, and unwritten cache may be corrupted. Use this only when graceful shutdown is unresponsive.',
			action: 'ForceOff', severity: 'force', icon: ZapOff, needsOn: true
		},
		{
			label: 'Force Restart',
			short: 'Force-restart the server?',
			desc: 'Hard-resets the CPU without giving the OS a chance to shut down cleanly. May corrupt open files and in-flight transactions. Use only when graceful restart hangs.',
			action: 'ForceRestart', severity: 'force', icon: RefreshCw, needsOn: true
		},
		{
			label: 'Push Power Button',
			short: 'Simulate the physical power button?',
			desc: 'Sends a single press to the chassis power button. The result depends on the OS power policy: usually graceful shutdown on Linux/Windows, or wake from sleep, or nothing at all. Equivalent to physically pressing the button on the front of the server.',
			action: 'PushPowerButton', severity: 'force', icon: MousePointerClick
		},
	];
	const danger: Action[] = [
		{
			label: 'NMI',
			short: 'Send an NMI (kernel crash trigger)?',
			desc: 'Non-Maskable Interrupt. Forces an immediate kernel panic on Linux or Blue Screen on Windows, producing a crash dump for debugging hangs. The server will become unresponsive and almost certainly lose data. ONLY use this if you are intentionally diagnosing a kernel hang and you understand the consequences.',
			action: 'Nmi', severity: 'danger', icon: TriangleAlert, needsOn: true
		},
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

	// Confirm every power action — even the graceful ones. Power state changes
	// always have non-trivial consequences (services dropped, sessions ended)
	// so a "do you really want to do this?" dialog is the right default.
	function handleClick(a: Action) {
		pendingAction = a.action;
	}

	const pendingActionDef = $derived(
		[...safe, ...force, ...danger].find((a) => a.action === pendingAction)
	);

	// Map severity → modal accent colour. Used both for the icon and the
	// confirm button so the visual weight matches the consequences.
	const severityStyle = {
		safe:   { iconColor: 'text-blue-400',  btnClass: 'bg-blue-600 hover:bg-blue-500' },
		force:  { iconColor: 'text-amber-400', btnClass: 'bg-amber-600 hover:bg-amber-500' },
		danger: { iconColor: 'text-red-400',   btnClass: 'bg-red-600 hover:bg-red-500' }
	} as const;
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
				class="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors
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
				class="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors
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
				class="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left text-sm transition-colors
					text-red-400 hover:bg-red-500/10 hover:text-red-300
					disabled:opacity-30 disabled:cursor-not-allowed disabled:hover:bg-transparent"
			>
				<a.icon class="w-4 h-4 shrink-0" />
				{a.label}
			</button>
		{/each}
	</div>
</div>

<!-- Confirmation modal — shown for EVERY power action.
     Esc and click-outside cancel; only the action button confirms. -->
{#if pendingAction && pendingActionDef}
	{@const sev = severityStyle[pendingActionDef.severity]}
	<div
		role="dialog"
		aria-modal="true"
		tabindex="-1"
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-3 backdrop-blur-sm sm:p-4"
		onclick={(e) => { if (e.target === e.currentTarget) pendingAction = null; }}
		onkeydown={(e) => { if (e.key === 'Escape') pendingAction = null; }}
	>
		<div
			role="document"
			tabindex="-1"
			class="max-h-[calc(100dvh-1.5rem)] w-full max-w-md overflow-y-auto rounded-xl border border-zinc-800 bg-zinc-900 shadow-2xl"
		>
			<!-- Header with icon + short question -->
			<div class="flex items-start gap-3 border-b border-zinc-800 px-4 pb-3 pt-5 sm:px-6">
				<div class="w-9 h-9 rounded-lg bg-zinc-800 flex items-center justify-center shrink-0">
					<pendingActionDef.icon class="w-4 h-4 {sev.iconColor}" />
				</div>
				<div class="flex-1 min-w-0">
					<h3 class="text-base font-semibold text-zinc-100">{pendingActionDef.short}</h3>
					<div class="text-xs text-zinc-500 mt-0.5">Power action · {pendingActionDef.label}</div>
				</div>
			</div>

			<!-- Body: explanation -->
			<div class="px-4 py-4 sm:px-6">
				<p class="text-sm text-zinc-300 leading-relaxed">{pendingActionDef.desc}</p>

				{#if pendingActionDef.severity !== 'safe'}
					<div class="mt-4 flex items-start gap-2 px-3 py-2 rounded-lg
						{pendingActionDef.severity === 'danger' ? 'bg-red-500/10 text-red-300' : 'bg-amber-500/10 text-amber-300'}
						text-xs">
						<AlertTriangle class="w-4 h-4 shrink-0 mt-0.5" />
						<span>
							{pendingActionDef.severity === 'danger'
								? 'This will forcibly interrupt the running OS and almost certainly cause data loss. Only proceed if you understand what you are doing.'
								: 'This bypasses the OS shutdown sequence — open files and in-flight transactions may be corrupted.'}
						</span>
					</div>
				{/if}
			</div>

			<!-- Footer: cancel / confirm -->
			<div class="flex flex-col-reverse gap-2 rounded-b-xl border-t border-zinc-800 bg-zinc-950/50 px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-6">
				<button
					onclick={() => (pendingAction = null)}
					class="min-h-11 w-full rounded-lg bg-zinc-800 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-700 sm:w-auto"
				>Cancel</button>
				<button
					onclick={() => pendingAction && execute(pendingAction)}
					class="min-h-11 w-full rounded-lg px-4 py-2 text-sm text-white transition-colors sm:w-auto {sev.btnClass}"
				>{pendingActionDef.label}</button>
			</div>
		</div>
	</div>
{/if}
