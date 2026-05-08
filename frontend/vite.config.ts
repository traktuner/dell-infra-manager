import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit()
	],
	server: {
		proxy: {
			'/api': 'http://localhost:8080',
			'/ws':  { target: 'ws://localhost:8080', ws: true }
		}
	},
	// @novnc/novnc does not declare deep sub-path exports (./core/rfb.js) in
	// its package.json exports field. Vite 8 (rolldown) enforces this strictly.
	// Marking it external for SSR prevents rolldown from trying to bundle it
	// server-side — it is browser-only, loaded via dynamic import in onMount.
	ssr: {
		external: ['@novnc/novnc']
	},
	optimizeDeps: {
		exclude: ['@novnc/novnc']
	}
});
