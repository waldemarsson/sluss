import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { afterEach, beforeEach, expect, test } from 'vitest';
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
});

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
	expect(
		getComputedStyle(screen.getByText('dirty').element()).marginRight
	).not.toBe('0px');

	expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);

	// Thumb targets, not pointer targets.
	for (const control of screen.getByRole('button').elements()) {
		expect(control.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
	}
	expect(
		screen.getByRole('link', { name: 'open' }).element().getBoundingClientRect().height
	).toBeGreaterThanOrEqual(44);
});

test('offers stop only for a running sandbox, in either rendering', async () => {
	await page.viewport(PHONE, 667);
	const narrow = await render(Dashboard);
	expect(narrow.getByRole('button', { name: 'stop' }).elements()).toHaveLength(1);
	expect(narrow.getByRole('button', { name: 'destroy' }).elements()).toHaveLength(2);
	narrow.unmount();

	await page.viewport(DESKTOP, 768);
	const wide = await render(Dashboard);
	expect(wide.getByRole('button', { name: 'stop' }).elements()).toHaveLength(1);
	expect(wide.getByRole('button', { name: 'destroy' }).elements()).toHaveLength(2);
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

	await expect.element(screen.getByText('No repositories configured', { exact: false })).toBeVisible();
	expect(screen.getByRole('button', { name: 'New sandbox' }).elements()).toHaveLength(0);
});

// An unanswered /api/config looks exactly like an empty one on the wire. Claiming
// nothing is configured before the answer arrives is wrong on every correct server.
test('claims nothing about the configuration until it has arrived', async () => {
	await page.viewport(PHONE, 667);
	access.config = { ...access.config, repos: [], scopes: [] };
	access.loaded = false;
	const screen = await render(Dashboard);

	expect(screen.getByText('No repositories configured', { exact: false }).elements()).toHaveLength(0);
	expect(screen.getByRole('button', { name: 'New sandbox' }).elements()).toHaveLength(0);
	// The fleet is independent of the configuration and still renders.
	await expect.element(screen.getByText('auth', { exact: true })).toBeVisible();
});
