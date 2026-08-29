<script lang="ts">
	export interface Link {
		href: string;
		label: string;
	}

	interface Props {
		links: Link[];
		/** the current path, so the active link can be marked */
		pathname: string;
	}

	let { links, pathname }: Props = $props();

	// The longest href that prefixes the current path wins. That marks Dashboard ("/")
	// only on the dashboard, because "/config/" is the longer prefix everywhere under
	// it, and it needs no per-link exact/prefix flag to say so.
	let current = $derived(
		links
			.filter((link) => pathname.startsWith(link.href))
			.reduce<Link | null>(
				(best, link) => (link.href.length > (best?.href.length ?? -1) ? link : best),
				null
			)
	);
</script>

<nav>
	{#each links as link (link.href)}
		<a href={link.href} aria-current={link === current ? 'page' : undefined}>{link.label}</a>
	{/each}
</nav>

<style>
	nav {
		display: flex;
		flex-wrap: wrap;
		gap: 1rem;
	}
	a {
		color: #8b8f98;
		text-decoration: none;
		border-bottom: 2px solid transparent;
	}
	a[aria-current='page'] {
		color: #e7e7e7;
		border-bottom-color: #6cb6ff;
	}
</style>
