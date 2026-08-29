import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import Dashboard from './+page.svelte';
import { access } from '#lib/access.svelte.js';
import { fleet } from '#lib/fleet.svelte.js';
import type { Sandbox } from '#lib/api.js';
// The same stylesheet the root layout loads. Without it the layout assertions below
// would be measuring unstyled markup.
import '../app.css';

const PHONE = 375;
const DESKTOP = 1024;

function sandbox(over: Partial<Sandbox>): Sandbox {
	return {
		scope: 'work',
		name: 'auth',
		id: 'd1e0',
		agent: 'opencode',
		status: 'running',
		repo: '/repos/sluss',
		repoName: 'sluss',
		worktree: '/worktrees/sluss/auth',
		branch: 'feat/auth',
		dirty: true,
		unmerged: 2,
		missing: false,
		webPort: 49155,
		...over
	};
}

const running = sandbox({});
const stopped = sandbox({
	name: 'docs',
	agent: 'claude',
	status: 'stopped',
	branch: 'docs/readme',
	dirty: false,
	unmerged: 0,
	webPort: 0
});

beforeEach(() => {
	access.config = {
		access: 'path',
		hostPrefix: 'sluss-',
		domain: '',
		repos: ['/repos/sluss'],
		scopes: ['work']
	};
	access.loaded = true;
	fleet.snapshot = {
		at: '2026-08-28T10:00:00Z',
		repos: [{ repo: '/repos/sluss', name: 'sluss', sandboxes: [running, stopped] }],
		scopeErrors: []
	};
});

afterEach(async () => {
	await page.viewport(DESKTOP, 768);
	vi.restoreAllMocks();
});

// Records what the page asks the server for, so the lifecycle tests assert the request
// rather than a rendered side effect of it.
function captureFetch(): { url: string; init?: RequestInit }[] {
	const calls: { url: string; init?: RequestInit }[] = [];
	vi.spyOn(window, 'fetch').mockImplementation((input, init) => {
		calls.push({ url: String(input), init: init ?? undefined });
		return Promise.resolve(new Response('{}', { status: 200 }));
	});
	return calls;
}

// Stubs the confirm dialog and hands back the messages it was shown.
function confirmReturns(answer: boolean): () => string[] {
	const asked: string[] = [];
	vi.spyOn(window, 'confirm').mockImplementation((message) => {
		asked.push(String(message));
		return answer;
	});
	return () => asked;
}

// Indexing under noUncheckedIndexedAccess yields T | undefined, and every use below
// wants the assertion to fail on a missing request rather than on a property of one.
function at<T>(items: T[], index = 0): T {
	const found = items[index];
	if (found === undefined) throw new Error(`no entry at index ${index} of ${items.length}`);
	return found;
}

test('renders every sandbox as a card on a phone', async () => {
	await page.viewport(PHONE, 667);
	const screen = await render(Dashboard);

	// The table is not merely hidden: only one rendering is ever in the DOM.
	expect(screen.getByRole('table').elements()).toHaveLength(0);

	await expect.element(screen.getByText('auth', { exact: true })).toBeVisible();
	await expect.element(screen.getByText('feat/auth', { exact: false })).toBeVisible();
	await expect.element(screen.getByText('running', { exact: true })).toBeVisible();
	await expect.element(screen.getByText('dirty')).toBeVisible();
	await expect.element(screen.getByText('2 unmerged')).toBeVisible();
	// The two flags sit directly beside each other and need to read as two things.
	expect(getComputedStyle(screen.getByText('dirty').element()).marginRight).not.toBe('0px');

	expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);

	// Thumb targets, not pointer targets.
	for (const control of screen.getByRole('button').elements()) {
		expect(control.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
	}
	expect(
		screen.getByRole('link', { name: 'open' }).element().getBoundingClientRect().height
	).toBeGreaterThanOrEqual(44);
});

test('offers stop for a running sandbox and start for a stopped one, in either rendering', async () => {
	await page.viewport(PHONE, 667);
	const narrow = await render(Dashboard);
	expect(narrow.getByRole('button', { name: 'stop' }).elements()).toHaveLength(1);
	expect(narrow.getByRole('button', { name: 'start docs' }).elements()).toHaveLength(1);
	expect(narrow.getByRole('button', { name: 'destroy' }).elements()).toHaveLength(2);
	narrow.unmount();

	await page.viewport(DESKTOP, 768);
	const wide = await render(Dashboard);
	expect(wide.getByRole('button', { name: 'stop' }).elements()).toHaveLength(1);
	expect(wide.getByRole('button', { name: 'start docs' }).elements()).toHaveLength(1);
	expect(wide.getByRole('button', { name: 'destroy' }).elements()).toHaveLength(2);
});

