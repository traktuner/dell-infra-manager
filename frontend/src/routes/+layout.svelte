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

<div class="min-h-screen bg-zinc-950 md:flex">
	<Sidebar />
	<main class="min-w-0 flex-1 overflow-x-hidden px-4 pb-24 pt-20 sm:px-5 md:p-6">
		{@render children()}
	</main>
</div>
