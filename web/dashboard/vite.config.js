import { sveltekit } from '@sveltejs/kit/vite';

export default {
	plugins: [sveltekit()],
	// `npm run dev` talks to a locally running slussd rather than mocking it.
	server: { proxy: { '/api': 'http://127.0.0.1:8420' } }
};
