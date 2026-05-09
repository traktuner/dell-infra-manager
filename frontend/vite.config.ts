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
	// noVNC v1.7+ uses top-level await; bump build target to es2022 so esbuild
	// emits it instead of failing with "Top-level await is not available".
	// All evergreen browsers from 2022+ support es2022.
	build: {
		target: 'es2022'
	},
	// noVNC is browser-only — loaded via dynamic import in onMount. Mark it
	// external for SSR so the prerender pass doesn't try to evaluate it.
	ssr: {
		external: ['@novnc/novnc']
	},
	optimizeDeps: {
		exclude: ['@novnc/novnc']
	}
});
