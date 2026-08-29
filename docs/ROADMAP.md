# sluss — Roadmap

Where the work stands and what is left. `SPEC.md` describes *what* sluss is; this describes *what
is done*, *what blocks the rest*, and *where the honest stopping points are*.

**Status:** implemented and tested against a stubbed sbx. Nothing has yet run against a real
sandbox, and the items below need hardware the devcontainer does not have.

---

## Done

| Milestone | Deliverable |
|---|---|
| **Lifecycle** | `internal/lifecycle` — worktree, branch, sandbox, attach, exec, stop, destroy, with the create-failure unwind and the dirty/unmerged destroy refusals. The one implementation; the command line and the dashboard both call it (D23). |
| **Fleet** | `internal/sbx` (the one place sbx is invoked), `internal/gitfacts`, `internal/fleet` — a stateless poll loop producing an immutable snapshot per tick. |
| **Serving** | `internal/server` (dashboard, JSON, SSE, lifecycle, secrets, kits) and `internal/proxy` (one port, `path` and `host` addressing, `FlushInterval: -1`). |
| **Dashboard** | SvelteKit, adapter-static, embedded via `go:embed`. The fleet and create form at `/`, secrets and kits at `/config/…` behind a top bar (D19). Built for a phone: the fleet becomes a card per sandbox and the create form collapses below `48rem`. |
| **Command** | One binary, `sluss` (D24): `start`, `list`, `attach`, `exec`, `stop`, `destroy`, `path`, `serve`, `doctor`, `update`, `version`. |
| **Distribution** | GitHub releases from a `v*` tag for darwin and linux on arm64 and amd64, a checksum-verifying `scripts/install.sh`, `scripts/uninstall.sh`, and self-update through `sluss update`. |

The milestones this replaces are worth naming, because their absence is deliberate: **M2's
reconciliation** is gone because there is no state to reconcile (D12), and **M3's token auth** is
deferred rather than built (D15).

---

## Open — needs the host Mac or the NAS

These are answered by running them, not by deciding.

| # | Question | Blocks |
|---|---|---|
| 1 | **Does OpenCode Web serve under a base path?** (`SPIKE.md` 8) | whether `path` mode can stay the default. A "no" is a spec revision, not a workaround. |
| 2 | **SSE through the proxy at TUI speed** (`SPIKE.md` 3), Traefik included | the premise of the whole project (`SPEC.md` §13) |
| 3 | **`sbx secret` / `sbx kit` surface** (`SPIKE.md` 5) | the secrets pane's exact argv — one file, `internal/sbx` |
| 4 | **Deployment acceptance** — laptop with zero infrastructure, then the NAS VM behind one Incus proxy device and one Traefik router, opened from a phone over the VPN | calling any of this finished |

Item 4 also needs `groups agent` to include `kvm` in the VM (`SPIKE.md` 6): `/dev/kvm` is
`root:kvm` mode 0660 and `cloud-init.yaml` adds `agent` to `docker` only.

---

## Kill criteria, still live

- SSE unpleasant after tuning → delete the proxy and use `sbx tui`. Very little of sluss remains,
  and that is a legitimate outcome.
- sbx breaking changes costing more than a day per release → too early to build on this.
- **The gate that matters:** *are sandboxes being created without thinking about it?* If they are
  not after a couple of weeks of real use, this was a tool worth building rather than one worth
  needing — keep the lifecycle commands, drop the dashboard.

---

## Structural invariants

Repeated from `AGENTS.md` and `DECISIONS.md` because they are cheap now and expensive later:

- **All sbx invocation lives in `internal/sbx`,** app-name a parameter on every command (D3, D10).
- **Lifecycle lives in `internal/lifecycle`,** and only there. The command line and the HTTP routes call the same functions in the same process (D23, superseding D14).
- **Every worktree sandbox also mounts the main repository's `.git` directory writable** (SPIKE 1, D4).
- **`FlushInterval: -1` is load-bearing** (D5). No buffering middleware, no `Content-Length` on
  streamed responses.
- **sluss holds no state** (D12), and **parses no YAML** (D6).
- **Profiles stay deferred** until a trigger in `PROFILES.md` fires (D10).