test('start posts to the start route for the sandbox it belongs to', async () => {
	const calls = captureFetch();
	const screen = await render(Dashboard);

	await screen.getByRole('button', { name: 'start docs' }).click();

	expect(calls).toHaveLength(1);
	expect(at(calls).url).toBe('/api/sandboxes/work/docs/start');
	expect(at(calls).init?.method).toBe('POST');
});

test('a dismissed confirm destroys nothing', async () => {
	const calls = captureFetch();
	const asked = confirmReturns(false);
	const screen = await render(Dashboard);

	await screen.getByRole('button', { name: 'destroy' }).first().click();

	expect(asked()).toHaveLength(1);
	expect(calls).toHaveLength(0);
});

test('a confirmed destroy sends no force unless the box is ticked', async () => {
	const calls = captureFetch();
	confirmReturns(true);
	const screen = await render(Dashboard);

	await screen.getByRole('button', { name: 'destroy' }).first().click();

	expect(calls).toHaveLength(1);
	expect(at(calls).url).toBe('/api/sandboxes/work/auth');
	expect(at(calls).init?.method).toBe('DELETE');
});

test('ticking force sends force=true and says so in the confirm', async () => {
	const calls = captureFetch();
	const asked = confirmReturns(true);
	const screen = await render(Dashboard);

	await screen.getByRole('checkbox', { name: 'force destroy auth' }).click();
	await screen.getByRole('button', { name: 'destroy' }).first().click();

	expect(calls).toHaveLength(1);
	expect(at(calls).url).toBe('/api/sandboxes/work/auth?force=true');
	// The prompt has to name the cost, not just ask the question again.
	expect(at(asked())).toContain('discarding uncommitted and unmerged work');
});

// The realistic way this UI hurts someone: arm force on one row, then destroy another.
test('force armed on one sandbox never forces a different one', async () => {
	const calls = captureFetch();
	confirmReturns(true);
	const screen = await render(Dashboard);

	await screen.getByRole('checkbox', { name: 'force destroy auth' }).click();
	await expect
		.element(screen.getByRole('checkbox', { name: 'force destroy docs' }))
		.not.toBeChecked();

	await screen.getByRole('button', { name: 'destroy' }).last().click();

	expect(calls).toHaveLength(1);
	expect(at(calls).url).toBe('/api/sandboxes/work/docs');
});

test('force does not stay armed after a destroy', async () => {
	const calls = captureFetch();
	confirmReturns(true);
	const screen = await render(Dashboard);

	await screen.getByRole('checkbox', { name: 'force destroy auth' }).click();
	await screen.getByRole('button', { name: 'destroy' }).first().click();
	await expect
		.element(screen.getByRole('checkbox', { name: 'force destroy auth' }))
		.not.toBeChecked();

	await screen.getByRole('button', { name: 'destroy' }).first().click();

	expect(calls).toHaveLength(2);
	expect(at(calls, 1).url).toBe('/api/sandboxes/work/auth');
});

// The counterpart to the reset above: cancelling answers this destroy, not the intent,
// so the tick survives and the next confirm still names the cost.
test('force stays armed when the confirm is dismissed', async () => {
	const calls = captureFetch();
	confirmReturns(false);
	const screen = await render(Dashboard);

	await screen.getByRole('checkbox', { name: 'force destroy auth' }).click();
	await screen.getByRole('button', { name: 'destroy' }).first().click();

	expect(calls).toHaveLength(0);
	await expect.element(screen.getByRole('checkbox', { name: 'force destroy auth' })).toBeChecked();
});

test('a refusal renders the script stderr unchanged', async () => {
	confirmReturns(true);
	vi.spyOn(window, 'fetch').mockResolvedValue(
		new Response(
			JSON.stringify({ exitCode: 1, stderr: 'error: agent/auth has uncommitted changes' }),
			{ status: 409, headers: { 'content-type': 'application/json' } }
		)
	);
	const screen = await render(Dashboard);

	await screen.getByRole('button', { name: 'destroy' }).first().click();

	await expect.element(screen.getByText('error: agent/auth has uncommitted changes')).toBeVisible();
});

