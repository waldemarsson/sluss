import { redirect } from '@sveltejs/kit';

// /config holds the configuration areas rather than being one itself. Secrets is the
// first of them, and the one the top bar links to by way of here.
export function load() {
	redirect(307, '/config/secrets/');
}
