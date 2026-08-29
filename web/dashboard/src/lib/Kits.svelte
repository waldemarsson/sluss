<script lang="ts">
	import { onMount } from 'svelte';
	import {
		command,
		request,
		requestText,
		type GitStatus,
		type Kit,
		type KitList
	} from '#lib/api.js';

	// spec.yaml is plain text here and everywhere else in sluss: nothing parses it,
	// nothing formats it, and sbx is the validator. What is typed is what is saved.
	let list = $state<Kit[]>([]);
	let status = $state<GitStatus>({ repository: false, dirty: false, detail: '' });
	let selected = $state('');
	let spec = $state('');
	let error = $state('');
	let busy = $state(false);
	// Only the newest spec load may write to `spec`, so the editor can never show one
	// kit's file while the selector names another.
	let latestSpec = 0;

	onMount(refreshList);

	// Loading the selected kit's spec is a side effect keyed on the selection; the
	// text it writes is never read back here.
	$effect(() => {
		if (selected) loadSpec(selected);
	});

	async function refreshList() {
		error = '';
		const result = await request<KitList>('/api/kits');
		if (!result.ok) {
			error = result.message;
			return;
		}
		list = result.body.kits ?? [];
		status = result.body.status ?? status;
		// `list.length > 0` would not narrow the index access under
		// noUncheckedIndexedAccess; binding the element does.
		const first = list[0];
		if (!selected && first) selected = first.name;
	}

	async function loadSpec(name: string) {
		const ticket = ++latestSpec;
		error = '';
		const result = await requestText(`/api/kits/${name}/spec`);
		if (ticket !== latestSpec) return;
		spec = result.ok ? result.body : '';
		if (!result.ok) error = result.message;
	}

	async function save() {
		busy = true;
		error = '';
		const result = await command(`/api/kits/${selected}/spec`, { method: 'PUT', body: spec });
		busy = false;
		// After the refresh, not before: refreshList clears `error` on entry.
		await refreshList();
		if (!result.ok) error = result.message;
	}
</script>

<section>
	<h2>Kits</h2>

	{#if list.length === 0}
		<p class="muted">No kits in the configured directory.</p>
	{:else}
		<div class="row">
			<select bind:value={selected} aria-label="kit">
				{#each list as kit (kit.name)}
					<option value={kit.name}>{kit.name}{kit.hasSpec ? '' : ' (no spec.yaml)'}</option>
				{/each}
			</select>
			<button onclick={save} disabled={busy || !selected}>save</button>
			<span class="muted">
				{#if !status.repository}
					not a git repository
				{:else if status.dirty}
					<span class="warn">uncommitted changes — commit them by hand</span>
				{:else}
					committed
				{/if}
			</span>
		</div>

		<textarea bind:value={spec} spellcheck="false" aria-label="spec.yaml"></textarea>
	{/if}

	{#if error}
		<p class="error">{error}</p>
	{/if}
</section>

<style>
	.row {
		display: flex;
		gap: 0.5rem;
		align-items: center;
		margin-bottom: 0.5rem;
	}
	textarea {
		width: 100%;
		min-height: 14rem;
		font-family: ui-monospace, SFMono-Regular, monospace;
		font-size: 13px;
		color: #e7e7e7;
		background: #1b1e24;
		border: 1px solid #333842;
		border-radius: 4px;
		padding: 0.5rem;
	}
	@media (max-width: 47.999rem) {
		.row {
			flex-wrap: wrap;
		}
		/* The spec is the whole point of this route, so on a phone it gets the screen.
		   The 16px is app.css's rule restated: a scoped selector carries a .svelte-*
		   class and outranks the bare `textarea` one, so the shared rule cannot reach
		   this editor and Safari would zoom the page when it took focus. */
		textarea {
			min-height: 55vh;
			font-size: 16px;
		}
	}
</style>
