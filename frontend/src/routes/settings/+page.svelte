<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type NotificationSettings, type NotificationSettingsInput } from '$lib/api';
	import { Mail, Send, Save, AlertCircle, CheckCircle2, RefreshCw } from '@lucide/svelte';

	// State for the form. Loaded from backend on mount; password is never
	// returned, so the input is always blank — has_password just tells us
	// whether one exists already (so we know "leave empty = keep existing").
	let loaded = $state(false);
	let saving = $state(false);
	let testing = $state(false);
	let triggering = $state(false);
	let banner = $state<{ kind: 'ok' | 'err'; msg: string } | null>(null);

	let enabled            = $state(false);
	let smtpHost           = $state('');
	let smtpPort           = $state(587);
	let smtpUsername       = $state('');
	let smtpPasswordInput  = $state(''); // blank = keep existing
	let smtpFrom           = $state('');
	let smtpTLS            = $state<'none' | 'starttls' | 'tls'>('starttls');
	let recipientsList     = $state<string[]>(['']);
	let onServerOffline    = $state(true);
	let onHealthCritical   = $state(true);
	let onJobFailed        = $state(true);
	let onFirmwareUpdates  = $state(false);
	let hasStoredPassword  = $state(false);

	function recipientsToJSON(list: string[]): string {
		return JSON.stringify(list.map((r) => r.trim()).filter(Boolean));
	}
	function recipientsFromJSON(s: string): string[] {
		try {
			const arr = JSON.parse(s);
			return Array.isArray(arr) && arr.length > 0 ? arr : [''];
		} catch {
			return [''];
		}
	}

	function buildPayload(): NotificationSettingsInput {
		return {
			enabled,
			smtp_host: smtpHost,
			smtp_port: smtpPort || 587,
			smtp_username: smtpUsername,
			smtp_password: smtpPasswordInput,
			smtp_from: smtpFrom,
			smtp_tls: smtpTLS,
			recipients: recipientsToJSON(recipientsList),
			on_server_offline: onServerOffline,
			on_health_critical: onHealthCritical,
			on_job_failed: onJobFailed,
			on_firmware_updates: onFirmwareUpdates
		};
	}

	function applyServerData(s: NotificationSettings) {
		enabled = s.enabled;
		smtpHost = s.smtp_host;
		smtpPort = s.smtp_port;
		smtpUsername = s.smtp_username;
		smtpFrom = s.smtp_from;
		smtpTLS = s.smtp_tls;
		recipientsList = recipientsFromJSON(s.recipients);
		onServerOffline = s.on_server_offline;
		onHealthCritical = s.on_health_critical;
		onJobFailed = s.on_job_failed;
		onFirmwareUpdates = s.on_firmware_updates;
		hasStoredPassword = s.has_password;
	}

	onMount(async () => {
		try {
			const s = await api.settings.getNotifications();
			applyServerData(s);
		} catch (e) {
			banner = { kind: 'err', msg: (e as Error).message };
		} finally {
			loaded = true;
		}
	});

	async function save() {
		saving = true;
		banner = null;
		try {
			await api.settings.updateNotifications(buildPayload());
			// Reload to reflect what was actually persisted (and to clear the
			// password input — server tells us via has_password).
			const fresh = await api.settings.getNotifications();
			applyServerData(fresh);
			smtpPasswordInput = '';
			banner = { kind: 'ok', msg: 'Settings saved.' };
		} catch (e) {
			banner = { kind: 'err', msg: (e as Error).message };
		} finally {
			saving = false;
		}
	}

	async function testEmail() {
		testing = true;
		banner = null;
		try {
			await api.settings.testNotifications(buildPayload());
			banner = { kind: 'ok', msg: 'Test email sent. Check your inbox.' };
		} catch (e) {
			banner = { kind: 'err', msg: (e as Error).message };
		} finally {
			testing = false;
		}
	}

	async function triggerDigest() {
		triggering = true;
		banner = null;
		try {
			await api.settings.sendDigestNow();
			banner = { kind: 'ok', msg: 'Firmware digest triggered. You\'ll receive an email within a few seconds if there are outdated components and the toggle is enabled.' };
		} catch (e) {
			banner = { kind: 'err', msg: (e as Error).message };
		} finally {
			triggering = false;
		}
	}

	function addRecipient() { recipientsList = [...recipientsList, '']; }
	function removeRecipient(i: number) {
		recipientsList = recipientsList.filter((_, idx) => idx !== i);
		if (recipientsList.length === 0) recipientsList = [''];
	}
</script>

