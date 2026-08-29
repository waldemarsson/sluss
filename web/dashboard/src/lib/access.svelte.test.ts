import { afterEach, expect, test, vi } from 'vitest';
import { Access } from './access.svelte.js';

afterEach(() => vi.unstubAllGlobals());

test('fetches the configuration once and holds it', async () => {
	const fetched = vi.fn(
		async () =>
			new Response(
				JSON.stringify({
					access: 'host',
					hostPrefix: 'sluss-',
					domain: 'example.test',
					repos: ['/repos/sluss'],
					scopes: ['work']
				}),
				{ status: 200 }
			)
	);
	vi.stubGlobal('fetch', fetched);
	const access = new Access();

	expect(access.loaded).toBe(false);
	access.load();
	access.load();
	await vi.waitFor(() => expect(access.config.scopes).toEqual(['work']));

	expect(fetched).toHaveBeenCalledTimes(1);
	expect(access.config.access).toBe('host');
	expect(access.loaded).toBe(true);
});

// slussd not answering is not worth breaking the page over: the create form is empty,
// everything else still works.
test('survives a failed fetch with the empty configuration', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => Promise.reject(new Error('offline')))
	);
	const access = new Access();

	access.load();
	// Settled, not successful: a consumer waiting on `loaded` must not wait forever
	// because slussd was down.
	await vi.waitFor(() => expect(access.loaded).toBe(true));

	expect(access.config.repos).toEqual([]);
	expect(access.config.scopes).toEqual([]);
});
