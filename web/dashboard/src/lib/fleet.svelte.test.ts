import { expect, test, vi } from 'vitest';
import { Fleet } from './fleet.svelte.js';
import type { Snapshot } from './api.js';

// EventSource has no test double in the browser, and the real one would need a server.
// This records what was opened so the "connect twice" case can be asserted at all.
class FakeEventSource {
	static opened: FakeEventSource[] = [];
	listeners = new Map<string, (event: MessageEvent) => void>();
	onerror: (() => void) | null = null;

	constructor(public url: string) {
		FakeEventSource.opened.push(this);
	}

	addEventListener(type: string, listener: (event: MessageEvent) => void) {
		this.listeners.set(type, listener);
	}

	deliver(type: string, data: unknown) {
		this.listeners.get(type)?.(new MessageEvent(type, { data: JSON.stringify(data) }));
	}

	fail() {
		this.onerror?.();
	}
}

const snapshot: Snapshot = {
	at: '2026-08-28T10:00:00Z',
	repos: [{ repo: '/repos/sluss', name: 'sluss', sandboxes: [] }],
	scopeErrors: []
};

// One walk through the stream's whole life. Split into separate tests these would share
// a connection through the instance anyway, which is the thing under test.
test('holds the latest snapshot across a drop and reconnects only once', () => {
	vi.stubGlobal('EventSource', FakeEventSource);
	FakeEventSource.opened = [];
	const fleet = new Fleet();

	expect(fleet.snapshot).toBe(null);
	expect(fleet.connected).toBe(false);

	fleet.connect();
	expect(FakeEventSource.opened).toHaveLength(1);
	expect(FakeEventSource.opened[0]?.url).toBe('/api/events');

	FakeEventSource.opened[0]?.deliver('fleet', snapshot);
	expect(fleet.connected).toBe(true);
	expect(fleet.snapshot?.repos[0]?.name).toBe('sluss');

	// A drop reports itself but leaves the last snapshot on screen.
	FakeEventSource.opened[0]?.fail();
	expect(fleet.connected).toBe(false);
	expect(fleet.snapshot?.repos[0]?.name).toBe('sluss');

	// The root layout may mount more than once in development; a second call must not
	// open a second stream.
	fleet.connect();
	expect(FakeEventSource.opened).toHaveLength(1);

	vi.unstubAllGlobals();
});

test('fills in the arrays a partial snapshot leaves out', () => {
	vi.stubGlobal('EventSource', FakeEventSource);
	FakeEventSource.opened = [];
	const fleet = new Fleet();

	fleet.connect();
	FakeEventSource.opened[0]?.deliver('fleet', { at: '2026-08-28T10:00:00Z' });

	expect(fleet.snapshot?.repos).toEqual([]);
	expect(fleet.snapshot?.scopeErrors).toEqual([]);

	vi.unstubAllGlobals();
});
