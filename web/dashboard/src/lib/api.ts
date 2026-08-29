// The shapes sluss sends the browser, mirrored by hand from the Go structs named
// against each type. Nothing generates this: the API is Go in this same repo, so a
// codegen step would cost more than it saves at this size. When a Go struct here
// changes, change it here too — `npm run check` catches every use, not the drift.

/** internal/fleet/fleet.go: Sandbox */
export type Sandbox = {
	scope: string;
	name: string;
	id: string;
	agent: string;
	status: string;
	/** primary checkout path, "" when unknown */
	repo: string;
	/** its base name, what the UI groups under */
	repoName: string;
	worktree: string;
	branch: string;
	dirty: boolean;
	unmerged: number;
	/** the worktree is gone from disk */
	missing: boolean;
	/** host port for OpenCode Web, 0 when unpublished */
	webPort: number;
};

/** internal/fleet/fleet.go: RepoGroup */
export type RepoGroup = {
	repo: string;
	name: string;
	sandboxes: Sandbox[];
};

/** internal/fleet/fleet.go: ScopeError */
export type ScopeError = {
	scope: string;
	error: string;
};

/** internal/fleet/fleet.go: Snapshot — the whole `fleet` SSE event body */
export type Snapshot = {
	at: string;
	repos: RepoGroup[];
	scopeErrors: ScopeError[];
};

/** GET /api/config — the anonymous struct at internal/server/server.go:149 */
export type AccessConfig = {
	access: string;
	hostPrefix: string;
	domain: string;
	repos: string[];
	scopes: string[];
};

/** internal/kits/kits.go: Kit */
export type Kit = {
	name: string;
	hasSpec: boolean;
};

/** internal/kits/kits.go: GitStatus */
export type GitStatus = {
	/** is it a git checkout at all */
	repository: boolean;
	dirty: boolean;
	/** porcelain output, or why the status is unknown */
	detail: string;
};

/** GET /api/kits */
export type KitList = {
	kits: Kit[];
	status: GitStatus;
};

/** GET /api/secrets/{scope} — names only; values are write-only (D1) */
export type SecretList = {
	names: string[];
};

// Every error body writeError() produces is `{"error": …}`. A refusal from a
// lifecycle route adds its own stderr (internal/lifecycle: Result), which is what the
// UI shows in preference to the generic message. Both are optional because an error
// body is also what a fetch falls back to on a non-JSON response.
export type ApiError = {
	error?: string;
	exitCode?: number;
	stdout?: string;
	stderr?: string;
};

// --- Requests ---------------------------------------------------------------

/**
 * Either the parsed body or one ready-to-display line. No component sees a Response, a
 * thrown fetch, or the precedence rule below.
 */
export type Result<T> = { ok: true; body: T } | { ok: false; message: string };

// A lifecycle route answers a refusal with its own stderr (see ApiError above),
// which is the message worth showing; every other route carries `error`. The status line
// is the last resort, for a response whose body is not JSON at all.
async function messageFor(response: Response): Promise<string> {
	const body: ApiError = await response.json().catch(() => ({}));
	return (body.stderr || body.error || `failed with ${response.status}`).trim();
}

async function send<T>(
	url: string,
	init: RequestInit | undefined,
	read: (response: Response) => Promise<T>
): Promise<Result<T>> {
	try {
		const response = await fetch(url, init);
		if (!response.ok) return { ok: false, message: await messageFor(response) };
		return { ok: true, body: await read(response) };
	} catch (problem) {
		// An unreachable server is a message like any other, so no caller writes a catch.
		return { ok: false, message: String(problem) };
	}
}

/** A JSON read. */
export function request<T>(url: string, init?: RequestInit): Promise<Result<T>> {
	return send(url, init, (response) => response.json() as Promise<T>);
}

/** A plain-text read — a kit's spec.yaml is text on the wire, not JSON. */
export function requestText(url: string, init?: RequestInit): Promise<Result<string>> {
	return send(url, init, (response) => response.text());
}

/**
 * A mutation whose success body is never read. The secret and kit writes answer 204 with
 * no body at all, so parsing one would turn every successful save into an error.
 */
export function command(url: string, init: RequestInit): Promise<Result<void>> {
	return send<void>(url, init, async () => undefined);
}
