<script>
	import { onMount } from 'svelte';
	import Secrets from '$lib/Secrets.svelte';
	import Kits from '$lib/Kits.svelte';

	let snapshot = $state(null);
	let access = $state({ access: 'path', hostPrefix: 'sluss-', domain: '', repos: [], scopes: [] });
	let connected = $state(false);
	let copied = $state('');
	let busy = $state('');
	let problem = $state('');
	let form = $state({ repo: '', scope: '', name: '', agent: 'opencode' });

	let repos = $derived(snapshot?.repos ?? []);
	let scopeErrors = $derived(snapshot?.scopeErrors ?? []);

	onMount(() => {
		fetch('/api/config')
			.then((r) => r.json())
			.then((c) => {
				access = c;
				form.repo = c.repos?.[0] ?? '';
				form.scope = c.scopes?.[0] ?? '';
			})
			.catch(() => {});

		// The whole snapshot arrives on every poll; there is no client-side state to
		// reconcile because the server holds none either.
		const events = new EventSource('/api/events');
		events.addEventListener('fleet', (event) => {
			snapshot = JSON.parse(event.data);
			connected = true;
		});
		events.onerror = () => (connected = false);
		return () => events.close();
	});

	// OpenCode is reached through sluss. Claude Code and Copilot have their own
	// remote interfaces, so they get deep links and are never proxied.
	function connectTo(sandbox) {
		if (sandbox.agent === 'claude') return { href: 'https://claude.ai/code', label: 'claude.ai' };
		if (sandbox.agent === 'copilot') return { href: 'https://github.com/copilot', label: 'github.com' };
		if (sandbox.agent !== 'opencode' || sandbox.status !== 'running' || !sandbox.webPort) return null;
		if (access.access === 'host') {
			return {
				href: `${location.protocol}//${access.hostPrefix}${sandbox.name}.${access.domain}/`,
				label: 'open'
			};
		}
		return { href: `/s/${sandbox.scope}/${sandbox.name}/`, label: 'open' };
	}

	// Every mutation runs scripts/sluss on the server. A refusal comes back as 409
	// with the script's own message, which is shown unchanged.
	async function act(label, request) {
		busy = label;
		problem = '';
		try {
			const response = await request();
			if (!response.ok) {
				const body = await response.json().catch(() => ({}));
				problem = (body.stderr || body.error || `failed with ${response.status}`).trim();
			}
		} catch (error) {
			problem = String(error);
		} finally {
			busy = '';
		}
	}

	function create(event) {
		event.preventDefault();
		return act('create', () =>
			fetch('/api/sandboxes', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify(form)
			})
		);
	}

	function stop(sandbox) {
		return act(`${sandbox.scope}/${sandbox.name}`, () =>
			fetch(`/api/sandboxes/${sandbox.scope}/${sandbox.name}/stop`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: '{}'
			})
		);
	}

	function destroy(sandbox) {
		return act(`${sandbox.scope}/${sandbox.name}`, () =>
			fetch(`/api/sandboxes/${sandbox.scope}/${sandbox.name}`, { method: 'DELETE' })
		);
	}

	async function copyAttach(name) {
		const command = `sluss attach ${name}`;
		try {
			await navigator.clipboard.writeText(command);
			copied = name;
			setTimeout(() => {
				if (copied === name) copied = '';
			}, 1500);
		} catch {
			copied = '';
			window.prompt('Copy this command', command);
		}
	}
</script>

<header>
	<h1>sluss</h1>
	<span class="status" class:live={connected}>{connected ? 'live' : 'reconnecting'}</span>
</header>

