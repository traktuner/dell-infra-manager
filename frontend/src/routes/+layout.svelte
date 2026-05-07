<script lang="ts">
	import '../app.css';
	import Sidebar from '$lib/components/Sidebar.svelte';
	import { wsManager } from '$lib/websocket';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(() => {
		wsManager.connect();
		return () => wsManager.disconnect();
	});
</script>

<div class="flex min-h-screen bg-zinc-950">
	<Sidebar />
	<main class="flex-1 overflow-auto p-6">
		{@render children()}
	</main>
</div>
