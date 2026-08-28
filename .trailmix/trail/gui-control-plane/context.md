# Discussion context — gui-control-plane

Not a trailmix artifact. Decision history behind `spec.md`, written to survive a context
reset. `spec.md` is what the work follows; this is why it says what it says.

## Where things stand (2026-08-28)

- Branch **`app/gui-control-plane`**, cut from `main` (`b9fb844`). Note: `script/update-command`
  carries `f3f41a9` "Add sluss update to install the latest script", which is **not** in `main`
  and therefore not on this branch. Unrelated to this work; merge it separately.
- Uncommitted: `docs/SPIKE.md` — assumption 6 answered (nested virt works on the NAS target).
- `.trailmix/` is gitignored, so `spec.md` and this file are on disk only, not tracked.
- Waypoint: `discuss` complete, spec awaiting sign-off. **`plan` is next.**
- Toolchain verified in the devcontainer: Go 1.27, Node 22, task 3.53, module
  `github.com/waldemarsson/sluss`. `sbx` is absent here by design — anything touching a real
  sandbox runs on the host. `scripts/test-sluss` already black-box tests against a stubbed sbx;
  reuse that harness rather than inventing one.


## Starting point

The user proposed a broader app spec (Projects / Sandboxes / Environments / Toolsets / Agents /
Worktrees / Secrets / Mounts / Lifecycle / Access) and asked for a review of it against the
committed docs, plus the future of the Go app. Constraint given up front and honoured
throughout: **do not change the current script path** — `scripts/sluss` is untouched by this
work.

Review findings against `docs/` v0.5, in short: the proposal was 3–5× the committed scope,
dropped profiles (the one differentiator `BACKGROUND.md` names), contradicted §3 non-goals on
terminals and multi-agent, and rested on `SPIKE.md` assumptions 2/3/6/7 that are still blank.
M0's verdict paragraph was never written and the M1c gate never ran.

## What the user decided

| Question | Answer |
|---|---|
| Ambition | Convenience for himself, at least initially. Not a product. |
| Runtime | sbx, not raw Docker. D1 stands. |
| GUI | Browser dashboard, laptop **and** NAS, reachable from phone |
| Phone target | Tap a sandbox running OpenCode Web, get it in the browser |
| Claude / Copilot | Managed through their own apps — sluss only does lifecycle + deep links |
| Config editing | GUI for secrets, environments, toolsets; "very simple, like a YAML editor" |
| Auth | Deferred. Do not build it now. |
| App scopes | Several, listed in config |
| Projects | Config lists repo paths explicitly |
| Kits | One kits directory, which is a git repo; user commits by hand |

## Design decisions, and what they replaced

**Stateless.** Everything derives per poll from `sbx --app-name X ls --json` plus git against
the worktree path sbx reports. Kills `docs/SPEC.md` §9's state files and roadmap M2's
reconciliation. Also answers the roadmap's open "what drives the SSE hub" question: one poll
loop feeds the fleet table, the SSE stream, and the proxy's routing map.

**Create/destroy shell out to `scripts/sluss`.** 669 lines that already do worktrees, the
unmerged-work guard and the unwind path. Not reimplemented in Go.

**No terminal.** Claude and Copilot have their own apps, OpenCode has its web UI; what's left
is debugging from a phone. "Help me connect" is a copyable `sluss attach <name>` plus deep
links. Stays a §3 non-goal.

**Secrets are write-only**, never an editor. Names read back from `sbx secret ls`; values go to
`sbx secret set` and are never rendered, logged, persisted, or used to populate a field.
Preserves D1's credential-proxy property.

**Kit editor is opaque text.** Reads and writes `spec.yaml` as bytes, sbx validates. Preserves
D6 and avoids modelling a schema Docker has said may change.

## Routing — three revisions, keep the third

The single most-revised decision. Recorded because two dead ends look reasonable in isolation.

1. **Wildcard DNS + host-header on `*.sluss.localhost`** (`docs/SPEC.md` §7). Dead: a phone
   will not resolve it.
2. **Port per sandbox, hashed, routed by the existing Traefik via an HTTP-provider endpoint.**
   Dead on contact with the real target — see below.
3. **One sluss port, sluss proxies internally.** Whatever is in front forwards one hostname to
   one port. Two addressing modes: `path` (`/s/<scope>/<name>/`, needs no infrastructure at
   all, the default) and `host` (Host header, serves root path).

Why 2 died, from `waldemarsson/homelab` (private; read with `gh api`):

- `truenas/instances/README.md` — the VM is on an Incus NAT bridge at `<vm-ip>`,
  reachable from the LAN only through Incus **proxy devices**. Each is a privileged
  `incus config device add` on the *host*, outside the VM sluss runs in. NAT mode also rejects
  a wildcard listen address. A port per sandbox = a sudo command on the NAS host per sandbox.
- `truenas/compose/traefik/static/traefik.yml` — **file provider only**. `providers.docker`
  removed 2026-07-13, HTTP provider never enabled. The endpoint had no consumer.
- Generating config for someone else's proxy also made sluss non-portable, which the user
  explicitly rejected: "I don't want traefik to be a hard dependency."

`host` mode uses a **single-label** prefix — `sluss-auth.<internal-domain>`, never
`auth.sluss.<internal-domain>`. `truenas/network.md`: a wildcard matches exactly one label,
and the router-side DNS record and the TLS cert are separate wildcards over the same zone. The nested
form is covered by neither; the prefix form by both, already.

Target infrastructure is now one Incus proxy device (`<host-port>` → VM `8420`, following the `300xx`
convention) and one Traefik router, both created once and never touched again.

## Auth — a correction worth not repeating

I twice escalated the risk of exposing sluss without auth, the second time on the assumption
that `<internal-domain>` was internet-facing. It is not: router-side local DNS only, never published,
reachable only over the VPN. `truenas/instances/README.md` already records this exact accepted risk
for the existing opencode VM, along with the conditions for revisiting it. The spec inherits that
stance by reference. **Do not re-litigate it.**

## Open questions — all gate a milestone

1. **Does OpenCode Web work under a base path?** D7 says it assumes root. Decides whether `path`
   can be the default. Highest value.
2. **SPIKE 3** — SSE through a proxy. Load-bearing again now that sluss proxies. Traefik is also
   in the path on the NAS.
3. ~~**Nested virt in an Incus QEMU VM.**~~ **Answered 2026-08-28: works.** Host
   `kvm_amd nested=1`, guest sees `svm`, `/dev/kvm` present. Recorded in `docs/SPIKE.md` 6.
   Residual: `agent` must be in the `kvm` group — `cloud-init.yaml` adds it to `docker` only.
   VM sizing is an operator concern, not a sluss design input.
4. **SPIKE 2** — is `--kit` repeatable? Gates only the environment × toolset compose UI.
5. **`sbx kit` and `sbx secret` surfaces** — undocumented in the recorded CLI notes.
6. **Does `opencode web` find the sbx-mounted workspace?** Its picker lists HOME's immediate
   children only, ignoring symlinks — a quirk that cost an abandoned per-project design in
   homelab.

## Deleted from the committed docs

State files, reconciliation, wildcard DNS, the token gate, the terminal emulator, and
`*.sluss.localhost`. `docs/SPEC.md` and `docs/ROADMAP.md` are superseded but not yet rewritten —
that happens at the document waypoint, after something ships.
