<script lang="ts">
	import { page } from '$app/state';
	import type { Snippet } from 'svelte';
	import NavLinks from '#lib/NavLinks.svelte';

	let { children }: { children: Snippet } = $props();

	const links = [
		{ href: '/config/secrets/', label: 'Secrets' },
		{ href: '/config/kits/', label: 'Kits' }
	];
</script>

<NavLinks {links} pathname={page.url.pathname} />

{@render children()}
