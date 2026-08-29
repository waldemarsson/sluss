<script lang="ts">
	import { onMount } from 'svelte';
	import type { ApiError, Sandbox } from '#lib/api.js';
	import { access } from '#lib/access.svelte.js';
	import { fleet } from '#lib/fleet.svelte.js';

	// One breakpoint, shared with the media queries below, so the table/card switch and
	// the responsive styles can never disagree about what "narrow" means.
	const WIDE = '(min-width: 48rem)';

	// Read before the first render, not in onMount: ssr is off, so `window` is already
	// here and deferring it would paint the narrow layout once on every desktop load.
	let wide = $state(window.matchMedia(WIDE).matches);
	let opened = $state(false);
	let copied = $state('');
	let busy = $state('');
	// Which sandbox has force armed, as "scope/name". One string rather than a flag per
	// row, so arming a second sandbox disarms the first by construction: a checkbox left
	// ticked on one row can never force the destroy of another.
	let forcing = $state('');
	let problem = $state('');
	let chosen = $state({ repo: '', scope: '', name: '', agent: 'opencode' });

	let repos = $derived(fleet.snapshot?.repos ?? []);
	let scopeErrors = $derived(fleet.snapshot?.scopeErrors ?? []);
	// Empty means "whichever is first", the same shape Secrets.svelte uses for scopes:
	// the configuration arrives after the first render, so nothing can be assigned into
	// the form up front.
	let repo = $derived(chosen.repo || access.config.repos?.[0] || '');
	let scope = $derived(chosen.scope || access.config.scopes?.[0] || '');
	// On a phone the four controls fill the screen before a single sandbox appears, so
	// they collapse; on a desktop there is room and the disclosure would only be a click
	// in the way.
	let formVisible = $derived(wide || opened);
	let configured = $derived((access.config.repos?.length ?? 0) > 0);

	onMount(() => {
		const query = window.matchMedia(WIDE);
		const sync = () => (wide = query.matches);
		query.addEventListener('change', sync);
		return () => query.removeEventListener('change', sync);
	});

	// OpenCode is reached through sluss. Claude Code and Copilot have their own
	// remote interfaces, so they get deep links and are never proxied.
	function connectTo(sandbox: Sandbox): { href: string; label: string } | null {
		if (sandbox.agent === 'claude') return { href: 'https://claude.ai/code', label: 'claude.ai' };
		if (sandbox.agent === 'copilot')
			return { href: 'https://github.com/copilot', label: 'github.com' };
		if (sandbox.agent !== 'opencode' || sandbox.status !== 'running' || !sandbox.webPort)
			return null;
		if (access.config.access === 'host') {
			return {
				href: `${location.protocol}//${access.config.hostPrefix}${sandbox.name}.${access.config.domain}/`,
				label: 'open'
			};
		}
		return { href: `/s/${sandbox.scope}/${sandbox.name}/`, label: 'open' };
	}

	// Every mutation runs scripts/sluss on the server. A refusal comes back as 409
	// with the script's own message, which is shown unchanged.
	async function act(label: string, request: () => Promise<Response>) {
		busy = label;
		problem = '';
		try {
			const response = await request();
			if (!response.ok) {
				const body: ApiError = await response.json().catch(() => ({}));
				problem = (body.stderr || body.error || `failed with ${response.status}`).trim();
			}
		} catch (error) {
			problem = String(error);
		} finally {
			busy = '';
		}
	}

	function create(event: SubmitEvent) {
		event.preventDefault();
		return act('create', () =>
			fetch('/api/sandboxes', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ repo, scope, name: chosen.name, agent: chosen.agent })
			})
		);
	}

	const keyOf = (sandbox: Sandbox) => `${sandbox.scope}/${sandbox.name}`;

	function start(sandbox: Sandbox) {
		return act(keyOf(sandbox), () =>
			fetch(`/api/sandboxes/${sandbox.scope}/${sandbox.name}/start`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: '{}'
			})
		);
	}

	function stop(sandbox: Sandbox) {
		return act(keyOf(sandbox), () =>
			fetch(`/api/sandboxes/${sandbox.scope}/${sandbox.name}/stop`, {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: '{}'
			})
		);
	}

	// Destroy is the one irreversible action the dashboard can take, and on a phone it
	// is a mis-tap away, so it always passes a confirm. Force is opt-in per sandbox and
	// says in the prompt what it discards. A destroy that runs disarms the checkbox, so
	// force cannot carry over to the next one; a dismissed confirm leaves it armed,
	// because cancelling the dialog answers this destroy, not the intent behind it, and
	// the next confirm still names the cost.
	async function destroy(sandbox: Sandbox) {
		const key = keyOf(sandbox);
		const force = forcing === key;
		const warning = force
			? `Destroy ${sandbox.name}, discarding uncommitted and unmerged work? This cannot be undone.`
			: `Destroy ${sandbox.name}?`;
		if (!window.confirm(warning)) return;
		await act(key, () =>
			fetch(`/api/sandboxes/${sandbox.scope}/${sandbox.name}${force ? '?force=true' : ''}`, {
				method: 'DELETE'
			})
		);
		forcing = '';
	}

	async function copyAttach(name: string) {
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

{#if !access.loaded}
	<!-- The configuration is one request away; saying anything here would only be
	     replaced a moment later. -->
{:else if !configured}
	<p class="muted">
		No repositories configured — see the <code>repos</code> key in the config file.
	</p>
{:else if !formVisible}
	<button class="disclose" onclick={() => (opened = true)}>New sandbox</button>
{/if}

{#if access.loaded && configured && formVisible}
	<form onsubmit={create}>
		<select
			value={repo}
			onchange={(event) => (chosen.repo = event.currentTarget.value)}
			aria-label="repository"
		>
			{#each access.config.repos ?? [] as option (option)}
				<option value={option}>{option}</option>
			{/each}
		</select>
		<select
			value={scope}
			onchange={(event) => (chosen.scope = event.currentTarget.value)}
			aria-label="scope"
		>
			{#each access.config.scopes ?? [] as option (option)}
				<option value={option}>{option}</option>
			{/each}
		</select>
		<input bind:value={chosen.name} placeholder="name" aria-label="sandbox name" />
		<select bind:value={chosen.agent} aria-label="agent">
			<option value="opencode">opencode</option>
			<option value="claude">claude</option>
			<option value="copilot">copilot</option>
		</select>
		<button type="submit" disabled={busy !== '' || !chosen.name}>
			{busy === 'create' ? 'starting…' : 'start'}
		</button>
	</form>
{/if}

{#if problem}
	<pre class="error">{problem}</pre>
{/if}

{#each scopeErrors as scopeError (scopeError.scope + scopeError.error)}
	<p class="error"><strong>{scopeError.scope}</strong>: {scopeError.error}</p>
{/each}

{#if !fleet.snapshot}
	<p class="muted">Waiting for the first poll…</p>
{:else if repos.length === 0}
	<p class="muted">No sandboxes in any configured scope.</p>
{/if}

{#each repos as group (group.repo)}
	<section>
		<h2>{group.name}<span class="muted"> · {group.repo}</span></h2>

		{#if wide}
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
					{#each group.sandboxes as sandbox (sandbox.scope + '/' + sandbox.name)}
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
								{:else}
									<button
										aria-label="start {sandbox.name}"
										onclick={() => start(sandbox)}
										disabled={busy !== ''}>start</button
									>
								{/if}
								<label class="force">
									<input
										type="checkbox"
										aria-label="force destroy {sandbox.name}"
										disabled={busy !== ''}
										checked={forcing === keyOf(sandbox)}
										onchange={(event) =>
											(forcing = event.currentTarget.checked ? keyOf(sandbox) : '')}
									/>
									<span>force</span>
								</label>
								<button onclick={() => destroy(sandbox)} disabled={busy !== ''}>destroy</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{:else}
			<!-- The same rows as a stack of cards. Rendered instead of the table, never
			     alongside it, so there is one of everything in the DOM. -->
			<ul class="cards">
				{#each group.sandboxes as sandbox (sandbox.scope + '/' + sandbox.name)}
					{@const connect = connectTo(sandbox)}
					<li>
						<div class="head">
							<span class="name">{sandbox.name}</span>
							<span class:running={sandbox.status === 'running'}>{sandbox.status}</span>
						</div>
						<p class="meta">
							<span class="muted">{sandbox.scope}</span> ·
							{#if sandbox.missing}
								<span class="warn">worktree missing</span>
							{:else}
								{sandbox.branch}
							{/if}
							· {sandbox.agent}
						</p>
						<p class="work">
							{#if sandbox.dirty}<span class="warn">dirty</span>{/if}
							{#if sandbox.unmerged > 0}<span class="warn">{sandbox.unmerged} unmerged</span>{/if}
							{#if !sandbox.dirty && sandbox.unmerged === 0 && !sandbox.missing}
								<span class="muted">clean</span>
							{/if}
						</p>
						<div class="actions">
							{#if connect}
								<a href={connect.href} target="_blank" rel="noreferrer">{connect.label}</a>
							{/if}
							<button onclick={() => copyAttach(sandbox.name)}>
								{copied === sandbox.name ? 'copied' : 'sluss attach'}
							</button>
							{#if sandbox.status === 'running'}
								<button onclick={() => stop(sandbox)} disabled={busy !== ''}>stop</button>
							{:else}
								<button
									aria-label="start {sandbox.name}"
									onclick={() => start(sandbox)}
									disabled={busy !== ''}>start</button
								>
							{/if}
							<label class="force">
								<input
									type="checkbox"
									aria-label="force destroy {sandbox.name}"
									disabled={busy !== ''}
									checked={forcing === keyOf(sandbox)}
									onchange={(event) =>
										(forcing = event.currentTarget.checked ? keyOf(sandbox) : '')}
								/>
								<span>force</span>
							</label>
							<button onclick={() => destroy(sandbox)} disabled={busy !== ''}>destroy</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
{/each}

<style>
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
	.running {
		color: #7bc47f;
	}
	form {
		display: flex;
		gap: 0.5rem;
		flex-wrap: wrap;
		margin-bottom: 1rem;
	}
	.disclose {
		margin-bottom: 1rem;
	}
	.cards {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	.cards li {
		border: 1px solid #2a2d34;
		border-radius: 6px;
		padding: 0.75rem;
	}
	.head {
		display: flex;
		justify-content: space-between;
		gap: 0.75rem;
	}
	.name {
		font-weight: 600;
		/* Sandbox names, branches and repo paths are long and unspaced; without this a
		   single one widens the card past the viewport. */
		overflow-wrap: anywhere;
	}
	.meta,
	.work {
		margin: 0.25rem 0 0;
		overflow-wrap: anywhere;
	}
	.work:empty {
		display: none;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-top: 0.75rem;
	}
	/* Every control in a card is a thumb target, including the anchor, which is why it
	   is boxed like the buttons beside it. */
	.actions > * {
		flex: 1 1 auto;
		min-height: 44px;
		display: flex;
		align-items: center;
		justify-content: center;
		text-align: center;
	}
	.actions a {
		border: 1px solid #333842;
		border-radius: 4px;
		text-decoration: none;
	}
	/* The checkbox and its word are one target: in the table they sit inline with the
	   buttons, and in a card .actions > * already makes the label a thumb-sized box. */
	.force {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		white-space: nowrap;
	}
	.force input {
		/* A default checkbox is a pointer target; this is the smallest square that is
		   still a comfortable one on a phone. */
		width: 20px;
		height: 20px;
		margin: 0;
	}
</style>
