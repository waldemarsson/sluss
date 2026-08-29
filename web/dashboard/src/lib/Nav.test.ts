import { render } from 'vitest-browser-svelte';
import { expect, test } from 'vitest';
import Nav from './Nav.svelte';

test('reports a delivering stream as live', async () => {
	const screen = await render(Nav, { pathname: '/', connected: true });

	await expect.element(screen.getByText('live')).toBeVisible();
});

test('reports a dropped stream as reconnecting', async () => {
	const screen = await render(Nav, { pathname: '/config/kits/', connected: false });

	await expect.element(screen.getByText('reconnecting')).toBeVisible();
});
