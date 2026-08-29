<script lang="ts">
	import { command, request, type SecretList } from '#lib/api.js';

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
	// Only the newest request may write to `names`. Without this a slow response for the
	// scope just left lands after the fresh one and renders its names under the current
	// scope's label — the one thing a per-scope secret list must never do.
	let latest = 0;

	// Loading the list is a genuine side effect — it reads `scope` and talks to the
	// server. Nothing it writes is read back here, so it cannot loop.
	$effect(() => {
		if (scope) refresh(scope);
	});

	async function refresh(forScope: string) {
		const ticket = ++latest;
		error = '';
		const result = await request<SecretList>(`/api/secrets/${forScope}`);
		if (ticket !== latest) return;
		if (!result.ok) {
			error = result.message;
			names = [];
			return;
		}
		names = result.body.names ?? [];
	}

	async function send(method: string, name: string, value: string | undefined) {
		busy = true;
		error = '';
		const result = await command(`/api/secrets/${scope}/${name}`, { method, body: value });
		busy = false;
		// Refreshed even after a failure: the list on screen is the only evidence of what
		// the scope actually holds now. The refusal is reported after that refresh, not
		// before it — `refresh` clears `error` on entry, so a message set here first would
		// be wiped before it ever rendered.
		await refresh(scope);
		if (!result.ok) error = result.message;
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
