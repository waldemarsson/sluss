import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { playwright } from '@vitest/browser-playwright';
// From 'vitest/config', not 'vite' — the plain vite `defineConfig` doesn't type the `test` key.
import { defaultExclude, defineConfig } from 'vitest/config';

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
					// The suffix is the rule, not the directory: src/lib now holds both a
					// framework-free module (api.ts) and the components and rune modules that
					// need a real browser, so a path split can no longer tell them apart.
					include: ['src/**/*.unit.test.{js,ts}'],
					environment: 'node'
				}
			},
			{
				extends: true,
				test: {
					name: 'component',
					include: ['src/lib/**/*.test.{js,ts}', 'src/routes/**/*.test.{js,ts}'],
					// Spread, not replaced: assigning `exclude` overrides Vitest's own defaults,
					// which are what keep a stray node_modules under src/ from being collected.
					exclude: [...defaultExclude, 'src/**/*.unit.test.{js,ts}'],
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
