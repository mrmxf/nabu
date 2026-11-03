import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Build the single-page app into ../webapp/dist, which `go build -tags
		// webembed` embeds in the binary (see webapp/). The server serves the
		// embedded app first, then pb_public/ (seeded lesson files) from disk.
		// fallback: index.html lets client routes (/session/:id) resolve.
		adapter: adapter({
			pages: '../webapp/dist',
			assets: '../webapp/dist',
			fallback: 'index.html',
			precompress: false,
			strict: false
		})
	}
};

export default config;
