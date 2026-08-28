---
slug: gui-control-plane
waypoint: review
status: approved
updated: 2026-08-28
findings: H1:fixed M1:fixed M2:fixed M3:fixed M4:fixed L1:fixed L2:fixed L3:fixed L4:fixed L5:fixed
---
Strengths: the sbx chokepoint holds — every invocation goes through `internal/sbx/sbx.go` with
app-name as a required parameter, asserted directly in `internal/sbx/sbx_test.go:53`
(`TestEveryCallIsScoped`). Lifecycle really is the script and not a Go reimplementation:
`internal/script/script_test.go` drives the repository's own `scripts/sluss` against a stubbed
sbx, so AC9 and AC10 are proven by behaviour rather than by mocks. `FlushInterval: -1` is present
and, unusually, actually tested — `internal/proxy/proxy_test.go:TestStreamingIsNotBuffered`
asserts the first chunk arrives before the upstream finishes. Statelessness is real: nothing
persists, `internal/fleet/fleet_test.go:TestRemovedSandboxDisappears` and
`TestVanishedSandboxStopsResolving` cover the `sbx rm` case end to end. Secret values are
piped on stdin and never enter argv (`internal/sbx/sbx_test.go:TestSecretValueNeverReachesArgv`).

HIGH:
- H1 · internal/server/server.go:206,217,233 · lifecycle runs on `r.Context()`, so a browser
  disconnect — tab closed, page reloaded, phone sleeping mid-request — cancels the context and
  SIGKILLs `scripts/sluss` in flight → the script's own unwind never runs, and a `start` killed
  after `git worktree add` but before `sbx create` returns leaves an orphan worktree and
  `agent/<name>` branch. The next `sluss start <name>` then takes the "worktree already exists"
  path and fails against a sandbox that was never created, so the name is stuck until someone
  cleans up by hand. Destroy has the mirror problem (sandbox removed, worktree left). Fix: run
  mutations on `context.WithoutCancel(r.Context())` with an explicit timeout — the operation
  should outlive the request that started it.

MEDIUM:
- M1 · internal/server/server.go:190 · no cross-origin defence on `POST /api/sandboxes`. The
  handler decodes JSON regardless of `Content-Type`, so a page the user visits can send a
  cross-origin *simple* request (`Content-Type: text/plain`) to `http://127.0.0.1:8420` and start
  sandboxes; the browser blocks reading the reply, not the side effect. `DELETE` and `PUT` are
  preflighted and therefore safe. Deferring authentication was a decision about *reachability*,
  and the browser is precisely the case where loopback is reachable from the whole web, so this
  is a gap in that reasoning rather than an instance of it. Fix: require
  `Content-Type: application/json` on the mutating POST, or reject `Sec-Fetch-Site: cross-site`.
- M2 · internal/server/server.go:196,206 · `extra` is forwarded verbatim into `sbx create`. There
  is no shell involved so this is not injection, but it is arbitrary sbx flags — including
  another `--publish`, or a second workspace mounting any readable host path into a sandbox.
  Harmless from a terminal, less so once M1 lets a web page reach the same endpoint, and moot
  only while nothing hostile is on the LAN. Fix: allowlist the flags the create form offers.
- M3 · internal/doctor/doctor.go:78 · `checkListener` proves the address by binding it, so running
  `slussd doctor` while `slussd serve` is up always reports `fail cannot bind …` and exits 1 —
  which is exactly when a person reaches for doctor. AC13 asks it to report the bound address,
  not to require the address be free. Fix: on bind failure, try connecting to the same address
  and report "already serving here" instead of a failure.
- M4 · internal/proxy/proxy.go:128 · the trailing-slash redirect is built from `req.URL.Path`
  alone, dropping the query string: `/s/personal/web?tab=chat` redirects to `/s/personal/web/`.
  Any state an app carries in its initial URL is lost on first hit. Fix: preserve `RawQuery`.

LOW:
- L1 · internal/proxy/proxy.go:145 · `statusFor` uses a bare type assertion on `*routeError`;
  `errors.As` would survive the first time one of these gets wrapped.
- L2 · internal/kits/kits.go:95 · `WriteSpec` truncates in place, so a crash mid-write leaves a
  half-written `spec.yaml`. Write to a temporary file and rename — the kits directory is a git
  repository, so it is recoverable, which is why this is LOW and not MEDIUM.
- L3 · internal/fleet/fleet.go:214 · git facts are gathered serially, three git processes per
  sandbox per tick. At the 3s default a twenty-sandbox fleet is ~60 processes every 3 seconds.
  Fine at the intended scale; worth knowing before the interval is lowered.
- L4 · internal/proxy/proxy_test.go:344 · the streaming assertion depends on a 300ms wall-clock
  threshold and may flake on a loaded CI runner.
- L5 · internal/sbx/sbx.go:104 · `List` decodes from the first `{` in the output, which handles
  the first-run banner but would break on a banner that itself contains a brace.

