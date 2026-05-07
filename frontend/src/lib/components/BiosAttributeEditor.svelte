<script lang="ts">
	import type { BiosRegistryEntry } from '$lib/types';
	import { api } from '$lib/api';
	import { Save, X, AlertCircle } from '@lucide/svelte';
	import { untrack } from 'svelte';

	type Props = {
		serverId: string;
		entry: BiosRegistryEntry;
		onclose?: () => void;
		onsave?: () => void;
	};
	let { serverId, entry, onclose, onsave }: Props = $props();

	let value = $state<string | number>(
		untrack(() =>
			typeof entry.current_value === 'number'
				? entry.current_value
				: String(entry.current_value ?? '')
		)
	);
	let saving = $state(false);
	let error = $state('');

	async function save() {
		saving = true;
		error = '';
		try {
			const attrs: Record<string, unknown> = {
				[entry.AttributeName]: entry.Type === 'Integer' ? Number(value) : value
			};
			await api.bios.patch(serverId, attrs, 'OnReset');
			onsave?.();
			onclose?.();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
		}
	}
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
	<div class="bg-zinc-900 border border-zinc-700 rounded-xl p-6 w-[480px] shadow-2xl">
		<div class="flex items-start justify-between mb-4">
			<div>
				<h3 class="font-semibold text-zinc-100">{entry.DisplayName}</h3>
				<p class="text-xs text-zinc-500 mt-0.5">{entry.AttributeName}</p>
			</div>
			<button onclick={onclose} class="text-zinc-500 hover:text-zinc-300">
				<X class="w-5 h-5" />
			</button>
		</div>

		<div class="mb-5">
			{#if entry.Type === 'Enumeration' && entry.AllowedValues?.length}
				<label for="bios-enum" class="block text-sm text-zinc-400 mb-1.5">Value</label>
				<select
					id="bios-enum"
					bind:value
					class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-zinc-200
						text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
				>
					{#each entry.AllowedValues as av}
						<option value={String(av)}>{String(av)}</option>
					{/each}
				</select>
			{:else if entry.Type === 'Boolean'}
				<label for="bios-bool" class="block text-sm text-zinc-400 mb-1.5">Value</label>
				<select
					id="bios-bool"
					bind:value
					class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-zinc-200 text-sm"
				>
					<option value="Enabled">Enabled</option>
					<option value="Disabled">Disabled</option>
				</select>
			{:else}
				<label for="bios-val" class="block text-sm text-zinc-400 mb-1.5">
					Value
					{#if entry.Type === 'Integer' && entry.LowerBound != null && entry.UpperBound != null}
						<span class="text-zinc-600">({entry.LowerBound}–{entry.UpperBound})</span>
					{/if}
				</label>
				<input
					id="bios-val"
					type={entry.Type === 'Integer' ? 'number' : 'text'}
					bind:value
					min={entry.LowerBound}
					max={entry.UpperBound}
					class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-2 text-zinc-200
						text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
				/>
			{/if}
		</div>

		<div class="bg-amber-500/10 border border-amber-500/20 rounded-lg px-3 py-2 mb-4 flex items-start gap-2">
			<AlertCircle class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
			<p class="text-xs text-amber-300">
				Changes will be applied at the next server reboot (OnReset).
			</p>
		</div>

		{#if error}
			<p class="text-sm text-red-400 mb-4">{error}</p>
		{/if}

		<div class="flex gap-3 justify-end">
			<button
				onclick={onclose}
				class="px-4 py-2 text-sm rounded-lg bg-zinc-800 text-zinc-300 hover:bg-zinc-700"
			>
				Cancel
			</button>
			<button
				onclick={save}
				disabled={saving || entry.ReadOnly}
				class="flex items-center gap-2 px-4 py-2 text-sm rounded-lg bg-blue-600 text-white
					hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
			>
				<Save class="w-4 h-4" />
				{saving ? 'Saving...' : 'Queue Change'}
			</button>
		</div>
	</div>
</div>
