---
slug: gui-control-plane
title: GUI control plane over sbx
created: 2026-08-28
updated: 2026-08-28
waypoint: discuss
status: draft
document: pending
---

# GUI control plane over sbx — spec

> Discussion history, rejected alternatives and the homelab findings behind these decisions
> are in `context.md` beside this file. Read it before revisiting a design choice.
>
> This repo is public, so deployment specifics are written as placeholders — `<nas-host>`,
> `<vm-ip>`, `<internal-domain>`, `<host-port>`. The real values live in the private
> `waldemarsson/homelab` repo and in local agent memory; none of the design depends on them.

**Problem / why:** `scripts/sluss` covers the sandbox lifecycle from a terminal in the primary
checkout, but there is no way to see every sandbox across repos and sbx scopes at once, and no
way to reach an OpenCode Web session from a phone. This is convenience tooling for one person —
not a product. `docs/BACKGROUND.md`'s honest read stands and the scope reflects it.

**In scope:**

- **Fleet dashboard** — every sandbox across the configured sbx scopes, with repo, branch,
  agent, status, dirty worktree and unmerged commit count. Grouped by repo, which is the
  "Projects" concept.
- **Connect** — one click per sandbox. OpenCode opens its Web UI through sluss; Claude Code and
  GitHub Copilot get deep links to claude.ai/code and github.com, since their own apps handle
  remote steering. Terminal access is a copyable `sluss attach <name>` command, not an emulator.
- **Lifecycle** — start, stop, destroy, create. All of it shells out to `scripts/sluss`.
- **Secrets pane** — per scope: list names, set a value, delete. Write-only.
- **Kit editor** — plain-text editing of `spec.yaml` under the configured kits directory, with
  that directory's git status shown.
- **Two deployments** — laptop bound to loopback, NAS bound so a front proxy can reach it.

**Out of scope:**

- Authentication. Deferred by decision; see Constraints for what that obliges.
- A terminal emulator, chat UI, file browser or IDE. Unchanged from `docs/SPEC.md` §3.
- Agent orchestration. sluss manages environments; agents do the work.
- Profiles as a sluss concept. `appNames` in config is the seed; `docs/PROFILES.md` triggers
  still govern when the resolver gets built.
- Reimplementing worktree lifecycle in Go, while `scripts/sluss` does it correctly.
- Kubernetes, cloud, remote execution, raw Docker. sbx is the only runtime.
- TLS, certificates, DNS records, and generating configuration for anyone else's proxy.

**Chosen approach:** a Go binary serving an embedded Svelte dashboard, holding **no persistent
state of its own**. Every sandbox fact is derived per poll from `sbx --app-name X ls --json`
plus git commands against the worktree path sbx reports. A single poll loop over the configured
scopes drives the fleet table, the dashboard's SSE stream, and the proxy's routing map.

This replaces `docs/SPEC.md` §9's state files and `docs/ROADMAP.md` M2's reconciliation
milestone. There is no second copy of the truth, so there is nothing to reconcile and an
`sbx rm` by hand becomes visible within one tick. It also resolves the roadmap's open question
about what drives the SSE hub.

OpenCode reaches the phone through **one sluss port and nothing else**. sluss runs an internal
reverse proxy and addresses sandboxes itself; whatever sits in front — Traefik, Caddy, nginx, an
Incus proxy device, or nothing at all — only ever has to forward one hostname to one port, which
every reverse proxy can do and which the target already does for a dozen services.

This reverses the previous revision. Port-per-sandbox is not viable on the actual target: the NAS
VM sits on an Incus NAT bridge at `<vm-ip>`, unreachable from the LAN except through Incus
*proxy devices*, and each one is a privileged `incus config device add` on the **host**, outside
the VM sluss runs in. NAT-mode devices additionally reject a wildcard listen address. A new port
per sandbox would mean a sudo command on the NAS host plus a Traefik route for every sandbox
created — the opposite of easy. One fixed port needs one device and one route, created once.

