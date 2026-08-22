# sluss — Spec v0.5

> A single-binary control plane over Docker Sandboxes (sbx). Creates isolated agent environments per repo, serves each one's OpenCode Web UI at a stable URL, and keeps work reviewable with ordinary git.

**MVP scope note:** per-client profiles are deliberately deferred. The design is preserved in `PROFILES.md` with trigger conditions for building it. Until then, sbx's `--app-name` and `--kit` flags are used manually.

**Status:** ready to build, pending the checks in `SPIKE.md`.

---

## 1. Problem

Running several coding agents in parallel means juggling terminal sessions and repeating environment setup per project. Existing tools solve parallelism, but none combine true sandbox isolation, OpenCode as the agent, and a chat UI reachable from any device on the LAN.

## 2. Product statement

Create isolated agent environments per repo. Reach each environment's OpenCode Web UI at a stable URL from any device on the LAN. Review the work with ordinary git. Run from one binary on macOS today and a Linux VM later.

## 3. Non-goals

- Building a chat UI, IDE, file browser, or terminal emulator
- Supporting agents other than OpenCode in v1
- Multi-agent orchestration or inter-agent coordination
- Kubernetes, cloud deployment, remote execution
- Reimplementing anything sbx already does
- Per-client profile management (deferred — see `PROFILES.md`)

---

## 4. Layer map

| Concern | Owner |
|---|---|
| microVM isolation, kernel boundary | sbx |
| Agent install and launch | sbx (`sbx create opencode`) |
| Secret storage, credential injection | sbx (`sbx secret`) |
| Network egress policy | sbx (`sbx policy`) |
| Environment tools, files, standards | sbx kits |
| Declarative environment definition | sbx (`.sbxenv.yaml`) |
| Identity scoping (`--app-name`), kits | sbx — invoked manually in the MVP |
| **Git worktree lifecycle** | **sluss** |
| **HTTP routing to sandbox ports** | **sluss** |
| **Cross-repo fleet view + dashboard** | **sluss** |

sluss shells out for everything in the top block. It never reimplements it.

---

## 5. Identity — MVP approach

**Profiles are deferred.** See `PROFILES.md` for the full design, the research behind it, and the conditions that should trigger building it.

For the MVP, identity is handled by sbx directly and passed through by sluss:

```bash
sluss env create auth --repo ~/src/mercurius \
  --app-name omegapoint --kit ~/kits/omegapoint
```

sluss stores `appName` and `kit` in environment state and replays them on every subsequent sbx call for that environment. It does not resolve them, validate them, or group by them.

**Critical constraint for later:** all sbx invocation must be centralised in `internal/sbx`, with app-name as a parameter on every command. Adding the profile resolver later is then a small change; scattering sbx calls across packages would make it a hunt.

Kits are hand-authored, live in git, and are referenced by path or `git+ssh://...#ref=v1.2&dir=name`. sluss never generates a kit `spec.yaml`.

---

## 6. Workspace model

sbx mounts the workspace from the host with bidirectional sync — the host sees changes live. No git transport tricks needed. But **two sandboxes cannot share one directory**, hence worktrees:

```
~/src/mercurius                      # your checkout, never mounted
~/src/mercurius-sluss/auth          # worktree on agent/auth → mounted into sandbox
~/src/mercurius-sluss/docs          # worktree on agent/docs → mounted into sandbox
```

- `env create` → `git worktree add ../<repo>-sluss/<id> -b agent/<id>`, mount that path
- Review → `cd` into the worktree, `git diff`, or open in an IDE
- `env destroy` → refuse if unmerged commits exist unless `--force`, then `git worktree remove`

**Not in the MVP:** an `--isolated` escape hatch mapping to `sbx create --clone` was specified in v0.5 and dropped. D4 removed the `git ext::` transport, which was the only way to get work out of a cloned sandbox, so the flag produced work that couldn't be reviewed. Stronger isolation needs an explicit sync step designed alongside it. See `ROADMAP.md`.

---

## 7. Routing

`sluss serve` runs one HTTP server on `:8420`, routing by `Host` header:

```
sluss.localhost           → dashboard
auth.sluss.localhost      → OpenCode for env "auth"
docs.sluss.localhost      → OpenCode for env "docs"
```

```go
proxy := &httputil.ReverseProxy{
    Rewrite:       func(r *httputil.ProxyRequest) { r.SetURL(target) },
    FlushInterval: -1,   // immediate flush — REQUIRED for OpenCode's SSE stream
}
```

`FlushInterval: -1` is not optional. It is the most likely cause of reported "web UI feels slower than the TUI" problems.

- **DNS:** `*.sluss.localhost` resolves to loopback in most resolvers. For phone access, add a wildcard in the UniFi Policy Table pointing at the Mac's IP.
- **TLS on Mac:** skip. Plain HTTP on the LAN.
- **TLS on NAS:** existing Traefik in front of `:8420`, terminating with the kaktus.nu wildcard.
- **Auth:** shared token from `~/.config/sluss/token`, set as a cookie, gating every mutating route. Non-negotiable before this is LAN-reachable — the dashboard has destroy buttons.

Routing table lives in memory, rebuilt from state at startup.

---

## 8. Dashboard

Svelte (adapter-static), embedded via `go:embed`, served from the same binary.

Its reason to exist over `sbx tui`: the columns that matter most — unmerged commit count, dirty worktree, branch — come from the host worktree, which only sluss knows about. sbx's TUI is also scoped per `--app-name`, so once you use more than one it can't show everything at once.

