---
slug: gui-control-plane
waypoint: plan
status: approved
updated: 2026-08-28
tasks: T1 T2 T3:done T4:done T5:done T6:done T7:done T8:done T9:done T10:done T11:done T12:done T13:done T14
---

# Plan: GUI control plane over sbx

**Goal:** ship a stateless Go daemon that serves an embedded Svelte fleet dashboard, proxies
OpenCode Web over one port, and drives every lifecycle action through the unchanged
`scripts/sluss`.

**Architecture:** one binary, no persistent state. A single poll loop asks
`sbx --app-name <scope> ls --json` per configured scope, enriches each sandbox with git facts
read from the worktree path sbx reports, and publishes an immutable snapshot. That snapshot is
the only source of truth for three consumers — the JSON/SSE API, the reverse proxy's routing
map, and `doctor`. Mutations shell out to `scripts/sluss` (lifecycle) or `sbx` (secrets); kits
are opaque bytes on disk. Packages are flat under `internal/`, concrete types throughout, no
interfaces except where a test genuinely needs a seam.

## Global constraints

Copied from `spec.md`; every task inherits these.

- `scripts/sluss` and its installed path are unchanged by this work. The Go binary calls it; it
  does not replace or modify it.
- All sbx invocation is centralised in `internal/sbx`, with app-name a parameter on every
  command — never an option with a default, never a call constructed outside that package.
- sluss parses no YAML. Kit `spec.yaml` is read and written as opaque text; sbx is the validator.
  No Go struct models the kit schema.
- `FlushInterval: -1` on sluss's proxy, no buffering middleware, no `Content-Length` on streamed
  responses. Do not remove it; say so explicitly in any summary that touches `internal/proxy`.
- sluss binds exactly one listener. `lan` decides its address and nothing else. Sandbox ports
  stay loopback in both access modes. On bind failure, exit naming the address tried — never
  fall back to a different address.
- Authentication is deferred by explicit decision. Do **not** add a token gate to mutating
  routes. AGENTS.md was corrected on 2026-08-28 to record the deferral; `docs/DECISIONS.md`
  follows at Document.
- Secret values are never written to sluss's disk, never rendered into a page, never used to
  populate a form field, and never logged. Only names are read back.
- sluss emits no configuration for any external proxy and knows the name of none.
- Go standard library first. Runtime deps on the target: `sbx`, `git`, `scripts/sluss`. No new Go
  module dependency without asking; SvelteKit + adapter-static on the web side.
- No Go test shells out to a real `sbx`. Tests stub it, the way `scripts/test-sluss` already does.
- The author is learning Go: prefer the plainer form, and explain any non-obvious idiom in a
  one-line comment (pointer receivers, `context.Context` threading, `atomic.Pointer`, channel
  patterns).

## File structure

- `cmd/slussd/main.go` — Create — entry point; flag parsing, `serve` / `doctor` / `version`,
  bind and shutdown. Replaces `cmd/sluss/main.go` (deleted; see Risks for the rename).
- `internal/config/config.go` — Create — load, default and validate `~/.config/sluss/config.json`;
  derive the bind address and the URL form.
- `internal/sbx/sbx.go` — Create — the single chokepoint for every `sbx` invocation.
- `internal/sbx/errors.go` — Create — map known sbx failures (not logged in, daemon not running,
  name exists, kit fetch failed) to readable messages with a suggested fix.
- `internal/sbxstub/stub.go` — Create — test-only helper that installs a fake `sbx` on `PATH`,
  serves a fixture for `ls --json`, and records argv. Imported by `sbx`, `fleet` and `script`
  tests; mirrors the stub in `scripts/test-sluss`.
- `internal/gitfacts/gitfacts.go` — Create — branch, dirty flag and unmerged commit count for one
  worktree path, tolerating a path that no longer exists.
- `internal/fleet/fleet.go` — Create — the poll loop, the snapshot type, and subscriber fan-out.
- `internal/proxy/proxy.go` — Create — snapshot-driven reverse proxy; path and host addressing.
- `internal/script/script.go` — Create — runner for `scripts/sluss` (start, stop, destroy) with
  cwd and environment set per call.
