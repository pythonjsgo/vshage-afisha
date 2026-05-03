import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	// Expose `PUBLIC_*` env vars to `import.meta.env` (default Vite only
	// exposes `VITE_*`). lib/api.ts reads `import.meta.env.PUBLIC_API_URL`
	// to pick the absolute backend URL — without this prefix, the var
	// silently evaluates to undefined, the code falls back to `/api`, and
	// SSR `fetch('/api/...')` throws TypeError because Node can't fetch
	// a relative URL.
	envPrefix: ['VITE_', 'PUBLIC_']
});