<div class="space-y-6 max-w-3xl">
	<div>
		<h1 class="text-xl font-semibold text-zinc-100">Settings</h1>
		<div class="text-sm text-zinc-500 mt-0.5">Configure notifications and integrations.</div>
	</div>

	{#if banner}
		<div class="flex items-center gap-2 px-4 py-3 rounded-xl text-sm
			{banner.kind === 'ok' ? 'bg-emerald-500/10 text-emerald-300' : 'bg-red-500/10 text-red-300'}">
			{#if banner.kind === 'ok'}
				<CheckCircle2 class="w-4 h-4" />
			{:else}
				<AlertCircle class="w-4 h-4" />
			{/if}
			{banner.msg}
		</div>
	{/if}

	{#if !loaded}
		<div class="text-zinc-500 text-sm">Loading settings…</div>
	{:else}
		<!-- Email Notifications -->
		<section class="bg-zinc-900 border border-zinc-800 rounded-xl p-6 space-y-5">
			<header class="flex items-start justify-between">
				<div class="flex items-center gap-2.5">
					<Mail class="w-4 h-4 text-zinc-400" />
					<h2 class="text-sm font-medium text-zinc-200">Email Notifications</h2>
				</div>
				<label class="flex items-center gap-2 text-sm text-zinc-300 cursor-pointer">
					<input type="checkbox" bind:checked={enabled}
						class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-500 focus:ring-blue-500"/>
					<span>Enabled</span>
				</label>
			</header>

			<div class="grid grid-cols-2 gap-4">
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">SMTP Host</span>
					<input type="text" bind:value={smtpHost} placeholder="smtp.example.com"
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
				</label>
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">Port</span>
					<input type="number" bind:value={smtpPort} placeholder="587"
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"/>
				</label>
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">Username</span>
					<input type="text" bind:value={smtpUsername} autocomplete="off"
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
				</label>
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">
						Password
						{#if hasStoredPassword}
							<span class="text-zinc-600">(leave empty to keep current)</span>
						{/if}
					</span>
					<input type="password" bind:value={smtpPasswordInput} autocomplete="new-password"
						placeholder={hasStoredPassword ? '••••••••' : ''}
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
				</label>
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">From address</span>
					<input type="email" bind:value={smtpFrom} placeholder="idrac-manager@example.com"
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
				</label>
				<label class="block">
					<span class="block text-xs text-zinc-500 mb-1">Encryption</span>
					<select bind:value={smtpTLS}
						class="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500">
						<option value="starttls">STARTTLS (port 587, recommended)</option>
						<option value="tls">SMTPS / Implicit TLS (port 465)</option>
						<option value="none">None (plaintext, port 25)</option>
					</select>
				</label>
			</div>

			<!-- Recipients -->
			<div>
				<div class="flex items-center justify-between mb-2">
					<span class="text-xs text-zinc-500">Recipients</span>
					<button onclick={addRecipient} class="text-xs text-blue-400 hover:text-blue-300">+ Add</button>
				</div>
				<div class="space-y-2">
					{#each recipientsList as _, i}
						<div class="flex gap-2">
							<input type="email" bind:value={recipientsList[i]} placeholder="alerts@example.com"
								class="flex-1 px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-lg text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500"/>
							{#if recipientsList.length > 1}
								<button onclick={() => removeRecipient(i)}
									class="px-3 text-xs rounded-lg text-zinc-500 hover:text-red-400 hover:bg-red-500/10">Remove</button>
							{/if}
						</div>
					{/each}
				</div>
			</div>

			<!-- Trigger toggles -->
			<div>
				<span class="text-xs text-zinc-500 block mb-2">Send email when:</span>
				<div class="grid grid-cols-2 gap-2 text-sm text-zinc-300">
					<label class="flex items-center gap-2 cursor-pointer p-2 hover:bg-zinc-800/50 rounded-lg">
						<input type="checkbox" bind:checked={onServerOffline}
							class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-500 focus:ring-blue-500"/>
						Server becomes offline
					</label>
					<label class="flex items-center gap-2 cursor-pointer p-2 hover:bg-zinc-800/50 rounded-lg">
						<input type="checkbox" bind:checked={onHealthCritical}
							class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-500 focus:ring-blue-500"/>
						Hardware health is critical
					</label>
					<label class="flex items-center gap-2 cursor-pointer p-2 hover:bg-zinc-800/50 rounded-lg">
						<input type="checkbox" bind:checked={onJobFailed}
							class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-500 focus:ring-blue-500"/>
						An iDRAC job fails
					</label>
					<label class="flex items-center gap-2 cursor-pointer p-2 hover:bg-zinc-800/50 rounded-lg">
						<input type="checkbox" bind:checked={onFirmwareUpdates}
							class="w-4 h-4 rounded border-zinc-700 bg-zinc-800 text-blue-500 focus:ring-blue-500"/>
						Firmware updates available
					</label>
				</div>
				{#if onFirmwareUpdates}
					<div class="mt-2 px-2 text-xs text-zinc-500">
						Sent once per day as a single combined email. The scan runs ~1&nbsp;hour after
						the daily Dell-catalog refresh.
						<button
							onclick={triggerDigest}
							disabled={triggering || !enabled}
							title="Run the digest scan now (no schedule wait). Sends only if there are outdated components."
							class="ml-2 inline-flex items-center gap-1 text-blue-400 hover:text-blue-300 disabled:opacity-50">
							<RefreshCw class="w-3 h-3 {triggering ? 'animate-spin' : ''}" />
							{triggering ? 'Sending…' : 'Send digest now'}
						</button>
					</div>
				{/if}
			</div>

			<!-- Action buttons -->
			<div class="flex items-center justify-end gap-3 pt-2 border-t border-zinc-800">
				<button onclick={testEmail} disabled={testing || !smtpHost}
					title="Send a test email using the current form values (does not save)"
					class="flex items-center gap-2 px-3 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700 disabled:opacity-50">
					<Send class="w-4 h-4" />
					{testing ? 'Sending…' : 'Send test email'}
				</button>
				<button onclick={save} disabled={saving}
					class="flex items-center gap-2 px-4 py-2 text-sm rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50">
					<Save class="w-4 h-4" />
					{saving ? 'Saving…' : 'Save settings'}
				</button>
			</div>
		</section>
	{/if}
</div>