- `internal/kits/kits.go` — Create — list kit directories, read/write `spec.yaml` as bytes, report
  the kits directory's git status.
- `internal/server/server.go` — Create — the HTTP mux: static assets, JSON API, SSE, proxy mount.
- `internal/server/assets.go` — Create — `go:embed` of `web/dashboard/build`, exposed as an
  `fs.FS` variable so tests can substitute a `fstest.MapFS`.
- `internal/doctor/doctor.go` — Create — the checks behind `slussd doctor`, returning a report
  value the command prints.
- `web/dashboard/` — Create — SvelteKit app (adapter-static, Svelte 5 runes): fleet table, create
  form, secrets pane, kit editor.
- `Taskfile.yml` — Modify — `web` task, `build` depends on it, `BIN` and package path follow the
  rename.
- `.github/workflows/ci.yml` — Modify — build path follows the rename; add the dashboard build.
- `.github/workflows/release.yml` — Modify — build path and archive name follow the rename. The job
  stays disabled; leaving the old path there would break the day it is re-enabled.
- `docs/SPIKE.md` — Modify — record the answers from T1 and T2.
- `scripts/sluss`, `scripts/install.sh`, `scripts/test-sluss` — **unchanged.**
- `spike/proxy/main.go` — Delete at the end of T9; its findings live in `internal/proxy`.

## Public contracts (keep stable across tasks)

- `internal/config` — `Load(path string) (*Config, error)` — read, default and validate; returns a
  named error when `access: host` has no `domain`, or the port is out of range.
- `internal/config` — `Config` fields `LAN bool`, `Port int`, `Access string`, `HostPrefix string`,
  `Domain string`, `AppNames []string`, `Repos []string`, `KitsDir string`, `WorktreeRoot string`.
- `internal/config` — `(*Config) BindAddr() string` — `127.0.0.1:<port>` when `lan` is false, else
  `:<port>`. The only place that decision is made.
- `internal/config` — `DefaultPath() string` — `$XDG_CONFIG_HOME/sluss/config.json`, else
  `~/.config/sluss/config.json`.
- `internal/sbx` — `Client` with field `Bin string` (default `sbx`).
- `internal/sbx` — `(*Client) List(ctx context.Context, appName string) ([]Sandbox, error)`.
- `internal/sbx` — `Sandbox` fields `Name`, `ID`, `Agent`, `Status string`, `Ports []Port`,
  `Workspaces []string`; `Port` fields `HostIP string`, `HostPort`, `SandboxPort int`,
  `Protocol string`. Names match the JSON sbx emits.
- `internal/sbx` — `(*Client) SecretNames(ctx, appName) ([]string, error)`,
  `(*Client) SecretSet(ctx, appName, name, value string) error`,
  `(*Client) SecretDelete(ctx, appName, name string) error`.
- `internal/sbx` — `(*Client) Version(ctx) (string, error)`.
- `internal/gitfacts` — `Read(ctx, repoPath, worktreePath string) (Facts, error)`; `Facts` fields
  `Branch string`, `Dirty bool`, `Unmerged int`, `Missing bool`.
- `internal/fleet` — `Sandbox` fields `Scope`, `Name`, `ID`, `Agent`, `Status`, `Repo`,
  `RepoName`, `Worktree`, `Branch string`, `Dirty bool`, `Unmerged int`, `Missing bool`,
  `WebPort int`. Identity is the pair `(Scope, Name)`.
- `internal/fleet` — `Snapshot` fields `At time.Time`, `Repos []RepoGroup`, `ScopeErrors []ScopeError`;
  `RepoGroup` fields `Repo`, `Name string`, `Sandboxes []Sandbox`.
- `internal/fleet` — `(*Snapshot) Find(scope, name string) (Sandbox, bool)`.
- `internal/fleet` — `New(cfg *config.Config, c *sbx.Client, interval time.Duration) *Poller`,
  `(*Poller) Run(ctx) error`, `(*Poller) Current() *Snapshot`,
  `(*Poller) Subscribe() (<-chan *Snapshot, func())` — the returned func unsubscribes.
