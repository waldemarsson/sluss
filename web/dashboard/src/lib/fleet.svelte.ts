import type { Snapshot } from '#lib/api.js';

// As in access.svelte.ts: the event body is untyped JSON, so the arrays are filled in
// here rather than guarded at every read. `snapshot` stays nullable — "no poll yet" is
// a real state the dashboard renders differently, and normalising cannot remove it.
function complete(body: Partial<Snapshot>): Snapshot {
	return {
		at: body.at ?? '',
		repos: body.repos ?? [],
		scopeErrors: body.scopeErrors ?? []
	};
}

// The SSE stream outlives any one route: the header badge in the layout and the fleet
// on the dashboard both read it, and navigating between routes must not restart it.
// The app uses the `fleet` singleton below; the class is exported so tests can hold an
// instance of their own rather than reaching into session state left by another test.
export class Fleet {
	snapshot = $state<Snapshot | null>(null);
	connected = $state(false);
	#events: EventSource | null = null;

	// Idempotent, and never torn down: the root layout mounts once and lives as long as
	// the tab does.
	connect() {
		if (this.#events) return;
		// The whole snapshot arrives on every poll; there is no client-side state to
		// reconcile because the server holds none either.
		this.#events = new EventSource('/api/events');
		this.#events.addEventListener('fleet', (event) => {
			this.snapshot = complete(JSON.parse(event.data));
			this.connected = true;
		});
		// A dropped stream leaves the last snapshot on screen — stale is more useful
		// than blank, and the badge says which it is.
		this.#events.onerror = () => (this.connected = false);
	}
}

export const fleet = new Fleet();
