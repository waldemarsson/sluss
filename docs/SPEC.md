# sluss — Spec v1.0

> **Superseded in part. History, not instructions.** This document describes the design as it
> stood while sluss was two things: a `sluss` shell script owning lifecycle, and a `slussd` daemon
> shelling out to it. Both are gone — there is now one binary named `sluss`, with lifecycle in
> `internal/lifecycle` (docs/DECISIONS.md **D23** and **D24**). Everything else here — the
> division of labour with sbx, the stateless design, the single-port routing, the two addressing
> modes — still holds. For what sluss does today, read [README.md](../README.md).

> A control plane over Docker Sandboxes (sbx): one daemon that shows every sandbox across repos
> and sbx scopes, reaches each OpenCode Web UI through a single port, and drives the sandbox
> lifecycle.

**Status:** implemented. Two spikes remain open and are named in §13 — until the first is
answered, path mode is the default but unproven against OpenCode Web.

**MVP scope note:** per-client profiles are deliberately deferred. The design is preserved in
`PROFILES.md` with trigger conditions for building it. Until then, sbx's `--app-name` and `--kit`
flags are used manually.

---

## 1. Problem

sluss's command line covers the sandbox lifecycle from a terminal in the primary checkout, but
there is no way to see every sandbox across repos and sbx scopes at once, and no way to reach an
OpenCode Web session from a phone.

This is convenience tooling for one person, not a product. `BACKGROUND.md`'s honest read stands.

## 2. Product statement

One page showing every sandbox, grouped by repository, with the facts that decide what to do next —
branch, agent, status, dirty worktree, unmerged commits. One click reaches the agent. Lifecycle
buttons run the same code a terminal would.

## 3. Non-goals

- A chat UI, IDE, file browser, or terminal emulator
- Multi-agent orchestration or inter-agent coordination
- Kubernetes, cloud deployment, remote execution, raw Docker
- Reimplementing anything sbx already does
- A second implementation of worktree lifecycle anywhere (D23 made `internal/lifecycle` the one)
- Per-client profile management (deferred — see `PROFILES.md`)
- TLS, certificates, DNS records, or generating configuration for anyone else's proxy

---

## 4. Layer map

| Concern | Owner |
|---|---|
| microVM isolation, kernel boundary | sbx |
| Agent install and launch | sbx (`sbx create opencode`) |
| Secret storage, credential injection | sbx (`sbx secret`) |
| Network egress policy | sbx (`sbx policy`) |
| Environment tools, files, standards | sbx kits |
| Identity scoping (`--app-name`), kits | sbx — invoked manually in the MVP |
| **Git worktree lifecycle** | **sluss** (`internal/lifecycle`) |
| **Fleet view, HTTP routing, dashboard** | **sluss** (`internal/server`) |

sluss delegates everything above those two rows to sbx and reimplements none of it.

## 5. One binary

*Superseded: this section described two binaries, `sluss` (a shell script) and `slussd` (the Go
daemon that ran it as a subprocess). D24 collapsed them.*

| | What it is | Who runs it |
|---|---|---|
| `sluss` | the Go binary (`cmd/sluss`) — lifecycle commands, dashboard, API, reverse proxy | a person in a terminal, and systemd on a server |

The command line and the dashboard call the same functions in the same process, so a sandbox
created from the browser and one created in a terminal are identical by construction.

## 6. No state

sluss persists nothing. Every sandbox fact is derived per poll from
`sbx --app-name <scope> ls --json` plus git commands against the worktree path sbx reports. One
poll loop over the configured scopes produces an immutable snapshot that drives the fleet table,
the SSE stream, the proxy's routing map and `doctor`.

There is no second copy of the truth, so there is nothing to reconcile: an `sbx rm` by hand
becomes visible within one tick and its route stops resolving. Repository and worktree come from
the two workspaces sluss mounts — the entry ending in `.git` is the repository, the other is
the worktree.

## 7. Routing — one port, two addressing modes

sluss binds exactly one listener. Sandbox ports stay on loopback in both modes, which is what
keeps a front proxy to a single route.

| `access` | URL | Needs |
|---|---|---|
| `path` (default) | `http://<host>/s/<scope>/<name>/` | nothing — no DNS, no certificate, no proxy |
| `host` | `https://<prefix><name>.<domain>/` | wildcard DNS and a matching wildcard certificate |

`host` mode uses a **single-label** prefix (`sluss-<name>.<domain>`, not
`<name>.sluss.<domain>`): a wildcard DNS record and a wildcard certificate each match exactly one
label, so a second label is covered by neither. Because the scope is not in the hostname, a name
that exists in two scopes is refused rather than routed arbitrarily; `path` mode carries the scope
and has no such problem.

`FlushInterval: -1` on the proxy, no buffering middleware, no `Content-Length` on streamed
responses. Any front proxy must not buffer either.

