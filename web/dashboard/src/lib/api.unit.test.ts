import { afterEach, expect, test, vi } from 'vitest';
import { command, request, requestText } from './api.js';

afterEach(() => vi.unstubAllGlobals());

const answers = (body: BodyInit | null, init?: ResponseInit) =>
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => new Response(body, init))
	);

test('reads a JSON body on success', async () => {
	answers(JSON.stringify({ names: ['GITHUB_TOKEN'] }), { status: 200 });

	const result = await request<{ names: string[] }>('/api/secrets/work');

	expect(result).toEqual({ ok: true, body: { names: ['GITHUB_TOKEN'] } });
});

test('reads a text body on success', async () => {
	answers('name: go-web', { status: 200 });

	const result = await requestText('/api/kits/go-web/spec');

	expect(result).toEqual({ ok: true, body: 'name: go-web' });
});

// The 204 case: parsing a body that is not there would turn every successful save into
// an error, which is why a mutation never reads one.
test('treats an empty 204 as success', async () => {
	answers(null, { status: 204 });

	const result = await command('/api/secrets/work/GITHUB_TOKEN', { method: 'DELETE' });

	expect(result.ok).toBe(true);
});

// The whole point of the shared rule: a lifecycle refusal says more in its
// stderr than the generic message wrapped around it, and every route now shows it.
test('prefers the refusal stderr over the generic error', async () => {
	answers(JSON.stringify({ error: 'sluss refused', stderr: '  refusing: 2 unmerged commits\n' }), {
		status: 409
	});

	const result = await command('/api/sandboxes/work/auth', { method: 'DELETE' });

	expect(result).toEqual({ ok: false, message: 'refusing: 2 unmerged commits' });
});

test('falls back to the error field when there is no stderr', async () => {
	answers(JSON.stringify({ error: 'no such scope' }), { status: 404 });

	const result = await request('/api/secrets/gone');

	expect(result).toEqual({ ok: false, message: 'no such scope' });
});

test('falls back to the status when the body is not JSON', async () => {
	answers('<html>502</html>', { status: 502 });

	const result = await request('/api/kits');

	expect(result).toEqual({ ok: false, message: 'failed with 502' });
});

// An unreachable sluss reaches the user as a line like any other, so no component
// carries a try/catch of its own.
test('turns a thrown fetch into a message', async () => {
	vi.stubGlobal(
		'fetch',
		vi.fn(async () => Promise.reject(new Error('offline')))
	);

	const result = await request('/api/kits');

	expect(result.ok).toBe(false);
	expect(result.ok === false && result.message).toContain('offline');
});
