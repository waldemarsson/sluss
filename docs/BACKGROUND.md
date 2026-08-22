# Background

How this design reached its current shape, and an honest read on whether it's worth building. For what it *is*, see [SPEC.md](SPEC.md); for what happens next, [ROADMAP.md](ROADMAP.md).

## Do this first

Run [SPIKE.md](SPIKE.md). Two of the seven assumptions are load-bearing:

1. **Is the sbx workspace mount bidirectional?** The whole worktree model depends on it.
3. **Is SSE through a reverse proxy fast enough?** The browser-chat premise is the reason this project exists.

If either is false, revise `SPEC.md` before writing code. An afternoon spent here is cheap; six weeks spent on a wrong assumption is not.

## How this got here

The design went through four revisions, and most of them made the project smaller:

**v0.1** — hand-rolled hardened Docker containers, no host mounts, a `git ext::` transport to extract work, Traefik with Docker labels, .NET. Ambitious and largely unnecessary.

**v0.2** — discovered Docker Sandboxes. sbx already had microVM isolation, a credential proxy, network policy, kits, skills, secrets, a TUI, and first-class OpenCode support. About 70% of v0.1 was deleted. Also corrected: Traefik's Docker labels can't see sbx sandboxes, because they're microVMs under `sandboxd`, not host containers.

**v0.3** — found `--app-name`, which scopes the sbx daemon, login, and secret store. That turned profiles from a naming convention into a real boundary. Confirmed the workspace mount is bidirectional, which killed the `ext::` design and brought back host worktrees. Moved to an embedded reverse proxy for SSE control and single-binary distribution.

**v0.4** — Go over C#, deliberately, partly to learn it. Svelte dashboard embedded via `go:embed`. Estimate corrected upward from a naive 400 lines to a realistic 2,200–4,100, once error handling, reconciliation, and unwind paths were counted honestly.

**v0.5** — profiles deferred. Since `--app-name` and `--kit` are sbx primitives, credential isolation works today by typing the flags; sluss would only add ergonomics. Design preserved in `PROFILES.md` with trigger conditions. Cuts Phase 1 to one or two weekends.

## The honest read

This space is crowded — Sculptor, Conductor, Claude Squad, Vibe Kanban, Emdash, container-use — and it has a high mortality rate. Bloop shut down in April 2026, Crystal was deprecated in February 2026, Codeanywhere is sunsetting.

The gap this fills is narrow: **OpenCode as the agent, reachable in a browser from any device, over sandboxed environments with proper git review.** With profiles deferred, what's left is convenience glue over sbx — worktrees plus a proxy plus a dashboard.

That's enough to justify building it for yourself. It is not a product, and the scope now reflects that. `PROFILES.md` holds the part that would have been.

[SPEC.md](SPEC.md) §14 has kill criteria. They're written down now because they get much harder to apply honestly once there's code you're fond of.
