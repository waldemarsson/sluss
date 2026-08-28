import { render } from 'vitest-browser-svelte';
import { afterEach, expect, test, vi } from 'vitest';
import Kits from './Kits.svelte';

afterEach(() => vi.unstubAllGlobals());

// render() is async in vitest-browser-svelte 3 — without the await, `screen` is a
// Promise and every locator call on it is undefined.
test('reports an empty kit directory', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(JSON.stringify({ kits: [] }), { status: 200 }))
	);

	const screen = await render(Kits);

	await expect.element(screen.getByText('No kits in the configured directory.')).toBeVisible();
});