Spec compliance:
- [x] AC1 — fleet across scopes with repo/branch/agent/status/dirty/unmerged (fleet_test.go, server_test.go)
- [x] AC2 — removal visible within one tick and the route stops resolving (fleet_test.go, proxy_test.go)
- [x] AC3 — `lan: false` binds loopback only (config_test.go, main_test.go); host-to-host proof is T14
- [ ] AC4 — path mode routes with zero infrastructure in tests, but whether OpenCode Web tolerates
      a base path is still unanswered (T1). Until that spike runs, AC4 is unproven, not met.
- [ ] AC5 — needs the NAS and a phone (T14)
- [ ] AC6 — needs the NAS (T14)
- [x] AC7 — both modes over one listener, `Match` covered per case (proxy_test.go)
- [x] AC8 — claude/copilot deep links rendered, never proxied (+page.svelte:37-48)
- [x] AC9 — same script, same worktree/branch/mounts (script_test.go, and the live smoke run)
- [x] AC10 — dirty and unmerged refusals surfaced verbatim, `--force` never passed
- [x] AC11 — names only, value never returned or logged, scoped per app-name
- [x] AC12 — byte-for-byte round trip including CRLF; kits directory status reported
- [~] AC13 — every check present, but M3 makes it fail against a running daemon

Verdict: With fixes — H1 is a real state-corruption path in ordinary use and should be fixed
before the daily loop; M1 and M2 should be fixed before anything runs with `lan: true`; the rest
can wait for the host spikes.

## Re-review (2026-08-28)

- H1 · held — `internal/server/server.go:236` (`lifecycleContext`) runs create, stop and destroy on
  `context.WithoutCancel(r.Context())` with a 30-minute cap. Verified by
  `server_test.go:TestLifecycleOutlivesTheRequestThatStartedIt`, which cancels the request 50ms
  into a 400ms script and then waits for the script's own completion marker.
- M1 · held — `internal/server/server.go:120` (`guard`) wraps every mutating route: `Sec-Fetch-Site`
  other than same-origin/none is 403, and POST without `application/json` is 415, which is the
  shape a cross-origin request can take without a preflight. Reads stay unguarded. Verified by
  `TestMutatingRoutesRefuseCrossSiteRequests` (6 cases) and by curl against the running daemon: a
  `text/plain` drive-by POST returns 403.
- M2 · held — `internal/server/server.go:353` (`allowedExtras`) accepts only `--cpus` and
  `--memory` with numeric values; `--workspace /etc` and `--publish 2222` are refused with a
  message naming the flag. Verified by `TestCreateRefusesArbitrarySbxFlags` (6 cases).
- M3 · held — `internal/doctor/doctor.go:80` falls back to dialling the address and reports Warn
  ("already in use and answering") instead of Fail. `slussd doctor` against a live daemon now
  exits 0. Verified by `TestPortInUseAndAnsweringWarns`, with `TestUnbindableAddressFails` keeping
  the genuine failure path covered.
- M4 · held — `internal/proxy/proxy.go:128` redirects via `slashed.RequestURI()`;
  `/s/personal/auth?tab=chat` now lands on `/s/personal/auth/?tab=chat`. Covered in
  `TestPathModeAddsTheTrailingSlash`.
- L1 · held — `errors.As` replaces the type assertion in `statusFor`.
- L2 · held — `WriteSpec` writes a temporary file and renames over the spec; the test also asserts
  no temporary file survives and that the mode stays 0644.
- L3 · held — git facts are gathered concurrently with a cap of 8
  (`internal/fleet/fleet.go:206`), errors collected under a mutex and sorted so snapshots stay
  deterministic. `go test -race ./...` is clean.
- L4 · held — the streaming test now blocks the upstream on a channel the test closes after
  reading the first chunk, so buffering fails the test by timeout rather than by a wall-clock
  guess. The proxy package's runtime dropped from 0.44s to 0.04s.
- L5 · held — `List` tries each `{` in turn instead of the first, with a test for a banner
  containing a brace and one for output with no JSON at all.

Regression risk in the touched code: the fleet poller is the only fix that introduced concurrency;
`-race` passes and snapshot ordering is now explicitly sorted for both groups and scope errors.
The `guard` changes the dashboard's own contract — the stop button had to start sending
`Content-Type: application/json` (`+page.svelte:96`), which is covered by the same test that
covers the cross-site cases. One incidental fix: `web/dashboard/static/.gitkeep` now regenerates
`build/.gitkeep` on every build, so a plain `npm run build` can no longer delete the file
`go:embed` depends on.

Gates: `go test -race ./...` green (11 packages, 112 test functions), `task check` green including
`shellcheck` and the script's 30 assertions.

Verdict: Yes — every finding verified fixed against the code, with a test or a live check for
each; the outstanding work is the host spikes (T1, T2) and deployment acceptance (T14), which no
change here can settle.
