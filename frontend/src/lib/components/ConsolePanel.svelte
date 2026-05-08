<script lang="ts">
	/**
	 * ConsolePanel — KVM-Console via noVNC (primary) mit SSH/SOL Fallback.
	 *
	 * Ablauf beim Öffnen des Tabs:
	 *  1. POST /vnc/enable → iDRAC aktiviert VNC via Redfish
	 *  2. Erfolg → noVNC verbindet sich über WS-Proxy auf iDRAC:5901
	 *  3. Fehler (kein Enterprise, VNC disabled etc.) → SSH/SOL via xterm.js
	 */
	import { onMount, onDestroy } from 'svelte';
	import { api } from '$lib/api';

	// xterm (SSH/SOL fallback)
	import { Terminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import { WebLinksAddon } from '@xterm/addon-web-links';
	import '@xterm/xterm/css/xterm.css';

	type Props = { serverId: string | undefined };
	let { serverId }: Props = $props();

	// ── State ────────────────────────────────────────────────────────────────
	type Mode    = 'loading' | 'vnc' | 'sol' | 'error';
	type ConnSt  = 'connecting' | 'connected' | 'disconnected' | 'error';

	let mode      = $state<Mode>('loading');
	let connState = $state<ConnSt>('connecting');
	let statusMsg = $state('');
	let vncToken  = $state('');

	// ── DOM refs ─────────────────────────────────────────────────────────────
	let vncContainer: HTMLDivElement;
	let solContainer: HTMLDivElement;

	// ── noVNC (loaded dynamically — SSR-safe) ────────────────────────────────
	let RFB: any = null;       // noVNC RFB class
	let rfb: any = null;       // active RFB instance

	// ── xterm (SOL fallback) ─────────────────────────────────────────────────
	let term: Terminal | null = null;
	let fitAddon: FitAddon | null = null;
	let solWS: WebSocket | null = null;
	let resizeObserver: ResizeObserver | null = null;

	// ── Lifecycle ────────────────────────────────────────────────────────────
	onMount(async () => {
		if (!serverId) return;

		// Load noVNC dynamically (it uses browser APIs, can't run in SSR).
		try {
			// @ts-ignore — noVNC has no TypeScript declarations
			const mod = await import('@novnc/novnc/core/rfb.js');
			RFB = mod.default;
		} catch (e) {
			console.warn('noVNC load failed, will use SOL only:', e);
		}

		await startConsole();
	});

	onDestroy(() => {
		teardownVNC();
		teardownSOL();
	});

	// ── Primary path: VNC ────────────────────────────────────────────────────
	async function startConsole() {
		mode = 'loading';
		connState = 'connecting';
		statusMsg = 'Enabling VNC on iDRAC…';

		try {
			const res = await api.vnc.enable(serverId!);

			if (res.fallback === 'sol' || !RFB) {
				// Redfish failed or noVNC unavailable → fall through to SOL
				statusMsg = res.error ?? 'VNC unavailable';
				startSOL();
				return;
			}

			vncToken = res.token;
			mode = 'vnc';
			// Give Svelte a tick to render vncContainer before mounting noVNC.
			await tick();
			startVNC(res.token);
		} catch (e) {
			// Network error calling /vnc/enable → fall back to SOL
			statusMsg = (e as Error).message;
			startSOL();
		}
	}

	function startVNC(token: string) {
		if (!vncContainer || !RFB) return;
		teardownVNC();

		const url = api.vnc.proxyUrl(serverId!, token);

		rfb = new RFB(vncContainer, url, {
			// noVNC will handle the VNC password via RFB authentication.
			// Our backend proxy is transparent — RFB auth goes through unchanged.
			wsProtocols: ['binary'],
		});

		// Scaling: scale the canvas to fill the container automatically.
		rfb.scaleViewport = true;
		rfb.resizeSession = true;   // send DesktopSize pseudo-encoding to iDRAC
		rfb.clipViewport  = false;

		rfb.addEventListener('connect', () => {
			connState = 'connected';
			statusMsg  = '';
		});

		rfb.addEventListener('disconnect', (e: any) => {
			connState = 'disconnected';
			statusMsg = e.detail?.reason ?? '';
		});

		rfb.addEventListener('credentialsrequired', () => {
			// iDRAC VNC requires the password we set via Redfish.
			// Fetch it from backend and provide it to noVNC.
			fetchVNCPassword().then((pw) => {
				if (pw) rfb.sendCredentials({ password: pw });
			});
		});

		rfb.addEventListener('securityfailure', () => {
			// Wrong password — the stored password may be stale.
			// Reset it so next enable() will re-configure iDRAC.
			api.vnc.reset(serverId!);
			connState = 'error';
			statusMsg  = 'VNC auth failed — click Reconnect to re-configure';
		});
	}

	async function fetchVNCPassword(): Promise<string | null> {
		// The frontend never stores the plaintext VNC password.
		// We ask the backend to decrypt it and return it once (token required).
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

	// ── Fallback path: SSH/SOL ───────────────────────────────────────────────
	function startSOL() {
		mode = 'sol';
		connState = 'connecting';

		// Wait for Svelte to render solContainer.
		setTimeout(() => initSOLTerminal(), 0);
	}

	function initSOLTerminal() {
		if (!solContainer) return;

		term = new Terminal({
			theme: {
				background: '#09090b',
				foreground: '#d4d4d8',
				cursor:     '#a1a1aa',
				selectionBackground: '#3f3f46',
			},
			fontFamily: '"JetBrains Mono", "Fira Code", Menlo, monospace',
			fontSize: 13,
			lineHeight: 1.4,
			cursorBlink: true,
			scrollback: 5000,
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

		connectSOLWebSocket();
	}

	function connectSOLWebSocket() {
		const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
		solWS = new WebSocket(`${proto}//${location.host}/api/v1/servers/${serverId}/console`);
		solWS.binaryType = 'arraybuffer';

		solWS.onopen = () => {
			connState = 'connected';
			sendSOLResize();
		};
		solWS.onmessage = (e) => {
			if (!term) return;
			if (e.data instanceof ArrayBuffer) term.write(new Uint8Array(e.data));
			else term.write(e.data as string);
		};
		solWS.onclose  = (e) => { connState = 'disconnected'; statusMsg = e.reason || ''; };
		solWS.onerror  = ()  => { connState = 'error'; statusMsg = 'WebSocket error'; };
	}

	function sendSOLResize() {
		if (solWS?.readyState !== WebSocket.OPEN || !term) return;
		solWS.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
	}

	function teardownSOL() {
		resizeObserver?.disconnect();
		solWS?.close();
		solWS = null;
		term?.dispose();
		term = null;
	}

	// ── UI actions ───────────────────────────────────────────────────────────
	function reconnect() {
		teardownVNC();
		teardownSOL();
		term?.reset?.();
		startConsole();
	}

	function sendCtrlAltDel() {
		rfb?.sendCtrlAltDel();
	}

	// Svelte tick helper (not imported from svelte — just a promise for nextTick).
	function tick(): Promise<void> {
		return new Promise((r) => setTimeout(r, 0));
	}
</script>

<div class="flex flex-col h-full min-h-0 bg-zinc-950">

	<!-- ── Status bar ───────────────────────────────────────────────────── -->
	<div class="flex items-center justify-between px-4 py-2 border-b border-zinc-800 shrink-0">
		<div class="flex items-center gap-2 text-xs">
			{#if mode === 'loading'}
				<span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
				<span class="text-zinc-400">{statusMsg || 'Connecting…'}</span>

			{:else if connState === 'connecting'}
				<span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
				<span class="text-zinc-400">Connecting…</span>

			{:else if connState === 'connected'}
				<span class="w-2 h-2 rounded-full bg-emerald-400"></span>
				{#if mode === 'vnc'}
					<span class="text-zinc-400">KVM · VNC</span>
				{:else}
					<span class="text-zinc-500">SSH/SOL</span>
					<span class="text-zinc-700">·</span>
					<span class="text-zinc-600 text-xs">VNC nicht verfügbar{statusMsg ? `: ${statusMsg}` : ''}</span>
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
				onclick={reconnect}
				class="px-2 py-1 text-xs rounded bg-zinc-800 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700 transition-colors"
			>Reconnect</button>
		</div>
	</div>

	<!-- ── VNC canvas (noVNC mounts here) ────────────────────────────────── -->
	{#if mode === 'vnc' || mode === 'loading'}
		<div
			bind:this={vncContainer}
			class="flex-1 min-h-0 {mode === 'loading' ? 'hidden' : ''}"
			style="cursor: default;"
		></div>
	{/if}

	<!-- ── SSH/SOL xterm fallback ─────────────────────────────────────────── -->
	{#if mode === 'sol'}
		<div
			bind:this={solContainer}
			class="flex-1 min-h-0 p-2"
		></div>
	{/if}

	<!-- ── Loading spinner ───────────────────────────────────────────────── -->
	{#if mode === 'loading'}
		<div class="flex-1 flex items-center justify-center">
			<div class="text-zinc-600 text-sm animate-pulse">{statusMsg || 'Verbinde…'}</div>
		</div>
	{/if}
</div>
