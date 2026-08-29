import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import SecretsPage from './secrets/+page.svelte';
import KitsPage from './kits/+page.svelte';
import { access } from '#lib/access.svelte.js';
import '../../app.css';

const PHONE = 375;

beforeEach(() => {
	access.config = {
		access: 'path',
		hostPrefix: 'sluss-',
		domain: '',
		repos: ['/repos/sluss'],
		scopes: ['work']
	};
	access.loaded = true;
	// Both areas load themselves on mount; neither route test is about what comes back.
	vi.stubGlobal(
		'fetch',
		vi.fn(async (input: string) =>
			input.startsWith('/api/kits')
				? new Response(
						JSON.stringify({
							kits: [{ name: 'base', hasSpec: true }],
							status: { repository: true, dirty: false, detail: '' }
						}),
						{ status: 200 }
					)
				: new Response(JSON.stringify({ names: ['ANTHROPIC_API_KEY'] }), { status: 200 })
		)
	);
});

afterEach(async () => {
	vi.unstubAllGlobals();
	await page.viewport(1024, 768);
});

test('the secrets route carries the secrets area and nothing else', async () => {
	await page.viewport(PHONE, 667);
	const screen = await render(SecretsPage);

	await expect.element(screen.getByLabelText('secret name')).toBeVisible();
	await expect.element(screen.getByLabelText('secret scope')).toBeVisible();
	expect(screen.getByLabelText('spec.yaml').elements()).toHaveLength(0);

	expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
});

test('the kits route carries the kits area and nothing else', async () => {
	await page.viewport(PHONE, 667);
	const screen = await render(KitsPage);

	await expect.element(screen.getByLabelText('spec.yaml')).toBeVisible();
	expect(screen.getByLabelText('secret name').elements()).toHaveLength(0);

	const editor = screen.getByLabelText('spec.yaml').element();
	expect(editor.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
	// Under 16px, Safari zooms the page when the editor takes focus and leaves it
	// zoomed. The component's own scoped rule outranks the shared one, so this is the
	// only place that can be asserted.
	expect(parseFloat(getComputedStyle(editor).fontSize)).toBeGreaterThanOrEqual(16);
	expect(screen.getByRole('button', { name: 'save' }).element().getBoundingClientRect().height)
		.toBeGreaterThanOrEqual(44);

	expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(window.innerWidth);
});