```
auth      mercurius   ● running   agent/auth   2 commits   12m ago   [open] [stop] [destroy]
docs      mercurius   ○ stopped   agent/docs   clean        3h ago   [start] [destroy]
babytabs  babytabs    ● running   agent/pwa    5 commits   just now  [open] [stop] [destroy]
```

Grouping by profile is a later addition; see `PROFILES.md`.

**API**

- `GET /api/environments` — initial list
- `GET /api/events` — SSE stream of state changes
- `POST /api/environments` — create
- `POST /api/environments/:id/stop`
- `DELETE /api/environments/:id`

Using SSE for the dashboard dogfoods the exact code path that carries OpenCode's stream. If flushing is broken, your own dashboard tells you first.

---

## 9. State

```json
// ~/.local/state/sluss/environments/auth.json
{
  "id": "auth",
  "appName": "omegapoint",
  "kit": "/Users/martin/kits/omegapoint",
  "repo": "/Users/martin/src/mercurius",
  "worktree": "/Users/martin/src/mercurius-sluss/auth",
  "branch": "agent/auth",
  "sandbox": "auth",
  "port": 14096
}
```

JSON, not YAML — sluss parses no YAML at all. It only *emits* the `.sbxenv.yaml` overlay, either as JSON-in-a-.yaml-file (YAML is a JSON superset) or as a small text template. See `SPIKE.md` assumption 7.

`env list` joins state files against `sbx --app-name <n> ls` for each distinct app-name in state. sbx is the source of truth for runtime status; sluss stores only what sbx doesn't know.

**Reconciliation** is required, not optional. State drifts the moment someone runs `sbx rm` by hand. Every read path must tolerate a sandbox that no longer exists, and `env list` should mark it rather than crash.

---

## 10. CLI

```bash
sluss doctor                       # sbx present? version? daemon up? logged in? virt available?
sluss serve                        # proxy + dashboard; launchd/systemd unit

sluss env create auth --repo ~/src/mercurius \
    --app-name omegapoint --kit ~/kits/omegapoint
sluss env list
sluss env open auth
sluss env stop auth
sluss env destroy auth [--force]
```

`--app-name` and `--kit` are stored in state and replayed. Defaults can come from `~/.config/sluss/config.json` so you don't type them every time.

`sbx tui`, `sbx exec`, `sbx policy log` are used directly. Don't wrap what doesn't need wrapping.

Deferred: `sluss profile list|login`. See `PROFILES.md`.

---

## 11. Implementation

**Go.** Chosen partly for single-command cross-compilation, partly deliberately as a learning project.

```bash
go build -o dist/sluss                                  # mac
GOOS=linux GOARCH=amd64 go build -o dist/sluss-linux    # nas
```

Layout:

```
cmd/sluss/main.go
internal/sbx/          # exec wrappers, JSON parsing, error mapping — THE chokepoint
internal/env/          # lifecycle: create, destroy, reconcile
internal/proxy/        # reverse proxy + routing table
internal/state/        # load/save/lock
web/embed.go           # go:embed
web/dashboard/         # SvelteKit
```

Dependencies: `cobra`, `lipgloss` (output), stdlib for everything else. Runtime dependencies on the target: `sbx` and `git` only. sbx cannot be bundled — it's a signed binary with device-flow auth, its own daemon, and a hardware-virt requirement. `sluss doctor` exists to make that legible.

Build: one `npm run build` produces arch-independent static assets consumed by both Go targets. Commit `web/dashboard/build/.gitkeep` so `go:embed` doesn't fail on a fresh clone.

---

## 12. Size estimate

| Area | LOC |
|---|---|
| sbx wrappers — args, JSON, exit codes, error mapping | 250–400 |
| Proxy — SSE, headers, upgrades, timeouts, dead backends | 150–250 |
| Worktree lifecycle — create, detect unmerged, cleanup, unwind | 200–300 |
| State — read/write/lock, crash recovery, reconcile | 200–300 |
| CLI commands, options, validation, output | 250–400 |
| Dashboard API — endpoints, SSE hub, auth | 200–300 |
| Svelte dashboard | 400–700 |
| Tests | 300–600 |
| Project files, wiring, logging, Taskfile, CI | 150–300 |

**2,100–3,550 lines.** Dropping profiles saves roughly 300–500 lines of resolver and plumbing — less than it feels like, because the mechanisms belonged to sbx anyway.

Error handling is the bulk of the work and the difference between a script and a tool. Every sbx call has a failure mode needing translation into something readable: not logged in, daemon down, name collision, kit fetch failed, network policy blocked an install hook.

**A minimal Phase 1 — no dashboard, no tests, no reconciliation — is 600–1,000 lines and one to two weekends.** That is the number that matters, because Phase 1 decides whether the rest is worth building.

---

## 13. Phases

| Phase | Deliverable |
|---|---|
| 0 | Run `SPIKE.md`. No code. |
| 1 | `env create/list/destroy`, worktrees, embedded proxy. No reconciliation, no tests. |
| 2 | Reconciliation, error handling, `doctor`, tests |
| 3 | Svelte dashboard |
| 4 | Linux build, NAS VM, Traefik in front |
| — | Profiles, when the triggers in `PROFILES.md` fire |

Phase 1 is deliberately crude. Its only job is telling you whether the daily loop is good before you spend the remaining weekends.

---

## 14. Kill criteria

- SSE is unpleasant after tuning → stop, use `sbx tui`, delete the proxy
- sbx breaking changes cost more than a day per release → too early to build on this
- Two weeks after Phase 1, environments aren't being created without thinking about it → this was a tool you wanted to build, not one you needed

These are much harder to apply honestly once there's code you like. That's why they're written down now.
