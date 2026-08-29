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

// The same ordering hazard as the secrets scope switch: the editor must never show one
// kit's spec while the selector names another.
test('a spec for the previously selected kit never replaces the current one', async () => {
	let releaseBase = () => {};
	const baseInFlight = new Promise<void>((resolve) => (releaseBase = resolve));

	vi.stubGlobal(
		'fetch',
		vi.fn(async (url: string) => {
			if (url === '/api/kits') {
				return new Response(
					JSON.stringify({
						kits: [
							{ name: 'base', hasSpec: true },
							{ name: 'go-web', hasSpec: true }
						],
						status: { repository: true, dirty: false, detail: '' }
					}),
					{ status: 200 }
				);
			}
			if (url === '/api/kits/base/spec') {
				await baseInFlight;
				return new Response('name: base', { status: 200 });
			}
			return new Response('name: go-web', { status: 200 });
		})
	);

	const screen = await render(Kits);
	// base is selected on load; switch to go-web while its spec is still in flight.
	await screen.getByLabelText('kit').selectOptions('go-web');
	await expect.element(screen.getByLabelText('spec.yaml')).toHaveValue('name: go-web');

	releaseBase();
	// As in Secrets.test.ts: one turn of the task queue drains the whole released chain.
	await new Promise((resolve) => setTimeout(resolve, 0));

	await expect.element(screen.getByLabelText('spec.yaml')).toHaveValue('name: go-web');
});

// The same ordering as Secrets: refreshList() clears `error` on entry, so save() reports
// its refusal after the reload rather than before it.
test('shows why a save was refused, after the list reloads', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async (url: string, init?: RequestInit) => {
			if (init?.method === 'PUT') {
				return new Response(JSON.stringify({ error: 'kits directory is read-only' }), {
					status: 409
				});
			}
			if (url === '/api/kits') {
				return new Response(
					JSON.stringify({
						kits: [{ name: 'base', hasSpec: true }],
						status: { repository: true, dirty: false, detail: '' }
					}),
					{ status: 200 }
				);
			}
			return new Response('name: base', { status: 200 });
		})
	);

	const screen = await render(Kits);
	await expect.element(screen.getByLabelText('spec.yaml')).toHaveValue('name: base');
	await screen.getByRole('button', { name: 'save' }).click();

	await expect.element(screen.getByText('kits directory is read-only')).toBeVisible();
});