Only OpenCode is proxied. Claude Code and GitHub Copilot get deep links to `claude.ai/code` and
`github.com`, because their own apps handle remote steering. Terminal access is a copyable
`sluss attach <name>` command, not an emulator.

## 8. Configuration

One hand-edited JSON file, never written by sluss. Unknown keys are refused, so a typo is an error
rather than a silently ignored setting.

```json
// ~/.config/sluss/config.json
{ "lan": false, "port": 8420, "access": "path",
  "hostPrefix": "sluss-", "domain": "<internal-domain>",
  "appNames": ["personal", "omegapoint"],
  "repos": ["/Users/marwal/src/mercurius"],
  "kitsDir": "/Users/marwal/kits",
  "worktreeRoot": "/Users/marwal/src/worktrees" }
```

`lan` means "bind beyond loopback" and decides the bind address and nothing else: `false` binds
`127.0.0.1`, `true` binds every interface. A failure to bind is fatal and names the address tried —
sluss never falls back to a different one, because a NAS that quietly bound loopback would look
healthy and be unreachable. `hostPrefix` and `domain` apply only when `access` is `host`;
`access: host` without a `domain` refuses to start.

Defaults: port `8420`, access `path`, host prefix `sluss-`, worktree root `~/src/worktrees`.

## 9. HTTP surface

| Route | Purpose |
|---|---|
| `GET /` | the embedded dashboard |
| `GET /api/config` | access mode, host prefix, domain, configured repos and scopes |
| `GET /api/fleet` | the current snapshot |
| `GET /api/events` | SSE; one event per snapshot, plus a heartbeat |
| `POST /api/sandboxes` | create/start — runs the script in the chosen repository |
| `POST /api/sandboxes/{scope}/{name}/stop` | stop |
| `DELETE /api/sandboxes/{scope}/{name}` | destroy — never forced |
| `GET /api/secrets/{scope}` | secret names only |
| `PUT`/`DELETE /api/secrets/{scope}/{name}` | set a value / remove a name |
| `GET /api/kits` | kit list plus the kits directory's git status |
| `GET`/`PUT /api/kits/{name}/spec` | `spec.yaml` as opaque bytes |
| `/s/{scope}/{name}/…` | the proxy, in `path` mode |

Lifecycle requests are detached from the connection that started them: closing the browser mid-run
must not kill the script between creating a worktree and creating the sandbox.

## 10. Dashboard

SvelteKit with adapter-static, embedded via `go:embed`, served by the same binary. The fleet table
is grouped by repository — that grouping is the "Projects" concept. Lifecycle buttons post to the
routes above and show the script's refusal verbatim when it declines.

## 11. Security posture

- **Authentication is deferred by decision**, inheriting the homelab's existing stance:
  `<internal-domain>` exists only in local DNS, and the deployment is LAN- and VPN-only. Revisit
  when it is reachable from outside the VPN, or when untrusted devices join the LAN.
- **That premise assumes reachability is the boundary, and a browser breaks it**, since any page
  can send requests to `127.0.0.1`. Mutating routes therefore reject a cross-site
  `Sec-Fetch-Site` and require `application/json` on POST, which is the only shape a cross-origin
  request takes without a preflight. This closes the drive-by path; it is not authentication and
  does not pretend to be.
- **Secret values are write-only**: piped to sbx on stdin, never in argv, never returned by any
  endpoint, never used to populate a form field.
- **Only configured repositories and scopes are reachable**, and `sbx create` accepts only the
  sizing flags the create form offers — an unfiltered list could mount an arbitrary host path into
  a sandbox.
- **Destroy is never forced.** The script refuses dirty or unmerged work and that refusal is what
  the user sees; discarding work stays a deliberate terminal action.

## 12. Deployment

| | Laptop | NAS VM |
|---|---|---|
| Config | `lan: false`, `access: path` | `lan: true` |
| Dashboard | `http://127.0.0.1:8420` | `https://sluss.<internal-domain>` |
| Open a sandbox | `…/s/personal/auth/` | the same, or `sluss-auth.<internal-domain>` in `host` mode |
| Infrastructure | none | one Incus proxy device, one Traefik router — both created once |
| Per new sandbox | nothing | nothing |

Nothing per sandbox is ever written to Traefik, and sluss knows the name of no external proxy.
What a front proxy adds is TLS and a nice hostname, not reachability.

## 13. What is not yet verified

- **Does OpenCode Web work under a base path?** D7 records it as assuming root. If it does not,
  `path` mode cannot carry OpenCode and `host` mode becomes mandatory. Highest-value check in the
  project; see `SPIKE.md` assumption 8.
- **SSE through the proxy at TUI speed** (`SPIKE.md` assumption 3), with Traefik in the path on the
  NAS.
- **`sbx secret` and `sbx kit` surfaces** (`SPIKE.md` assumption 5's unfinished half). The secrets
  pane assumes `secret ls` returns names and `secret set` reads stdin; if either is wrong,
  `internal/sbx` is the only file that changes.
