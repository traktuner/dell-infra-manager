<script lang="ts">
	import type { VirtualMedia } from '$lib/types';
	import { api } from '$lib/api';
	import { Disc, LogOut, Link, AlertCircle } from '@lucide/svelte';

	type Props = { serverId: string };
	let { serverId }: Props = $props();

	let media = $state<VirtualMedia[]>([]);
	let loading = $state(true);
	let imageUrl = $state('');
	let working = $state(false);
	let error = $state('');

	async function load() {
		loading = true;
		try {
			media = await api.virtualmedia.status(serverId);
		} catch (e) {
			error = (e as Error).message;
		} finally {
			loading = false;
		}
	}

	$effect(() => { load(); });

	const cdSlot = $derived(media.find((m) => m.Id === 'CD' || m.MediaTypes?.includes('CD')));

	async function insert() {
		if (!imageUrl.trim()) return;
		working = true;
		error = '';
		try {
			await api.virtualmedia.insert(serverId, imageUrl.trim());
			imageUrl = '';
			await load();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			working = false;
		}
	}

	async function eject() {
		working = true;
		error = '';
		try {
			await api.virtualmedia.eject(serverId);
			await load();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			working = false;
		}
	}
</script>

<div class="space-y-4">
	{#if loading}
		<div class="text-zinc-500 text-sm">Loading...</div>
	{:else}
		<!-- Current status -->
		<div class="bg-zinc-800/50 rounded-xl p-4">
			<div class="flex items-center gap-3 mb-2">
				<Disc class="w-5 h-5 text-zinc-400" />
				<span class="font-medium text-zinc-200">Virtual CD/DVD</span>
			</div>
			{#if cdSlot?.Inserted}
				<div class="flex items-start gap-2 mb-3">
					<div class="w-2 h-2 rounded-full bg-emerald-500 mt-1.5 shrink-0"></div>
					<div>
						<div class="text-sm text-emerald-400 font-medium">Media inserted</div>
						<div class="text-xs text-zinc-500 break-all mt-0.5">{cdSlot.Image}</div>
						{#if cdSlot.ConnectedVia}
							<div class="text-xs text-zinc-600 mt-0.5">via {cdSlot.ConnectedVia}</div>
						{/if}
					</div>
				</div>
				<button
					onclick={eject}
					disabled={working}
					class="flex items-center gap-2 px-3 py-1.5 text-sm rounded-lg bg-red-600/20
						text-red-400 hover:bg-red-600/30 disabled:opacity-50"
				>
					<LogOut class="w-4 h-4" />
					{working ? 'Ejecting...' : 'Eject Media'}
				</button>
			{:else}
				<div class="text-sm text-zinc-500">No media inserted</div>
			{/if}
		</div>

		<!-- Mount new ISO -->
		{#if !cdSlot?.Inserted}
			<div>
				<label for="iso-url" class="block text-sm text-zinc-400 mb-1.5">
					<Link class="w-3.5 h-3.5 inline mr-1" />
					ISO URL (HTTP, HTTPS, CIFS, NFS)
				</label>
				<div class="flex gap-2">
					<input
						id="iso-url"
						type="url"
						bind:value={imageUrl}
						placeholder="http://fileserver/ubuntu-24.04.iso"
						class="flex-1 bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-sm
							text-zinc-200 placeholder:text-zinc-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
					/>
					<button
						onclick={insert}
						disabled={working || !imageUrl.trim()}
						class="px-4 py-2 text-sm rounded-lg bg-blue-600 text-white hover:bg-blue-500
							disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap"
					>
						{working ? 'Mounting...' : 'Mount ISO'}
					</button>
				</div>
			</div>
		{/if}

		{#if error}
			<div class="flex items-center gap-2 text-sm text-red-400 bg-red-500/10 rounded-lg px-3 py-2">
				<AlertCircle class="w-4 h-4 shrink-0" />
				{error}
			</div>
		{/if}
	{/if}
</div>
