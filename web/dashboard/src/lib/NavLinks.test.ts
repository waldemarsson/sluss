import { render } from 'vitest-browser-svelte';
import { expect, test } from 'vitest';
import NavLinks from './NavLinks.svelte';

const top = [
	{ href: '/', label: 'Dashboard' },
	{ href: '/config/', label: 'Configuration' }
];

test('marks the dashboard on the dashboard', async () => {
	const screen = await render(NavLinks, { links: top, pathname: '/' });

	await expect.element(screen.getByRole('link', { name: 'Dashboard' })).toHaveAttribute(
		'aria-current',
		'page'
	);
	expect(
		screen.getByRole('link', { name: 'Configuration' }).element().getAttribute('aria-current')
	).toBe(null);
});

// "/" prefixes every path, so only the longest match can decide this one.
test('marks configuration anywhere under it', async () => {
	const screen = await render(NavLinks, { links: top, pathname: '/config/kits/' });

	await expect.element(screen.getByRole('link', { name: 'Configuration' })).toHaveAttribute(
		'aria-current',
		'page'
	);
	expect(
		screen.getByRole('link', { name: 'Dashboard' }).element().getAttribute('aria-current')
	).toBe(null);
});

test('marks the current area in the configuration sub-navigation', async () => {
	const screen = await render(NavLinks, {
		links: [
			{ href: '/config/secrets/', label: 'Secrets' },
			{ href: '/config/kits/', label: 'Kits' }
		],
		pathname: '/config/kits/'
	});

	await expect.element(screen.getByRole('link', { name: 'Kits' })).toHaveAttribute(
		'aria-current',
		'page'
	);
	expect(screen.getByRole('link', { name: 'Secrets' }).element().getAttribute('aria-current')).toBe(
		null
	);
});