- `internal/proxy` — `New(cfg *config.Config, p *fleet.Poller) *Router`,
  `(*Router) Handler() http.Handler`, `(*Router) Match(host, path string) (scope, name, rest string, ok bool)`.
- `internal/script` — `Runner` with fields `Script string`, `WorktreeRoot string`.
- `internal/script` — `(*Runner) Start(ctx, StartOpts) (Result, error)`,
  `(*Runner) Stop(ctx, repo, appName string, names ...string) (Result, error)`,
  `(*Runner) Destroy(ctx, repo, appName, name string) (Result, error)`; `StartOpts` fields
  `Repo`, `Name`, `Agent`, `AppName string`, `Extra []string`; `Result` fields `ExitCode int`,
  `Stdout`, `Stderr string`.
- `internal/kits` — `List(dir string) ([]Kit, error)`, `ReadSpec(dir, name string) ([]byte, error)`,
  `WriteSpec(dir, name string, body []byte) error`, `Status(ctx, dir string) (GitStatus, error)`.
- `internal/server` — `New(cfg, poller, runner, sbxClient, assets fs.FS) *Server`,
  `(*Server) Handler() http.Handler`.

### HTTP surface (stable across tasks)

- `GET /` and asset paths — the embedded dashboard.
- `GET /api/fleet` — current snapshot as JSON.
- `GET /api/events` — SSE; one event per snapshot, plus a heartbeat.
- `POST /api/sandboxes` — create/start via the script. Body: repo, name, agent, scope, extra args.
- `POST /api/sandboxes/{scope}/{name}/stop`.
- `DELETE /api/sandboxes/{scope}/{name}` — destroy, never forced.
- `GET /api/secrets/{scope}` — names only.
- `PUT /api/secrets/{scope}/{name}` — set; body is the value, never echoed.
- `DELETE /api/secrets/{scope}/{name}`.
- `GET /api/kits` — kit names plus the kits directory's git status.
- `GET /api/kits/{name}/spec` and `PUT /api/kits/{name}/spec` — opaque bytes.
- `/s/{scope}/{name}/...` — proxy, path mode. Host mode dispatches on the `Host` header before
  the mux.

Routing uses the standard library `http.ServeMux` method-and-wildcard patterns (Go 1.22+). No
router dependency.

## Tasks

Ordering: T1 and T2 are host-only spikes and gate T9 / T11 / T12; T3–T8 have no spike dependency
and can start immediately.

### T1: Spike — is the proxy premise true?
**Files:** Modify — `docs/SPIKE.md`
**Deliverable:** two recorded answers, run on the host Mac with a real OpenCode sandbox and the
existing `spike/proxy` binary. (a) Does OpenCode Web work when served under a base path
(`/s/<scope>/<name>/`) — do its own asset and SSE URLs resolve, or does it assume root? (b)
SPIKE 3: tokens through the proxy versus direct versus `sbx tui` — same prompt, same session.
**Contract:** none. The verdict decides whether `path` stays the default access mode.
**Behaviors to cover:** not a code task. Record the sbx version, the exact commands, and a plain
verdict sentence for each; if (a) is false, stop and say so rather than planning around it.
**Gate:** `docs/SPIKE.md` assumption 3 and the base-path question both carry an answer, and the
verdict names the default access mode.

### T2: Spike — sbx `secret`, `kit` and app-name isolation
**Files:** Modify — `docs/SPIKE.md`
**Deliverable:** the unfinished half of SPIKE 5 plus the two CLI surfaces the panes need: does
`secret ls` return names only; does `secret set` read the value from stdin (argv would expose it
in `ps`); is a secret set in one app-name invisible in another; do sandbox names collide across
app-names; what subcommands does `sbx kit` have, and is there a validate.
**Contract:** none. Fixes the exact argv `internal/sbx` builds for secrets in T11.
**Behaviors to cover:** not a code task.
**Gate:** SPIKE 5's answer is complete rather than partial, and the CLI surface section records
`sbx secret --help` and `sbx kit --help` verbatim.

