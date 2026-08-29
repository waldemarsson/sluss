<script lang="ts">
	import type { ApiError, SecretList } from '#lib/api.js';

	interface Props {
		scopes?: string[];
	}

	// Write-only by design: names are listed, values are sent and never read back.
	// Nothing here ever populates a field with an existing value, because sluss
	// cannot see one.
	let { scopes = [] }: Props = $props();

	// Empty means "whichever scope is first"; picking one in the select fills it in.
	let chosen = $state('');
	let scope = $derived(chosen || scopes[0] || '');
	let names = $state<string[]>([]);
	let error = $state('');
	let busy = $state(false);
	let draft = $state({ name: '', value: '' });

	// Loading the list is a genuine side effect — it reads `scope` and talks to the
	// server. Nothing it writes is read back here, so it cannot loop.
	$effect(() => {
		if (scope) refresh(scope);
	});

	async function refresh(forScope: string) {
		error = '';
		try {
			const response = await fetch(`/api/secrets/${forScope}`);
			const body: SecretList & ApiError = await response.json();
			if (!response.ok) {
				error = body.error ?? `failed with ${response.status}`;
				names = [];
				return;
			}
			names = body.names ?? [];
		} catch (problem) {
			error = String(problem);
		}
	}

	async function send(method: string, name: string, value: string | undefined) {
		busy = true;
		error = '';
		try {
			const response = await fetch(`/api/secrets/${scope}/${name}`, { method, body: value });
			if (!response.ok) {
				const body: ApiError = await response.json().catch(() => ({}));
				error = body.error ?? `failed with ${response.status}`;
			}
		} catch (problem) {
			error = String(problem);
		} finally {
			busy = false;
			await refresh(scope);
		}
	}

	async function set(event: SubmitEvent) {
		event.preventDefault();
		await send('PUT', draft.name, draft.value);
		// The value leaves the browser and is not kept anywhere on this page.
		draft = { name: '', value: '' };
	}
</script>

<section>
	<h2>Secrets</h2>

	<form onsubmit={set}>
		<select
			value={scope}
			onchange={(event) => (chosen = event.currentTarget.value)}
			aria-label="secret scope"
		>
			{#each scopes as option (option)}
				<option value={option}>{option}</option>
			{/each}
		</select>
		<input bind:value={draft.name} placeholder="NAME" aria-label="secret name" />
		<input
			bind:value={draft.value}
			type="password"
			placeholder="value"
			aria-label="secret value"
			autocomplete="off"
		/>
		<button type="submit" disabled={busy || !draft.name || !draft.value}>set</button>
	</form>

	{#if error}
		<p class="error">{error}</p>
	{/if}

	{#if names.length === 0}
		<p class="muted">No secrets in this scope.</p>
	{:else}
		<ul>
			{#each names as name (name)}
				<li>
					<code>{name}</code>
					<button onclick={() => send('DELETE', name, undefined)} disabled={busy}>delete</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	li {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.2rem 0;
	}
	form {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 0.75rem;
	}
	code {
		overflow-wrap: anywhere;
	}
	/* Side by side there is no room for a scope, a name and a value on a phone, and a
	   secret value is the last field anyone wants to type into a squeezed box. */
	@media (max-width: 47.999rem) {
		form {
			flex-direction: column;
			align-items: stretch;
		}
		li {
			gap: 0.75rem;
		}
	}
</style>