Two addressing schemes over that single port, chosen by `access` in config:

- **`path`** (default) — `https://<host>/s/<scope>/<name>/`. Needs no DNS, no wildcard, no
  certificate and no reverse proxy at all. This is the portable mode and the one the laptop uses.
  It depends on OpenCode Web tolerating a base path; D7 records it as assuming root, which is the
  single assumption this mode rests on and is now a spike item.
- **`host`** — `https://<prefix><name>.<domain>/`, sluss routing on the `Host` header. Serves
  root path, so it works regardless of the D7 answer. Needs wildcard DNS and a matching
  certificate.

`host` mode uses a **single-label prefix**, `sluss-auth.<internal-domain>`, not
`auth.sluss.<internal-domain>`. `homelab/truenas/network.md` is explicit that a wildcard matches
exactly one label and that the DNS record and the TLS cert are separate wildcards over the same
zone. A second label would be covered by neither. With the prefix form, both the existing router-side
wildcard record and the existing `*.<internal-domain>` cert already cover every sandbox sluss
will ever create, with no DNS or certificate work per sandbox.

Nothing per sandbox is ever written to Traefik. The earlier `/traefik/routes` endpoint is gone,
and it would not have worked anyway: `homelab/truenas/compose/traefik/static/traefik.yml` runs the
**file provider only** — `providers.docker` was removed 2026-07-13 and the HTTP provider was never
enabled. Generating config for someone else's proxy also made sluss non-portable, which is the
thing this revision exists to fix.

Configuration is one hand-edited JSON file. The GUI edits kits and secrets; it does not edit
its own config.

```json
// ~/.config/sluss/config.json
{ "lan": false, "port": 8420, "access": "path",
  "hostPrefix": "sluss-", "domain": "<internal-domain>",
  "appNames": ["personal", "omegapoint"],
  "repos": ["/Users/marwal/src/mercurius", "/Users/marwal/code/private/sluss"],
  "kitsDir": "/Users/marwal/kits",
  "worktreeRoot": "/Users/marwal/src/worktrees" }
```

`lan` means "bind beyond loopback": `false` on the laptop, `true` in the NAS VM so the Incus
proxy device can reach it. `hostPrefix` and `domain` are used only when `access` is `host`.

**No reverse proxy is a dependency.** sluss ships no TLS, no DNS and no certificate handling, and
requires none of it to work. What a front proxy adds is TLS and a nice hostname, not reachability.

| | Laptop | NAS VM |
|---|---|---|
| Config | `lan: false`, `access: path` | `lan: true` |
| Dashboard | `http://127.0.0.1:8420` | `https://sluss.<internal-domain>` |
| Open a sandbox | `.../s/personal/auth/` | `.../s/personal/auth/`, or `sluss-auth.<internal-domain>` in `host` mode |
| Infrastructure | none | one Incus proxy device, one Traefik router — both once |
| Per new sandbox | nothing | nothing |

For the target specifically, that infrastructure is one device following the `300xx` convention
in `homelab/truenas/adding-an-app.md`:

```bash
sudo incus config device add sluss web proxy nat=true \
  listen=tcp:<nas-host>:<host-port> connect=tcp:<vm-ip>:8420
```

plus one router in `homelab/truenas/compose/traefik/dynamic/routes.yml` pointing
`sluss.<internal-domain>` at `http://<nas-host>:<host-port>`, identical in shape to the twelve
already there. Neither is touched again.

**Constraints:**

- `scripts/sluss` and its installed path are unchanged by this work. The Go binary calls it;
  it does not replace or modify it.
- All sbx invocation is centralised in `internal/sbx`, with app-name a parameter on every
  command. Carried verbatim from D3/D10 — it is what keeps profiles a small later addition.
- sluss parses no YAML (D6). Kit `spec.yaml` is read and written as opaque text; sbx is the
  validator. No Go structs model the kit schema, which Docker has said may change.
