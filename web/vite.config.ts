import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// In dev the SvelteKit server runs on :5173 and proxies API + dashboard
// requests to the PocketBase server on :8090, so the app is same-origin.
export default defineConfig({
	plugins: [sveltekit()],
	server: {
		proxy: {
			'/api': 'http://127.0.0.1:8090',
			'/_': 'http://127.0.0.1:8090'
		}
	}
});
