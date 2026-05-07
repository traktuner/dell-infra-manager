<script lang="ts">
	import { page } from '$app/state';
	import VirtualMediaPanel from '$lib/components/VirtualMediaPanel.svelte';

	const id = $derived(page.params.id);
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<a href="/servers/{id}" class="text-zinc-500 hover:text-zinc-300 text-sm">← Server</a>
		<h1 class="text-xl font-semibold text-zinc-100">Virtual Media</h1>
	</div>

	<div class="bg-zinc-900 border border-zinc-800 rounded-xl p-6 max-w-xl">
		<VirtualMediaPanel serverId={id} />
	</div>
</div>
