import adapter from '@sveltejs/adapter-static';

// adapter-static writes plain files that go:embed can carry inside the binary;
// there is no Node server in production (docs/DECISIONS.md D8).
export default {
	kit: {
		adapter: adapter({ pages: 'build', assets: 'build', precompress: false })
	}
};
