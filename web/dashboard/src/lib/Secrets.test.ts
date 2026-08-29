import { render } from 'vitest-browser-svelte';
import { afterEach, expect, test, vi } from 'vitest';
import Secrets from './Secrets.svelte';

afterEach(() => vi.unstubAllGlobals());

// A released promise chain is microtasks, and those all drain before any timer fires, so
// yielding to the task queue once is enough to let the late response do its worst before
// anything is asserted about what did *not* happen. The zero is the point: no duration
// here is load-bearing.
const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test('lists the names in the scope it is showing', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(JSON.stringify({ names: ['GITHUB_TOKEN'] }), { status: 200 }))
	);

	const screen = await render(Secrets, { scopes: ['work'] });

	await expect.element(screen.getByText('GITHUB_TOKEN')).toBeVisible();
});

// The names of one scope rendered under another scope's label is the worst thing this
// page can do, and it needs no failure to happen — only a slow first response.
test('a response for the previous scope never replaces the current one', async () => {
	let releaseWork = () => {};
	const workInFlight = new Promise<void>((resolve) => (releaseWork = resolve));

	vi.stubGlobal(
		'fetch',
		vi.fn(async (url: string) => {
			if (url === '/api/secrets/work') {
				await workInFlight;
				return new Response(JSON.stringify({ names: ['WORK_ONLY'] }), { status: 200 });
			}
			return new Response(JSON.stringify({ names: ['PERSONAL_ONLY'] }), { status: 200 });
		})
	);

	const screen = await render(Secrets, { scopes: ['work', 'personal'] });
	// Switch away while work is still in flight, and let personal land first.
	await screen.getByLabelText('secret scope').selectOptions('personal');
	await expect.element(screen.getByText('PERSONAL_ONLY')).toBeVisible();

	releaseWork();
	await settle();

	expect(screen.getByText('WORK_ONLY').elements()).toHaveLength(0);
	await expect.element(screen.getByText('PERSONAL_ONLY')).toBeVisible();
});

test('reports a refusal instead of listing nothing silently', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(JSON.stringify({ error: 'no such scope' }), { status: 404 }))
	);

	const screen = await render(Secrets, { scopes: ['gone'] });

	await expect.element(screen.getByText('no such scope')).toBeVisible();
});

// The defect this covers: send() set `error`, then refresh() cleared it on entry, so a
// refused write reported nothing and the user was left guessing from the list alone.
test('shows why a write was refused, alongside the refreshed list', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async (_url: string, init?: RequestInit) =>
			init?.method === 'PUT'
				? new Response(JSON.stringify({ error: 'secret store is read-only' }), { status: 409 })
				: new Response(JSON.stringify({ names: ['GITHUB_TOKEN'] }), { status: 200 })
		)
	);

	const screen = await render(Secrets, { scopes: ['work'] });
	await screen.getByLabelText('secret name').fill('NEW_TOKEN');
	await screen.getByLabelText('secret value').fill('s3cret');
	await screen.getByRole('button', { name: 'set' }).click();

	await expect.element(screen.getByText('secret store is read-only')).toBeVisible();
	// The list refreshed too — the reason and the current state arrive together.
	await expect.element(screen.getByText('GITHUB_TOKEN')).toBeVisible();
});
