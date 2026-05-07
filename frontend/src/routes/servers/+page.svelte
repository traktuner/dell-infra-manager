<script lang="ts">
	import type { Server } from '$lib/types';
	import { api } from '$lib/api';
	import StatusBadge from '$lib/components/StatusBadge.svelte';
	import { onMount } from 'svelte';
	import { Plus, Trash2, Edit2, CheckCircle, XCircle, ExternalLink } from '@lucide/svelte';

	let servers = $state<Server[]>([]);
	let loading = $state(true);
	let showForm = $state(false);
	let editingId = $state<string | null>(null);
	let testing = $state(false);
	let testResult = $state<{ ok: boolean; error?: string } | null>(null);
	let saving = $state(false);
	let form = $state({
		name: '',
		hostname: '',
		port: 443,
		username: 'root',
		password: '',
		tls_verify: false,
		tags: '[]'
	});

	async function load() {
		servers = await api.servers.list();
		loading = false;
	}

	onMount(load);

	function openCreate() {
		editingId = null;
		resetForm();
		showForm = true;
	}

	function openEdit(s: Server) {
		editingId = s.id;
		form = {
			name: s.name,
			hostname: s.hostname,
			port: s.port,
			username: s.username,
			password: '', // empty = keep existing
			tls_verify: s.tls_verify,
			tags: s.tags || '[]'
		};
		testResult = null;
		showForm = true;
	}

	async function save() {
		saving = true;
		try {
			if (editingId) {
				// On edit, only send password if user typed one (otherwise keep existing)
				const payload: Record<string, unknown> = { ...form };
				if (!form.password) delete payload.password;
				await api.servers.update(editingId, payload);
			} else {
				await api.servers.create(form);
			}
			showForm = false;
			editingId = null;
			resetForm();
			await load();
		} catch (e) {
			alert((e as Error).message);
		} finally {
			saving = false;
		}
	}

	async function testConn() {
		testing = true;
		testResult = null;
		try {
			testResult = await api.servers.testCredentials(form);
		} catch (e) {
			testResult = { ok: false, error: (e as Error).message };
		} finally {
			testing = false;
		}
	}

	async function deleteServer(id: string, name: string) {
		if (!confirm(`Delete server "${name}"?`)) return;
		await api.servers.delete(id);
		await load();
	}

	function resetForm() {
		form = { name: '', hostname: '', port: 443, username: 'root', password: '', tls_verify: false, tags: '[]' };
		testResult = null;
	}
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<h1 class="text-xl font-semibold text-zinc-100">Servers</h1>
		<button
			onclick={openCreate}
			class="flex items-center gap-2 px-4 py-2 rounded-lg bg-blue-600 text-white text-sm hover:bg-blue-500"
		>
			<Plus class="w-4 h-4" />
			Add Server
		</button>
	</div>

	<!-- Add/Edit Server form -->
	{#if showForm}
		<div class="bg-zinc-900 border border-zinc-700 rounded-xl p-6">
			<h2 class="font-semibold text-zinc-200 mb-4">{editingId ? 'Edit Server' : 'Add Server'}</h2>
			<div class="grid grid-cols-2 gap-4 mb-4">
				<div>
					<label for="srv-name" class="block text-sm text-zinc-400 mb-1">Display Name</label>
					<input id="srv-name" bind:value={form.name} placeholder="dell-r640-01"
						class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500" />
				</div>
				<div>
					<label for="srv-host" class="block text-sm text-zinc-400 mb-1">iDRAC Hostname / IP</label>
					<input id="srv-host" bind:value={form.hostname} placeholder="192.168.1.100"
						class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500" />
				</div>
				<div>
					<label for="srv-port" class="block text-sm text-zinc-400 mb-1">Port</label>
					<input id="srv-port" bind:value={form.port} type="number"
						class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500" />
				</div>
				<div>
					<label for="srv-user" class="block text-sm text-zinc-400 mb-1">Username</label>
					<input id="srv-user" bind:value={form.username}
						class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500" />
				</div>
				<div>
					<label for="srv-pass" class="block text-sm text-zinc-400 mb-1">
						Password{#if editingId} <span class="text-zinc-600 text-xs">(leave empty to keep)</span>{/if}
					</label>
					<input id="srv-pass" bind:value={form.password} type="password"
						placeholder={editingId ? '••••••••' : ''}
						class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm text-zinc-200 focus:outline-none focus:ring-2 focus:ring-blue-500" />
				</div>
				<div class="flex items-center gap-3 pt-5">
					<input bind:checked={form.tls_verify} type="checkbox" id="tls"
						class="w-4 h-4 rounded border-zinc-600 bg-zinc-800" />
					<label for="tls" class="text-sm text-zinc-400">Verify TLS Certificate</label>
				</div>
			</div>

			{#if testResult}
				<div class="flex items-center gap-2 mb-4 text-sm {testResult.ok ? 'text-emerald-400' : 'text-red-400'}">
					{#if testResult.ok}
						<CheckCircle class="w-4 h-4" />
						Connection successful
					{:else}
						<XCircle class="w-4 h-4" />
						{testResult.error}
					{/if}
				</div>
			{/if}

			<div class="flex items-center gap-3">
				<button onclick={save} disabled={saving}
					class="px-4 py-2 text-sm rounded-lg bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50">
					{saving ? 'Saving...' : editingId ? 'Update Server' : 'Save Server'}
				</button>
				{#if !editingId}
					<button onclick={testConn} disabled={testing || !form.hostname || !form.password}
						class="px-4 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700 disabled:opacity-50">
						{testing ? 'Testing...' : 'Test Connection'}
					</button>
				{/if}
				<button onclick={() => { showForm = false; editingId = null; resetForm(); }}
					class="px-4 py-2 text-sm rounded-lg text-zinc-500 hover:text-zinc-300">
					Cancel
				</button>
			</div>
		</div>
	{/if}

	<!-- Server table -->
	<div class="bg-zinc-900 border border-zinc-800 rounded-xl overflow-hidden">
		<table class="w-full text-sm">
			<thead>
				<tr class="border-b border-zinc-800 text-left text-zinc-500 text-xs uppercase tracking-wide">
					<th class="px-5 py-3">Name</th>
					<th class="px-5 py-3">Hostname</th>
					<th class="px-5 py-3">Tags</th>
					<th class="px-5 py-3"></th>
				</tr>
			</thead>
			<tbody class="divide-y divide-zinc-800">
				{#each servers as s}
					<tr class="hover:bg-zinc-800/30">
						<td class="px-5 py-3">
							<a href="/servers/{s.id}" class="font-medium text-zinc-200 hover:text-white flex items-center gap-1.5">
								{s.name}
								<ExternalLink class="w-3 h-3 text-zinc-600" />
							</a>
						</td>
						<td class="px-5 py-3 text-zinc-400 font-mono text-xs">{s.hostname}:{s.port}</td>
						<td class="px-5 py-3">
							{#each JSON.parse(s.tags || '[]') as tag}
								<span class="inline-block px-2 py-0.5 rounded text-xs bg-zinc-800 text-zinc-400 mr-1">{tag}</span>
							{/each}
						</td>
						<td class="px-5 py-3">
							<div class="flex items-center gap-2 justify-end">
								<button
									onclick={() => openEdit(s)}
									title="Edit"
									class="p-1.5 text-zinc-500 hover:text-zinc-300 hover:bg-zinc-800 rounded-lg"
								>
									<Edit2 class="w-3.5 h-3.5" />
								</button>
								<button
									onclick={() => deleteServer(s.id, s.name)}
									title="Delete"
									class="p-1.5 text-zinc-500 hover:text-red-400 hover:bg-red-500/10 rounded-lg"
								>
									<Trash2 class="w-3.5 h-3.5" />
								</button>
							</div>
						</td>
					</tr>
				{/each}
				{#if !loading && servers.length === 0}
					<tr>
						<td colspan="4" class="px-5 py-10 text-center text-zinc-600">
							No servers configured yet.
						</td>
					</tr>
				{/if}
			</tbody>
		</table>
	</div>
</div>