### T3: Configuration
**Files:** Create — `internal/config/config.go`
**Deliverable:** config loading with defaults (port 8420, `access: path`, worktree root
`~/src/worktrees`), validation, and the single place the bind address is derived.
**Contract:** `Load`, `Config`, `BindAddr`, `DefaultPath` as above.
**Behaviors to cover with tests:** defaults applied to a minimal file; `access: host` without
`domain` refused by name; unknown `access` value refused; port out of range refused; `lan: false`
binds loopback and `lan: true` binds all interfaces; a missing config file reports the path it
looked for; malformed JSON reports the offending file.
**Gate:** `go test ./internal/config/...` green.

### T4: sbx client — the chokepoint
**Files:** Create — `internal/sbx/sbx.go`, `internal/sbx/errors.go`, `internal/sbxstub/stub.go`
**Deliverable:** every sbx invocation in the project funnelled through one package, with app-name
a required parameter, plus the stub helper the later packages test against.
**Contract:** `Client`, `Sandbox`, `Port`, `List`, `SecretNames`, `SecretSet`, `SecretDelete`,
`Version` as above.
**Behaviors to cover with tests:** `--app-name <scope>` precedes the subcommand in every
invocation; the fixture from `docs/SPIKE.md` decodes into the right fields; a stopped sandbox with
no `ports` key yields an empty slice, not an error; a first-run banner printed before the JSON
still decodes (SPIKE 5); an empty scope yields zero sandboxes and no error; a non-zero exit is
wrapped with what was attempted and the mapped message, not raw stderr; `SecretSet` never places
the value in argv (assert on the recorded argv) and never appears in an error string.
**Gate:** `go test ./internal/sbx/...` green.

### T5: Git facts for a worktree
**Files:** Create — `internal/gitfacts/gitfacts.go`
**Deliverable:** branch, dirty flag and unmerged count for one worktree, computed against the
primary checkout's current HEAD — the same base `scripts/sluss destroy` uses.
**Contract:** `Read`, `Facts` as above.
**Behaviors to cover with tests:** clean worktree reports its branch, not dirty, zero unmerged;
an uncommitted change flips dirty; two commits on the task branch report two unmerged; a merged
branch reports zero; a worktree path that no longer exists returns `Missing` with no error and no
crash; a path that is not a git checkout returns an error naming the path. Tests build real
temporary repositories with `git`; nothing is stubbed.
**Gate:** `go test ./internal/gitfacts/...` green.

### T6: Fleet poller and snapshot
**Files:** Create — `internal/fleet/fleet.go`
**Deliverable:** one loop over the configured scopes producing an immutable snapshot, grouped by
repo, that every other component reads. Repo and worktree are derived from the `workspaces` array
sbx returns — the entry ending in `.git` is the repo, the other is the worktree — falling back to
`worktreeRoot/<repo>/<name>` when only one is present.
**Contract:** `Sandbox`, `Snapshot`, `RepoGroup`, `Find`, `New`, `Run`, `Current`, `Subscribe`
as above.
**Behaviors to cover with tests:** sandboxes from two scopes appear once each, grouped by repo,
with git facts merged in; identity stays scope-qualified so the same name in two scopes yields
two rows; a sandbox removed between ticks is gone from the next snapshot and nothing panics; a
scope whose `sbx ls` fails appears in `ScopeErrors` while other scopes still populate; a poll that
outlives the interval causes the next tick to be skipped, never stacked; a subscriber receives
each new snapshot and unsubscribing stops delivery without blocking the poller; a slow subscriber
is dropped rather than stalling the loop.
**Gate:** `go test ./internal/fleet/...` green.

