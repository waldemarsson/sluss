<script lang="ts">
	import NavLinks from '#lib/NavLinks.svelte';

	interface Props {
		/** the current path, so the active section can be marked */
		pathname: string;
		/** whether the fleet stream is delivering */
		connected: boolean;
	}

	// Props rather than reads of `page` and `fleet`, so the bar renders on its own. The
	// root layout is what wires it to the session's state.
	let { pathname, connected }: Props = $props();

	const links = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/config/', label: 'Configuration' }
	];
</script>

<header>
	<h1>sluss</h1>
	<NavLinks {links} {pathname} />
	<span class="status" class:live={connected}>{connected ? 'live' : 'reconnecting'}</span>
</header>

<style>
	header {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.75rem 1.25rem;
		margin-bottom: 1.5rem;
	}
	/* Pushed to the far end of a wide header; wraps onto its own line on a narrow one. */
	.status {
		margin-left: auto;
		color: #8b8f98;
	}
	.status.live {
		color: #7bc47f;
	}
</style>
