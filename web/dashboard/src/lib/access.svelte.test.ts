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

// sluss not answering is not worth breaking the page over: the create form is empty,
// everything else still works.
test('survives a failed fetch with the empty configuration', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => Promise.reject(new Error('offline')))
	);
	const access = new Access();

	access.load();
	// Settled, not successful: a consumer waiting on `loaded` must not wait forever
	// because sluss was down.
	await vi.waitFor(() => expect(access.loaded).toBe(true));

	expect(access.config.repos).toEqual([]);
	expect(access.config.scopes).toEqual([]);
});

// The wire is untyped: a body missing a key must not become a missing array in every
// consumer. The defaults live in one place so no route has to guard the read.
test('fills in the keys a partial configuration leaves out', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(JSON.stringify({ access: 'host' }), { status: 200 }))
	);
	const access = new Access();

	access.load();
	await vi.waitFor(() => expect(access.loaded).toBe(true));

	expect(access.config.access).toBe('host');
	expect(access.config.repos).toEqual([]);
	expect(access.config.scopes).toEqual([]);
	expect(access.config.hostPrefix).toBe('sluss-');
	expect(access.config.domain).toBe('');
});