### T7: HTTP server, fleet API and SSE
**Files:** Create — `internal/server/server.go`, `internal/server/assets.go`
**Deliverable:** the mux serving the embedded dashboard, `GET /api/fleet` and `GET /api/events`,
with assets injected as an `fs.FS` so tests need no built dashboard.
**Contract:** `New`, `(*Server) Handler` as above, plus the fleet and events routes.
**Behaviors to cover with tests:** `/api/fleet` returns the current snapshot with the documented
field names; `/api/events` sets the SSE content type, sends the current snapshot immediately, then
one event per snapshot, and ends when the client disconnects; an unknown API path is 404 with a
JSON body; asset requests serve `index.html` for the app root and 404 for a missing file; no
handler writes a `Content-Length` on the event stream.
**Gate:** `go test ./internal/server/...` green.

### T8: Fleet dashboard
**Files:** Create — `web/dashboard/` (SvelteKit, adapter-static); Modify — `Taskfile.yml`
**Deliverable:** the table AC1 describes — every sandbox across scopes, grouped by repo, with
branch, agent, status, dirty flag and unmerged count, live over SSE. Connect column: OpenCode gets
a link through sluss, Claude Code links to claude.ai/code and Copilot to github.com, neither
proxied; every row offers a copyable `sluss attach <name>` command. A row whose worktree is
missing renders marked; a stopped sandbox renders without a connect link.
**Contract:** consumes `/api/fleet` and `/api/events`; adds `task web` and makes `task build`
depend on it.
**Behaviors to cover with tests:** Svelte component tests are out of scope for this project; cover
instead with a Go test that the built asset tree is served and that the snapshot JSON carries every
field the table binds to. Verify the table by hand against a live fleet in T14.
**Gate:** `task web` builds, `task check` green, and the dashboard renders a two-scope fleet from a
stubbed API.

### T9: Reverse proxy
**Files:** Create — `internal/proxy/proxy.go`; Delete — `spike/proxy/main.go`
**Deliverable:** both addressing modes over the one listener, routed from the current snapshot.
`path`: `/s/<scope>/<name>/...` stripped before forwarding. `host`: `<prefix><name>.<domain>`
matched on the `Host` header, serving root. `FlushInterval: -1`, no buffering, no
`Content-Length` on streamed responses.
**Contract:** `New`, `Handler`, `Match` as above.
**Behaviors to cover with tests:** a running OpenCode sandbox is forwarded to the loopback host
port from the snapshot, with the prefix stripped in path mode and the path untouched in host mode;
`/s/<scope>/<name>` without a trailing slash redirects to the slashed form; an unknown scope or
name is 404 immediately, never a hang against a dead backend; a sandbox that disappeared between
ticks 404s once the snapshot updates; a stopped sandbox with no published port 404s with a
message saying it is stopped; in host mode a name matching two scopes is refused with a message
rather than routed arbitrarily; a streamed response reaches the client incrementally (assert the
first chunk arrives before the handler returns).
**Gate:** `go test ./internal/proxy/...` green, and the summary states explicitly that
`FlushInterval: -1` is present.

### T10: Lifecycle through the script
**Files:** Create — `internal/script/script.go`; Modify — `internal/server/server.go`,
`web/dashboard/`
**Deliverable:** create, start, stop and destroy driven from the GUI by executing
`scripts/sluss` with cwd set to the chosen repo and `SLUSS_APP_NAME` / `SLUSS_AGENT` /
`SLUSS_WORKTREE_ROOT` in its environment — the same code path a terminal takes. Destroy is never
forced; the script's refusal is surfaced to the user verbatim.
**Contract:** `Runner`, `Start`, `Stop`, `Destroy`, `StartOpts`, `Result`, plus the three
lifecycle routes.
**Behaviors to cover with tests:** with the real `scripts/sluss` and a stubbed `sbx`, start
creates the worktree and branch and passes the chosen agent to `sbx create`; the environment
carries the scope, agent and worktree root; cwd is the requested repo, and a repo not in the
configured list is refused before anything runs; destroy on a dirty worktree exits non-zero and
the message reaches the HTTP response unchanged; destroy on a clean merged worktree removes
sandbox, worktree and branch; `--force` is never passed; a name that fails the script's validation
returns the script's own error; the script's path is resolved once at startup and a missing script
is an error naming the path.
**Gate:** `go test ./internal/script/... ./internal/server/...` green and `task check` green.