- `FlushInterval: -1` on sluss's proxy, no buffering middleware, no `Content-Length` on
  streamed responses (D5). Any front proxy must not buffer either — Traefik does not by
  default, but that needs confirming rather than assuming.
- **sluss binds exactly one listener.** `lan` decides its address and nothing else. Sandbox
  ports stay loopback in both access modes, which is what makes a single front-proxy route
  sufficient and keeps the exposed surface to one port.
- **Authentication is deferred, inheriting the homelab's existing and documented stance.**
  `<internal-domain>` exists only in the router's local DNS and is never published publicly;
  `homelab/truenas/instances/README.md` records the same accepted risk for the current opencode
  VM — "you are on the LAN is the whole auth story, and it is only reachable only over the VPN" —
  together with a standing instruction never to port-forward its port. sluss inherits both. The
  conditions that homelab names for revisiting apply unchanged here: reachable from outside the
  VPN, or untrusted devices on the LAN.
- sluss emits no configuration for any external proxy, and knows the name of none. Everything it
  serves is on its own port.
- Secret values are never written to sluss's disk, never rendered into a page, and never used
  to populate a form field. Only names are read back. This preserves D1's property that raw
  keys stay behind sbx's credential proxy.
- Go, plus SvelteKit with adapter-static embedded via `go:embed` (D2, D8). Runtime dependencies
  on the target: `sbx`, `git`, and `scripts/sluss`.

**Acceptance criteria:**

- [ ] AC1: With two sandboxes running in different sbx scopes, the dashboard lists both with
      correct repo, branch, agent, status, dirty flag and unmerged commit count.
- [ ] AC2: `sbx rm -f <name>` run by hand is reflected in the dashboard within one poll
      interval, and its route stops resolving. Nothing crashes.
- [ ] AC3: With `lan: false`, nothing sluss controls is reachable from another host.
- [ ] AC4: On a laptop with no reverse proxy, no wildcard DNS and no certificate anywhere,
      `access: path` reaches every sandbox's OpenCode Web UI. Zero infrastructure is a supported
      configuration, not a degraded one.
- [ ] AC5: In the NAS VM behind one Incus proxy device and one Traefik router, a phone on
      the VPN opens a sandbox over TLS and streams tokens at TUI speed.
- [ ] AC6: Creating and destroying sandboxes changes nothing outside sluss — no Incus device, no
      Traefik route, no DNS record, no config file anywhere on the NAS host is added, edited or
      removed, and the new sandbox is reachable within one poll interval.
- [ ] AC7: Switching `access` between `path` and `host` changes only how URLs are formed. Both
      modes serve every sandbox over the same single port.
- [ ] AC8: A Claude Code sandbox and a Copilot sandbox each show a working deep link to their
      own remote interface, and neither is proxied.
- [ ] AC9: Creating a sandbox from the GUI produces the same worktree, branch and mounts as
      running `sluss start` in a terminal, because it is the same code path.
- [ ] AC10: Destroy from the GUI refuses dirty or unmerged work, surfacing the script's refusal
      rather than bypassing it.
- [ ] AC11: Setting a secret makes its name appear in the pane for that scope only; the value is
      never returned by any endpoint and does not appear in any log.
- [ ] AC12: Editing a kit's `spec.yaml` in the browser writes the file byte-for-byte as typed,
      and the kits directory shows as dirty until committed by hand.
- [ ] AC13: `sluss doctor` reports the bound address, whether it is loopback or exposed, the
      access mode and the URL form it produces, sbx presence and version, and each configured
      scope's reachability.

**Edge cases:**

- A sandbox whose worktree path no longer exists on disk — list it, mark it, do not crash.
- A configured app-name that has never been used: sbx starts a fresh daemon and prints a
  first-run banner before answering (SPIKE 5). The scope must appear empty, not errored.
- A stopped sandbox reports no `ports` at all (SPIKE 4). Its row renders without a connect
  link rather than a broken one.
- Two sandboxes in different scopes sharing a name — SPIKE 5 has not established whether names
  collide across app-names. Keys must be scope-qualified regardless.
