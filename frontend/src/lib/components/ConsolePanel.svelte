<script lang="ts">
	/**
	 * ConsolePanel — KVM-Console via noVNC, mit SSH/SOL als Fallback.
	 *
	 *   1. POST /vnc/enable (idempotent) → port + token
	 *      - Backend liest iDRAC-Status; PATCHt nur wenn nicht bereits aktiv.
	 *      - Bei Reconnect derselbe Pfad: idempotent, schnell, kein Mehraufwand.
	 *   2. noVNC verbindet WebSocket → Backend tunnelt zu iDRAC:<port>
	 *   3. Bei Fehler (kein Enterprise, Redfish nicht erreichbar) → SSH/SOL.
	 */
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';

	import { Terminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import '@xterm/xterm/css/xterm.css';

	type Props = { serverId: string | undefined };
	let { serverId }: Props = $props();

	type Mode    = 'loading' | 'vnc' | 'sol' | 'error';
	type ConnSt  = 'connecting' | 'connected' | 'disconnected' | 'error';

	let mode      = $state<Mode>('loading');
	let connState = $state<ConnSt>('connecting');
	let statusMsg = $state('');
	let vncToken  = $state('');
	let vncPort   = $state(5901);

	let vncContainer = $state<HTMLDivElement | null>(null);
	let solContainer = $state<HTMLDivElement | null>(null);

	// noVNC RFB class — loaded dynamically (browser-only).
	let RFB: any = null;
	let rfb: any = null;

	// xterm SOL fallback state.
	let term: Terminal | null = null;
	let fitAddon: FitAddon | null = null;
	let solWS: WebSocket | null = null;
	let resizeObserver: ResizeObserver | null = null;

	onMount(async () => {
		if (!serverId) return;
		try {
			// @ts-ignore — noVNC has no TypeScript declarations.
			const mod: any = await import('@novnc/novnc');
			RFB = mod.default ?? mod.RFB ?? mod;
		} catch (e) {
			console.warn('noVNC unavailable, will use SOL only:', e);
		}
		await connect();
	});

	onDestroy(() => {
		teardownVNC();
		teardownSOL();
	});

	// connect: idempotent — works for first open and reconnect equally.
	async function connect() {
		teardownVNC();
		teardownSOL();
		mode = 'loading';
		connState = 'connecting';
		statusMsg = 'Verbinde…';

		try {
			const res = await api.vnc.enable(serverId!);

			if (res.fallback === 'sol' || !RFB) {
				statusMsg = res.error ?? 'VNC nicht verfügbar';
				startSOL();
				return;
			}

			vncToken = res.token;
			vncPort  = res.port;
			mode = 'vnc';
			await tick();
			startVNC();
		} catch (e) {
			statusMsg = (e as Error).message;
			startSOL();
		}
	}

	function startVNC() {
		if (!vncContainer || !RFB) return;

		const url = api.vnc.proxyUrl(serverId!, vncToken);

		rfb = new RFB(vncContainer, url, { wsProtocols: ['binary'] });
		rfb.scaleViewport = true;
		rfb.resizeSession = true;
		rfb.clipViewport  = false;

		rfb.addEventListener('connect', () => {
			connState = 'connected';
			statusMsg  = '';
		});
		rfb.addEventListener('disconnect', (e: any) => {
			connState = 'disconnected';
			statusMsg  = e.detail?.reason ?? '';
		});
		rfb.addEventListener('credentialsrequired', () => {
			fetchVNCPassword().then((pw) => {
				if (pw) rfb.sendCredentials({ password: pw });
			});
		});
		rfb.addEventListener('securityfailure', () => {
			// Stored password no longer matches what's on iDRAC — force a fresh
			// rotation on the next reconnect.
			api.vnc.reset(serverId!);
			connState = 'error';
			statusMsg  = 'Auth fehlgeschlagen — Reconnect erneuert das Passwort';
		});
	}

	async function fetchVNCPassword(): Promise<string | null> {
		try {
			const res = await fetch(
				`/api/v1/servers/${serverId}/vnc/password?token=${encodeURIComponent(vncToken)}`
			);
			if (!res.ok) return null;
			const { password } = await res.json();
			return password as string;
		} catch {
			return null;
		}
	}

	function teardownVNC() {
		try { rfb?.disconnect(); } catch {}
		rfb = null;
	}

	// ── SOL fallback ─────────────────────────────────────────────────────────
	function startSOL() {
		mode = 'sol';
		connState = 'connecting';
		setTimeout(initSOLTerminal, 0);
	}

	function initSOLTerminal() {
		if (!solContainer) return;

		term = new Terminal({
			theme: {
				background: '#09090b',
				foreground: '#d4d4d8',
				cursor:     '#a1a1aa',
				selectionBackground: '#3f3f46'
			},
			fontFamily: '"JetBrains Mono", "Fira Code", Menlo, monospace',
			fontSize: 13,
			lineHeight: 1.4,
			cursorBlink: true,
			scrollback: 5000
		});

		fitAddon = new FitAddon();
		term.loadAddon(fitAddon);
		term.loadAddon(new WebLinksAddon());
		term.open(solContainer);
		fitAddon.fit();

		term.onData((data) => {
			if (solWS?.readyState === WebSocket.OPEN) {
				solWS.send(new TextEncoder().encode(data));
			}
		});

		resizeObserver = new ResizeObserver(() => {
			fitAddon?.fit();
			sendSOLResize();
		});
		resizeObserver.observe(solContainer);

		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		solWS = new WebSocket(`${proto}//${location.host}/api/v1/servers/${serverId}/console`);
		solWS.binaryType = 'arraybuffer';

		solWS.onopen = () => { connState = 'connected'; sendSOLResize(); };
		solWS.onmessage = (e) => {
			if (!term) return;
			if (e.data instanceof ArrayBuffer) term.write(new Uint8Array(e.data));
			else term.write(e.data as string);
		};
		solWS.onclose = (e) => { connState = 'disconnected'; statusMsg = e.reason || ''; };
		solWS.onerror = ()  => { connState = 'error'; statusMsg = 'WebSocket error'; };
	}

	function sendSOLResize() {
		if (solWS?.readyState !== WebSocket.OPEN || !term) return;
		solWS.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
	}

	function teardownSOL() {
		resizeObserver?.disconnect();
		resizeObserver = null;
		solWS?.close();
		solWS = null;
		term?.dispose();
		term = null;
	}

	function sendCtrlAltDel() { rfb?.sendCtrlAltDel(); }

	function tick(): Promise<void> {
		return new Promise((r) => setTimeout(r, 0));
	}
</script>

<div class="flex flex-col h-full min-h-0 bg-zinc-950">

	<!-- Status bar -->
	<div class="flex items-center justify-between px-4 py-2 border-b border-zinc-800 shrink-0">
		<div class="flex items-center gap-2 text-xs">
			{#if connState === 'connecting'}
				<span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
				<span class="text-zinc-400">{statusMsg || 'Verbinde…'}</span>

			{:else if connState === 'connected'}
				<span class="w-2 h-2 rounded-full bg-emerald-400"></span>
				{#if mode === 'vnc'}
					<span class="text-zinc-400">KVM · VNC</span>
					<span class="text-zinc-700">·</span>
					<span class="text-zinc-600">Port {vncPort}</span>
				{:else}
					<span class="text-zinc-500">SSH/SOL</span>
					{#if statusMsg}
						<span class="text-zinc-700">·</span>
						<span class="text-zinc-600">VNC: {statusMsg}</span>
					{/if}
				{/if}

			{:else if connState === 'disconnected'}
				<span class="w-2 h-2 rounded-full bg-zinc-600"></span>
				<span class="text-zinc-500">Getrennt{statusMsg ? ` — ${statusMsg}` : ''}</span>

			{:else}
				<span class="w-2 h-2 rounded-full bg-red-500"></span>
				<span class="text-red-400">{statusMsg || 'Fehler'}</span>
			{/if}
		</div>

		<div class="flex items-center gap-2">
			{#if mode === 'vnc' && connState === 'connected'}
				<button
					onclick={sendCtrlAltDel}
					class="px-2 py-1 text-xs rounded bg-zinc-800 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700 transition-colors"
					title="Ctrl+Alt+Del an Server senden"
				>Ctrl+Alt+Del</button>
			{/if}
			{#if mode === 'sol' && connState === 'connected'}
				<span class="text-xs text-zinc-600">
					Tipp: <kbd class="px-1 py-0.5 bg-zinc-800 rounded text-zinc-400">console com2</kbd> für Serial-over-LAN
				</span>
			{/if}
			<button
				onclick={connect}
				class="px-2 py-1 text-xs rounded bg-zinc-800 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700 transition-colors"
			>Reconnect</button>
		</div>
	</div>

	<!-- VNC canvas -->
	{#if mode === 'vnc'}
		<div bind:this={vncContainer} class="flex-1 min-h-0" style="cursor: default;"></div>
	{/if}

	<!-- SSH/SOL terminal -->
	{#if mode === 'sol'}
		<div bind:this={solContainer} class="flex-1 min-h-0 p-2"></div>
	{/if}

	<!-- Loading -->
	{#if mode === 'loading'}
		<div class="flex-1 flex items-center justify-center">
			<div class="text-zinc-600 text-sm animate-pulse">{statusMsg || 'Verbinde…'}</div>
		</div>
	{/if}
</div>