### T11: Secrets pane
**Files:** Modify — `internal/server/server.go`, `web/dashboard/`
**Deliverable:** per-scope list of secret names, set a value, delete a name — write-only, using
the argv T2 established.
**Contract:** the three `/api/secrets` routes.
**Behaviors to cover with tests:** listing returns names only, for the requested scope, and a
scope not in the configured list is refused; setting a value returns no value in the response
body; the value never appears in any log line or error message the server produces; deleting
removes the name from the next listing; a failure from sbx surfaces as a readable message.
**Gate:** `go test ./internal/server/...` green.

### T12: Kit editor
**Files:** Create — `internal/kits/kits.go`; Modify — `internal/server/server.go`,
`web/dashboard/`
**Deliverable:** list the kit directories under `kitsDir`, edit each `spec.yaml` as plain text,
and show that directory's git status. No YAML parsing anywhere; sbx validates.
**Contract:** `List`, `ReadSpec`, `WriteSpec`, `Status`, plus the `/api/kits` routes.
**Behaviors to cover with tests:** a write round-trips byte-for-byte including trailing newline
and CRLF; a kit name containing a path separator or `..` is refused before touching the
filesystem; a kit directory with no `spec.yaml` is listed and reports as empty rather than
erroring; the status reports dirty after a write and clean after a commit; a `kitsDir` that is not
a git repository reports that plainly instead of failing the whole pane; a `kitsDir` that does not
exist yields an empty list and a clear message.
**Gate:** `go test ./internal/kits/... ./internal/server/...` green.

### T13: Command line — `serve`, `doctor`, `version`
**Files:** Create — `cmd/slussd/main.go`, `internal/doctor/doctor.go`; Delete —
`cmd/sluss/main.go`; Modify — `Taskfile.yml`, `.github/workflows/ci.yml`,
`.github/workflows/release.yml`
**Deliverable:** the binary. `serve` loads config, starts the poller, binds exactly one listener
and shuts down cleanly on SIGINT/SIGTERM. `doctor` reports the bound address and whether it is
loopback or exposed, the access mode with an example URL, sbx presence and version, each
configured scope's reachability, the resolved `scripts/sluss` path and whether it supports the
subcommands the GUI needs, each configured repo's validity as a git checkout, and the kits
directory's state. `version` keeps the existing `-X main.version` build stamp.
**Contract:** `doctor.Run(ctx, cfg) Report` and the three subcommands.
**Behaviors to cover with tests:** an occupied port exits non-zero naming the address tried and
never binds a different one; `lan: false` binds loopback only; `doctor` reports every check with
an ok/failed state and exits non-zero when any required check fails; a missing sbx is reported as
a failed check rather than a crash; a scope that errors is reported per scope; an unknown
subcommand prints usage and exits 2, matching the script's convention.
**Gate:** `task check` green, and both cross-compile targets in CI build.

### T14: Deployment acceptance
**Files:** Modify — `docs/SPIKE.md` (the `kvm` group finding)
**Deliverable:** the two deployments proven on real hardware. Laptop: `lan: false`,
`access: path`, no reverse proxy, no DNS, no certificate — every sandbox's OpenCode Web UI opens
and nothing is reachable from another host. NAS VM: `lan: true` behind one Incus proxy device and
one Traefik router, a phone on the VPN opens a sandbox over TLS and streams at TUI speed; confirm
Traefik does not buffer the stream. Confirm `groups agent` includes `kvm` before anything else,
and record the fix if it does not.
**Contract:** none.
**Behaviors to cover:** manual, against AC3, AC4, AC5, AC6, AC7. Creating and destroying a
sandbox must change nothing outside sluss — no Incus device, no Traefik route, no DNS record, no
file on the NAS host — and the new sandbox must be reachable within one poll interval. Switching
`access` between `path` and `host` must change only the URL form.
**Gate:** each of AC3–AC7 recorded as passed with the command or observation that proved it.

## Tests

- AC1 → T6 (snapshot merges sbx and git facts across scopes) + T8 (the table binds every field)
- AC2 → T6 (a sandbox removed between ticks disappears, nothing panics) + T9 (its route 404s)
- AC3 → T3 (`BindAddr` for `lan: false`) + T13 (binds loopback only) + T14 (verified from a
  second host)
