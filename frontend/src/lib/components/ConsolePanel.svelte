<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Terminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import '@xterm/xterm/css/xterm.css';

	type Props = { serverId: string | undefined };
	let { serverId }: Props = $props();

	let container: HTMLDivElement;
	let term: Terminal | null = null;
	let fitAddon: FitAddon | null = null;
	let ws: WebSocket | null = null;
	let resizeObserver: ResizeObserver | null = null;
	let status = $state<'connecting' | 'connected' | 'disconnected' | 'error'>('connecting');
	let statusMsg = $state('');

	function buildWsUrl(id: string | undefined): string {
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		return `${proto}//${location.host}/api/v1/servers/${id ?? ''}/console`;
	}

	function connect() {
		if (!serverId) return;
		status = 'connecting';
		statusMsg = '';

		ws = new WebSocket(buildWsUrl(serverId));
		ws.binaryType = 'arraybuffer';

		ws.onopen = () => {
			status = 'connected';
			// Send current terminal size so the PTY is sized correctly from the start.
			sendResize();
		};

		ws.onmessage = (e) => {
			if (!term) return;
			if (e.data instanceof ArrayBuffer) {
				term.write(new Uint8Array(e.data));
			} else {
				// Text frame — e.g. error messages streamed before PTY is up.
				term.write(e.data as string);
			}
		};

		ws.onclose = (e) => {
			status = 'disconnected';
			statusMsg = e.reason || `Code ${e.code}`;
			term?.write('\r\n\x1b[33m[Session closed]\x1b[0m\r\n');
		};

		ws.onerror = () => {
			status = 'error';
			statusMsg = 'WebSocket error';
		};
	}

	function sendResize() {
		if (ws?.readyState !== WebSocket.OPEN || !term) return;
		ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
	}

	function disconnect() {
		ws?.close();
		ws = null;
	}

	function reconnect() {
		disconnect();
		term?.reset();
		connect();
	}

	onMount(() => {
		term = new Terminal({
			theme: {
				background: '#09090b',   // zinc-950
				foreground: '#d4d4d8',   // zinc-300
				cursor:     '#a1a1aa',   // zinc-400
				selectionBackground: '#3f3f46', // zinc-700
				black:   '#18181b',
				red:     '#f87171',
				green:   '#4ade80',
				yellow:  '#facc15',
				blue:    '#60a5fa',
				magenta: '#c084fc',
				cyan:    '#22d3ee',
				white:   '#f4f4f5',
				brightBlack:   '#3f3f46',
				brightRed:     '#fca5a5',
				brightGreen:   '#86efac',
				brightYellow:  '#fde68a',
				brightBlue:    '#93c5fd',
				brightMagenta: '#d8b4fe',
				brightCyan:    '#67e8f9',
				brightWhite:   '#fafafa',
			},
			fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", Menlo, monospace',
			fontSize: 13,
			lineHeight: 1.4,
			cursorBlink: true,
			allowProposedApi: true,
			scrollback: 5000,
		});

		fitAddon = new FitAddon();
		term.loadAddon(fitAddon);
		term.loadAddon(new WebLinksAddon());
		term.open(container);
		fitAddon.fit();

		// Forward keystrokes to the SSH session.
		term.onData((data) => {
			if (ws?.readyState === WebSocket.OPEN) {
				ws.send(new TextEncoder().encode(data));
			}
		});

		// Notify the backend when the terminal is resized.
		resizeObserver = new ResizeObserver(() => {
			fitAddon?.fit();
			sendResize();
		});
		resizeObserver.observe(container);

		connect();
	});

	onDestroy(() => {
		resizeObserver?.disconnect();
		disconnect();
		term?.dispose();
	});
</script>

<div class="flex flex-col h-full min-h-0">
	<!-- Status bar -->
	<div class="flex items-center justify-between px-4 py-2 bg-zinc-950 border-b border-zinc-800 shrink-0">
		<div class="flex items-center gap-2 text-xs">
			{#if status === 'connecting'}
				<span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
				<span class="text-zinc-400">Connecting…</span>
			{:else if status === 'connected'}
				<span class="w-2 h-2 rounded-full bg-emerald-400"></span>
				<span class="text-zinc-400">Connected · iDRAC SSH</span>
			{:else if status === 'disconnected'}
				<span class="w-2 h-2 rounded-full bg-zinc-600"></span>
				<span class="text-zinc-500">Disconnected{statusMsg ? ` — ${statusMsg}` : ''}</span>
			{:else}
				<span class="w-2 h-2 rounded-full bg-red-500"></span>
				<span class="text-red-400">{statusMsg || 'Error'}</span>
			{/if}
		</div>
		<div class="flex items-center gap-3 text-xs text-zinc-600">
			<span>Type <kbd class="px-1 py-0.5 bg-zinc-800 rounded text-zinc-400">console com2</kbd> for Serial-over-LAN</span>
			<button
				onclick={reconnect}
				class="px-2 py-1 rounded bg-zinc-800 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700 transition-colors"
			>Reconnect</button>
		</div>
	</div>

	<!-- Terminal -->
	<div
		bind:this={container}
		class="flex-1 min-h-0 p-2 bg-zinc-950"
	></div>
</div>