<form onsubmit={create}>
	<select bind:value={form.repo} aria-label="repository">
		{#each access.repos ?? [] as repo (repo)}
			<option value={repo}>{repo}</option>
		{/each}
	</select>
	<select bind:value={form.scope} aria-label="scope">
		{#each access.scopes ?? [] as scope (scope)}
			<option value={scope}>{scope}</option>
		{/each}
	</select>
	<input bind:value={form.name} placeholder="name" aria-label="sandbox name" />
	<select bind:value={form.agent} aria-label="agent">
		<option value="opencode">opencode</option>
		<option value="claude">claude</option>
		<option value="copilot">copilot</option>
	</select>
	<button type="submit" disabled={busy !== '' || !form.name}>
		{busy === 'create' ? 'starting…' : 'start'}
	</button>
</form>

{#if problem}
	<pre class="error">{problem}</pre>
{/if}

{#each scopeErrors as scopeError (scopeError.scope + scopeError.error)}
	<p class="error"><strong>{scopeError.scope}</strong>: {scopeError.error}</p>
{/each}

{#if !snapshot}
	<p class="muted">Waiting for the first poll…</p>
{:else if repos.length === 0}
	<p class="muted">No sandboxes in any configured scope.</p>
{/if}

{#each repos as repo (repo.repo)}
	<section>
		<h2>{repo.name}<span class="muted"> · {repo.repo}</span></h2>
		<table>
			<thead>
				<tr>
					<th>Sandbox</th>
					<th>Scope</th>
					<th>Branch</th>
					<th>Agent</th>
					<th>Status</th>
					<th>Work</th>
					<th>Connect</th>
					<th>Terminal</th>
					<th>Lifecycle</th>
				</tr>
			</thead>
			<tbody>
				{#each repo.sandboxes as sandbox (sandbox.scope + '/' + sandbox.name)}
					{@const connect = connectTo(sandbox)}
					<tr>
						<td>{sandbox.name}</td>
						<td class="muted">{sandbox.scope}</td>
						<td>
							{#if sandbox.missing}
								<span class="warn">worktree missing</span>
							{:else}
								{sandbox.branch}
							{/if}
						</td>
						<td>{sandbox.agent}</td>
						<td class:running={sandbox.status === 'running'}>{sandbox.status}</td>
						<td>
							{#if sandbox.dirty}<span class="warn">dirty</span>{/if}
							{#if sandbox.unmerged > 0}<span class="warn">{sandbox.unmerged} unmerged</span>{/if}
							{#if !sandbox.dirty && sandbox.unmerged === 0 && !sandbox.missing}
								<span class="muted">clean</span>
							{/if}
						</td>
						<td>
							{#if connect}
								<a href={connect.href} target="_blank" rel="noreferrer">{connect.label}</a>
							{:else}
								<span class="muted">—</span>
							{/if}
						</td>
						<td>
							<button onclick={() => copyAttach(sandbox.name)}>
								{copied === sandbox.name ? 'copied' : 'sluss attach'}
							</button>
						</td>
						<td>
							{#if sandbox.status === 'running'}
								<button onclick={() => stop(sandbox)} disabled={busy !== ''}>stop</button>
							{/if}
							<button onclick={() => destroy(sandbox)} disabled={busy !== ''}>destroy</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</section>
{/each}

<Secrets scopes={access.scopes ?? []} />
<Kits />

<style>
	:global(body) {
		margin: 0;
		padding: 1.5rem;
		font: 14px/1.5 ui-sans-serif, system-ui, sans-serif;
		color: #e7e7e7;
		background: #16181d;
	}
	header {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		margin-bottom: 1.5rem;
	}
	h1 {
		font-size: 1.1rem;
		margin: 0;
	}
	h2 {
		font-size: 0.95rem;
		font-weight: 600;
		margin: 1.5rem 0 0.5rem;
	}
	table {
		width: 100%;
		border-collapse: collapse;
	}
	th {
		text-align: left;
		font-weight: 500;
		color: #8b8f98;
		border-bottom: 1px solid #2a2d34;
		padding: 0.35rem 0.6rem 0.35rem 0;
	}
	td {
		padding: 0.4rem 0.6rem 0.4rem 0;
		border-bottom: 1px solid #21242a;
	}
	.muted {
		color: #8b8f98;
	}
	.warn {
		color: #e0a458;
		margin-right: 0.5rem;
	}
	.running {
		color: #7bc47f;
	}
	.status {
		color: #8b8f98;
	}
	.status.live {
		color: #7bc47f;
	}
	.error {
		color: #e06c75;
		white-space: pre-wrap;
		font: inherit;
	}
	form {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}
	select,
	input {
		font: inherit;
		color: #e7e7e7;
		background: #23262d;
		border: 1px solid #333842;
		border-radius: 4px;
		padding: 0.15rem 0.4rem;
	}
	a {
		color: #6cb6ff;
	}
	button {
		font: inherit;
		color: #e7e7e7;
		background: #23262d;
		border: 1px solid #333842;
		border-radius: 4px;
		padding: 0.15rem 0.5rem;
		cursor: pointer;
	}
</style>
