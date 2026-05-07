import type { WSEvent } from './types';

type EventHandler = (event: WSEvent) => void;

class WebSocketManager {
	private ws: WebSocket | null = null;
	private handlers: Map<string, Set<EventHandler>> = new Map();
	private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	private reconnectDelay = 1000;

	connect() {
		const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
		const url = `${proto}//${window.location.host}/ws`;

		this.ws = new WebSocket(url);

		this.ws.onmessage = (e) => {
			try {
				const event: WSEvent = JSON.parse(e.data);
				this.dispatch(event);
			} catch {
				// ignore malformed messages
			}
		};

		this.ws.onclose = () => {
			this.scheduleReconnect();
		};

		this.ws.onerror = () => {
			this.ws?.close();
		};

		this.ws.onopen = () => {
			this.reconnectDelay = 1000; // reset backoff on success
		};
	}

	private scheduleReconnect() {
		if (this.reconnectTimer) return;
		this.reconnectTimer = setTimeout(() => {
			this.reconnectTimer = null;
			this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000);
			this.connect();
		}, this.reconnectDelay);
	}

	private dispatch(event: WSEvent) {
		const handlers = this.handlers.get(event.type);
		if (handlers) {
			handlers.forEach((h) => h(event));
		}
		// Also dispatch to wildcard handlers
		const wildcards = this.handlers.get('*');
		if (wildcards) {
			wildcards.forEach((h) => h(event));
		}
	}

	on(eventType: string, handler: EventHandler): () => void {
		if (!this.handlers.has(eventType)) {
			this.handlers.set(eventType, new Set());
		}
		this.handlers.get(eventType)!.add(handler);
		return () => this.handlers.get(eventType)?.delete(handler);
	}

	disconnect() {
		if (this.reconnectTimer) {
			clearTimeout(this.reconnectTimer);
			this.reconnectTimer = null;
		}
		this.ws?.close();
		this.ws = null;
	}
}

export const wsManager = new WebSocketManager();
