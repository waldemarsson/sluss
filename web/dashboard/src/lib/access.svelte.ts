import type { AccessConfig } from '#lib/api.js';

// sluss is trusted, but the body it sends is still untyped JSON, and a key it omits
// would otherwise be a missing array in every consumer. Filling the gaps once here is
// what lets a route read `config.repos` straight instead of guarding it.
function complete(body: Partial<AccessConfig>): AccessConfig {
	return {
		access: body.access ?? 'path',
		hostPrefix: body.hostPrefix ?? 'sluss-',
		domain: body.domain ?? '',
		repos: body.repos ?? [],
		scopes: body.scopes ?? []
	};
}

// How sluss addresses sandboxes, plus the repos and scopes the create form and the
// secrets page offer. It is fixed for the life of the server, so one fetch per session
// is enough. As with `fleet`, the class is exported for tests; the app uses `access`.
export class Access {
	// The empty shape rather than null, so every consumer can read through without a
	// guard for the window before the fetch lands.
	config = $state<AccessConfig>(complete({}));
	// False until the fetch settles, either way. Without it a consumer cannot tell an
	// empty configuration from one that has not arrived, and every load flashes the
	// "nothing configured" state on a correctly configured server.
	loaded = $state(false);
	#requested = false;

	// Failure is swallowed: the dashboard is still useful with an empty create form,
	// and the fleet's own errors are reported where they happen.
	load() {
		if (this.#requested) return;
		this.#requested = true;
		fetch('/api/config')
			.then((r) => r.json())
			.then((c: Partial<AccessConfig>) => (this.config = complete(c)))
			.catch(() => {})
			.finally(() => (this.loaded = true));
	}
}

export const access = new Access();
