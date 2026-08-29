<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import Nav from '#lib/Nav.svelte';
	import { access } from '#lib/access.svelte.js';
	import { fleet } from '#lib/fleet.svelte.js';
	import '../app.css';

	let { children }: { children: Snippet } = $props();

	// One stream and one configuration fetch for the whole session. Both are idempotent
	// and neither is torn down: this layout outlives every route, so navigating between
	// them never drops the fleet.
	onMount(() => {
		fleet.connect();
		access.load();
	});
</script>

<Nav pathname={page.url.pathname} connected={fleet.connected} />

{@render children()}