test('keeps the nine-column table on a desktop', async () => {
	await page.viewport(DESKTOP, 768);
	const screen = await render(Dashboard);

	await expect.element(screen.getByRole('table')).toBeVisible();
	const headers = screen.getByRole('table').element().querySelectorAll('thead th');
	expect([...headers].map((th) => th.textContent)).toEqual([
		'Sandbox',
		'Scope',
		'Branch',
		'Agent',
		'Status',
		'Work',
		'Connect',
		'Terminal',
		'Lifecycle'
	]);
});

test('collapses the create form on a phone until it is asked for', async () => {
	await page.viewport(PHONE, 667);
	const screen = await render(Dashboard);

	expect(screen.getByLabelText('sandbox name').elements()).toHaveLength(0);
	const disclose = screen.getByRole('button', { name: 'New sandbox' });
	await expect.element(disclose).toBeVisible();

	await disclose.click();
	await expect.element(screen.getByLabelText('sandbox name')).toBeVisible();
	await expect.element(screen.getByLabelText('repository')).toBeVisible();
});

test('leaves the create form open on a desktop', async () => {
	await page.viewport(DESKTOP, 768);
	const screen = await render(Dashboard);

	await expect.element(screen.getByLabelText('sandbox name')).toBeVisible();
	expect(screen.getByRole('button', { name: 'New sandbox' }).elements()).toHaveLength(0);
});

// The disclosure must not be the only place an empty configuration shows up, or a NAS
// with no repos configured looks like a working dashboard with nothing running.
test('says so when no repositories are configured, without hiding it', async () => {
	await page.viewport(PHONE, 667);
	access.config = { ...access.config, repos: [], scopes: [] };
	const screen = await render(Dashboard);

	await expect
		.element(screen.getByText('No repositories configured', { exact: false }))
		.toBeVisible();
	expect(screen.getByRole('button', { name: 'New sandbox' }).elements()).toHaveLength(0);
});

// An unanswered /api/config looks exactly like an empty one on the wire. Claiming
// nothing is configured before the answer arrives is wrong on every correct server.
test('claims nothing about the configuration until it has arrived', async () => {
	await page.viewport(PHONE, 667);
	access.config = { ...access.config, repos: [], scopes: [] };
	access.loaded = false;
	const screen = await render(Dashboard);

	expect(screen.getByText('No repositories configured', { exact: false }).elements()).toHaveLength(
		0
	);
	expect(screen.getByRole('button', { name: 'New sandbox' }).elements()).toHaveLength(0);
	// The fleet is independent of the configuration and still renders.
	await expect.element(screen.getByText('auth', { exact: true })).toBeVisible();
});

// The arming checkbox for an irreversible action should not move around: before the
// lifecycle controls got a row of their own it wrapped differently depending on whether
// the card carried a connect link, landing beside destroy on some cards and a row above
// it on others.
test('keeps the force checkbox on the destroy row of every card', async () => {
	fleet.snapshot = {
		at: '2026-08-28T10:00:00Z',
		repos: [
			{
				repo: '/repos/sluss',
				name: 'sluss',
				// auth is opencode, running and published, so its card carries a connect
				// link; bare is stopped with no port, so its card has none. Those are the
				// two card shapes, and the wrap used to differ between them.
				sandboxes: [
					running,
					sandbox({ name: 'bare', status: 'stopped', webPort: 0, dirty: false, unmerged: 0 })
				]
			}
		],
		scopeErrors: []
	};
	await page.viewport(PHONE, 800);
	const screen = await render(Dashboard);

	const rows = ['auth', 'bare'].map((name) => {
		const box = screen.getByRole('checkbox', { name: `force destroy ${name}` }).element();
		const card = box.closest('li');
		if (!card) throw new Error(`no card around the ${name} checkbox`);
		const destroy = card.querySelectorAll('button');
		return {
			box: box.getBoundingClientRect(),
			destroy: at(
				[...destroy].filter((b) => b.textContent?.trim() === 'destroy')
			).getBoundingClientRect(),
			card: card.getBoundingClientRect()
		};
	});

	for (const row of rows) {
		// Same row as the button it arms, not the row above it.
		expect(row.box.top).toBeGreaterThanOrEqual(row.destroy.top);
		expect(row.box.bottom).toBeLessThanOrEqual(row.destroy.bottom);
	}

	// And the same place across both card shapes. The tolerance is the width difference
	// between the "stop" and "start" buttons that precede it, which is about a pixel.
	const offset = (row: (typeof rows)[number]) => row.box.left - row.card.left;
	expect(Math.abs(offset(at(rows)) - offset(at(rows, 1)))).toBeLessThan(3);

	expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
});