- A sandbox name that is not a valid DNS label in `host` mode, or collides with another scope's
  sandbox of the same name once the scope is dropped from the hostname. `path` mode carries the
  scope in the URL and has neither problem, which is part of why it is the default.
- A request for a sandbox that has since disappeared: 404 from sluss, not a hang against a dead
  backend.
- The configured port is already taken, or `lan: true` cannot bind: exit naming the address
  tried. Never fall back to a different address — a NAS that silently binds loopback looks
  healthy and is unreachable.
- A repo listed in config that is not a git checkout, or a `kitsDir` that is not a git repo.
- A poll that outlives its interval: skip the tick, never stack them.
- `scripts/sluss` missing or older than the GUI expects — `doctor` reports it.
- `access: host` with `domain` unset: refuse to start.

**Affected areas (current code):**

- `cmd/sluss/main.go` — currently a 31-line stub that exits non-zero. Becomes the real entry
  point.
- `scripts/sluss` — **unchanged.** Invoked as a subprocess with cwd set to the chosen repo and
  `SLUSS_APP_NAME` / `SLUSS_AGENT` / `SLUSS_WORKTREE_ROOT` in its environment.
- `web/dashboard/` — SvelteKit app; `build/.gitkeep` already committed so `go:embed` survives a
  fresh clone.
- `spike/proxy/main.go` — the throwaway SSE proxy from M0. Its findings feed `internal/proxy`,
  which this design does need.
- `docs/SPEC.md`, `docs/ROADMAP.md` — superseded by this spec; rewritten at document time.
- `docs/DECISIONS.md` — gains entries for statelessness, single-port routing with two
  addressing modes, deferred auth, and shelling out to the script. D3, D4, D5, D6, D9 carry
  forward unchanged; §7's `*.sluss.localhost` scheme is superseded.
- `docs/SPIKE.md` — assumption 6 answered 2026-08-28 (nested virt works on the target).
  Assumptions 2, 3 and 7 remain open; 5 is partial.

**Open questions:**

Answered by spiking, not deciding. Sequencing them is `plan`'s job; what each one gates is
named in words rather than by milestone number, since no milestone list exists yet.

- **Does OpenCode Web work under a base path?** D7 records it as assuming root. If it does,
  `path` mode is the portable default and needs nothing from anyone's infrastructure. If it does
  not, `host` mode becomes mandatory for OpenCode and wildcard DNS returns as a requirement —
  already satisfied on the target, but no longer free elsewhere. Highest-value check in the
  list — it decides the default access mode, so answer it before building the proxy.
- **SPIKE 3 — is SSE through a reverse proxy fast enough?** Load-bearing again, since sluss now
  proxies. Traefik is also in the path on the NAS, so its buffering needs the same scrutiny D5
  demanded of ours.
- **Can the VM's `agent` user actually open `/dev/kvm`?** Nested virtualisation is confirmed
  working on the target (SPIKE 6, answered 2026-08-28: host `kvm_amd nested=1`, guest sees `svm`,
  `/dev/kvm` present) — but the device is `root:kvm` mode 0660 and `homelab`'s `cloud-init.yaml`
  adds `agent` to `docker` only. `groups agent` settles it; `usermod -aG kvm agent` fixes it.
- **SPIKE 2 — is `--kit` repeatable?** Gates only the environment × toolset compose UI in the
  create form. A single-kit picker works either way; "one environment plus N toolsets" has no
  mechanism without it.
- **What subcommands does `sbx kit` have?** Recorded in the CLI surface with no detail. If it
  offers validate, the kit editor's validate button is free. Gates the kit editor.
- **What is `sbx secret`'s exact surface, and does `secret ls` return names only?** The
  write-only design depends on it. Gates the secrets pane.
- **SPIKE 5's unfinished half** — set a secret in one scope, `secret ls` in another. The
  multi-scope design assumes that boundary is real; the recorded answer establishes only that
  `--app-name` starts a separate daemon.
