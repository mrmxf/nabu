import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	kit: {
		// Build the single-page app into pb_public at the REPO ROOT (../ from
		// web/) — the sibling of the default ./pb_data, so `serve` from the repo
		// root finds it. fallback: index.html lets client routes (/session/:id) resolve.
		adapter: adapter({
			pages: '../pb_public',
			assets: '../pb_public',
			fallback: 'index.html',
			precompress: false,
			strict: false
		})
	}
};

export default config;
