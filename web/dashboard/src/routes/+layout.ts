// One static bundle, rendered in the browser: the fleet only exists at runtime.
export const prerender = true;
export const ssr = false;

// Nested routes must prerender to `config/secrets/index.html` rather than
// `config/secrets.html`, because sluss serves the build with a plain
// http.FileServerFS (internal/server/server.go) and that only resolves a directory
// index. Without this a bookmarked /config/secrets 404s from the binary.
export const trailingSlash = 'always';