- AC4 → T9 (path-mode routing) + T14 (laptop, zero infrastructure)
- AC5 → T1 (SSE latency verdict) + T9 (incremental streaming) + T14 (phone over TLS)
- AC6 → T10 (lifecycle touches only the script) + T14 (nothing changes on the NAS host)
- AC7 → T9 (both modes over one listener; `Match` covers each)
- AC8 → T8 (deep links rendered per agent, and no proxy route is created for them)
- AC9 → T10 (same script, same worktree/branch/mounts, asserted against the stubbed sbx argv)
- AC10 → T10 (dirty and unmerged destroys refused, message surfaced unchanged)
- AC11 → T11 (names only; value never returned or logged; scoped to one app-name)
- AC12 → T12 (byte-for-byte round-trip; kits directory reports dirty)
- AC13 → T13 (every doctor check present, ok/failed state, non-zero exit on failure)

## Risks / trade-offs

- **The base-path question is load-bearing and unanswered.** If OpenCode Web cannot serve under
  `/s/<scope>/<name>/`, `path` mode cannot carry OpenCode, and AC4 — zero infrastructure as a
  supported configuration — fails. T1 answers it before T9 starts; a "no" is a spec revision, not
  a workaround.
- **Binary name collision.** `scripts/install.sh` installs the script as `~/.local/bin/sluss`, and
  the Go binary was also `sluss`. Whichever landed first on `PATH` would win, and a Go binary that
  resolved the script by a bare `PATH` lookup could invoke itself. The plan renames the daemon to
  `slussd` (`cmd/slussd`) — `sluss` stays unambiguously the script, which the constraint requires
  anyway. This deviates from `spec.md`'s "Affected areas", which names `cmd/sluss/main.go`;
  approved 2026-08-28. `slussd` resolves the script explicitly — `$SLUSS_SCRIPT`, else the
  installed path — never by bare lookup, which could find `slussd` itself.
- **AGENTS.md previously required a token gate on every mutating route.** Corrected 2026-08-28 to
  record the deferral, so the implementer no longer reads two contradictory instructions.
  `docs/DECISIONS.md` still carries the old routing scheme and is rewritten at Document.
- **No auth plus `lan: true` means the destroy buttons are reachable by anything on the LAN.**
  Accepted by the spec, inheriting the homelab's documented stance for the existing opencode VM.
  The conditions for revisiting are unchanged: reachable outside the VPN, or untrusted devices on
  the LAN.
- **CI builds no dashboard.** `web/dashboard/build/.gitkeep` keeps `go:embed` compiling, so a CI
  binary serves an empty UI. `task build` depends on `task web` locally; adding node to CI is a
  small follow-up, not a blocker, but a release built without it would be broken — release is
  currently disabled in `release.yml`, which is why this is a risk rather than a task.
- **Nothing here is tested against a real sbx.** Every Go test stubs it, so the fixture in
  `docs/SPIKE.md` is the contract. If sbx's JSON shape has drifted since 2026-08-25, T4's tests
  pass and the real fleet is empty. T14 is the only place that catches it.
- **`--kit` repeatability (SPIKE 2) stays unanswered.** It gates only the environment × toolset
  compose UI, which is not in this plan — the create form offers a single kit picker either way.
- **Destroy is never forced from the GUI.** AC10 asks for the script's refusal to be surfaced, so
  the API offers no force flag; discarding unmerged work stays a deliberate terminal action. If
  that proves annoying in daily use, it is a later change, not a gap.
- **Fourteen tasks is a lot of surface for one person's convenience tooling.** T3–T9 deliver the
  fleet dashboard and the proxy — the two things `docs/BACKGROUND.md` says justify the project. If
  the daily loop disappoints after T9, stopping there is a legitimate outcome, and T10–T12 are the
  parts to drop.

## Open questions

- None. T1 and T2 exist to answer the spec's open questions before the code that depends on them;
  everything else is decided.

## Amendments

- None
