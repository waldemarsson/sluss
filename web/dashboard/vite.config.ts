import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { playwright } from '@vitest/browser-playwright';
// From 'vitest/config', not 'vite' — the plain vite `defineConfig` doesn't type the `test` key.
import { defineConfig } from 'vitest/config';

export default defineConfig({
	// SvelteKit 3 takes its config through the Vite plugin; there is no svelte.config.js.
	plugins: [
		sveltekit({
			// adapter-static writes plain files that go:embed can carry inside the binary;
			// there is no Node server in production (docs/DECISIONS.md D8).
			adapter: adapter({ pages: 'build', assets: 'build', precompress: false })
		})
	],
	// `npm run dev` talks to a locally running slussd rather than mocking it.
	server: { proxy: { '/api': 'http://127.0.0.1:8420' } },

	// Two projects, split by directory, matching the homehub and babytabs frontends.
	test: {
		projects: [
			{
				extends: true,
				test: {
					name: 'unit',
					include: ['src/**/*.test.{js,ts}'],
					exclude: ['src/lib/**', 'src/routes/**'],
					environment: 'node'
					// The dashboard is all components so far — every .svelte file talks to the
					// API and the DOM, and there is no framework-free module to cover here yet,
					// so `test:unit` carries --passWithNoTests. Drop that flag once src/ grows
					// one; it stays off `test:component` so a broken glob fails loudly there.
				}
			},
			{
				extends: true,
				test: {
					name: 'component',
					include: ['src/lib/**/*.test.{js,ts}', 'src/routes/**/*.test.{js,ts}'],
					// A real browser, not jsdom: these components are driven by fetch and by
					// form-control state (bind:value on a <select> and a <textarea>), and
					// asserting those against a DOM simulation would be testing the simulation.
					browser: {
						enabled: true,
						// Vitest 4 split the browser providers into their own packages, so this
						// is the `playwright()` factory rather than the `'playwright'` string
						// older docs still show.
						provider: playwright(),
						headless: true,
						instances: [{ browser: 'chromium' }]
					}
				}
			}
		]
	}
});
