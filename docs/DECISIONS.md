# Decision log

Rationale for choices that look arbitrary from inside the code, and the alternatives already rejected. If you're about to argue for one of the rejected options, read the reason first — it may still be wrong, but it wasn't unconsidered.

---

## D1 — Build on sbx rather than raw Docker

**Chosen:** Docker Sandboxes (`sbx`) as the isolation primitive.

**Why:** sbx gives a microVM per sandbox — separate kernel, separate Docker daemon, separate filesystem — plus a host-side credential proxy so the agent never sees raw API keys, deny-by-default network policy, declarative kits, skills, a secret store, and first-class OpenCode support. An earlier version of this spec hand-rolled hardened Docker containers; roughly 70% of it was deleted when sbx turned out to ship the same thing.

**Rejected:**
- *Hardened Docker containers* — weaker boundary (shared kernel), and everything would need building.
- *nono* (Landlock/Seatbelt) — elegant and Apache-2.0, but sandboxes processes, not environments. No fleet.
- *Firecracker/OpenSandbox direct* — fleet-scale, far more than needed.

**Cost accepted:** sbx is experimental, Docker says the kit format and CLI may change, and it requires a Docker account. Mitigated by keeping sluss a thin shell-out layer with no SDK coupling.

---

## D2 — Go, not C#

**Chosen:** Go.

**Why:** single-command cross-compilation to the NAS target, `go:embed` for dashboard assets, small static binaries, and the whole surrounding ecosystem (sbx, container-use, the kits TCK) is Go. Also an explicit decision to learn the language on a well-scoped project.

**Rejected:** C#, despite the author knowing it well. `dotnet publish --self-contained` does cross-publish, so that advantage was smaller than first stated. The learning-curve cost is real and accepted deliberately.

**Consequence:** expect Phase 1 to take longer than the estimate implies.

---

## D3 — Profiles = `--app-name` + kit (DEFERRED)

**Status: not in the MVP.** Full design and trigger conditions in `PROFILES.md`. The reasoning below stands; only the timing changed.

**Chosen:** each profile maps to an sbx app-name (boundary) plus a kit (content).

**Why:** `--app-name` scopes daemon state, login, and the secret store, so two profiles genuinely cannot see each other's credentials. Kits carry the rest — network allowlist, injected files, credential sources, and a rendered memory file for coding standards.

**Rejected:**
- *Templates* — `sbx template` is snapshot-shaped (save/reload a configured sandbox). That's state, not config. Kits are the declarative artifact.
- *Profiles as sluss-only convention* — an earlier draft. Discarded once `--app-name` was found to be a real boundary.

**Invariant (applies even in the MVP):** all sbx invocation is centralised in `internal/sbx`, with app-name a parameter on every command. Scattering sbx calls would make adding profiles later a hunt instead of a small change.

---

## D4 — Host worktrees, not in-sandbox clones

**Chosen:** a git worktree per environment on the host, mounted into the sandbox together
with the main repository's `.git` directory as a second writable workspace.

**Why:** sbx syncs the mounted workspace bidirectionally, so the host sees agent changes
live and review is ordinary `git diff`. A linked worktree's `.git` file points into the
main repository; Git commands fail in the sandbox unless that metadata is mounted too.
The spike confirmed two sandboxes can share the metadata mount while using separate
worktrees and branches. This intentionally gives each sandbox writable access to the
repository's shared objects and refs. One worktree directory can only be mounted by one
sandbox, so parallel environments still need separate worktrees.

**Rejected:**
- *`git ext::` transport over `docker exec` / `sbx exec`* — a clever earlier design for extracting work from a fully isolated container. Obsolete once bidirectional sync was confirmed. Do not resurrect it.
- *Clone-into-sandbox as the default* — stronger isolation but loses live host visibility. Kept as `--isolated` in v0.5, then **dropped from the MVP**: without the `ext::` transport there is no way to get work back out of a cloned sandbox, so the flag stranded it. Reintroducing clone mode requires designing the sync step first. See `ROADMAP.md`.

---

## D5 — Embedded reverse proxy, not Traefik

**Chosen:** `httputil.ReverseProxy` inside sluss.

**Why:** keeps the binary self-contained, puts SSE flushing under direct control, and keeps routing state in one process. Critically, sandboxes are microVMs under `sandboxd`, **not containers on the host Docker daemon** — so Traefik's Docker label provider cannot discover them. Label-based routing was in an earlier draft and is wrong.

**On the NAS:** existing Traefik goes in front of `:8420` for TLS termination only.

**Non-negotiable:** `FlushInterval: -1`. Without it, OpenCode's SSE stream buffers and the web UI feels broken.

---

## D6 — JSON for sluss state, YAML only at the sbx boundary

**Chosen:** sluss parses no YAML. State and profiles are JSON. The `.sbxenv.yaml` overlay is emitted, either as JSON (YAML is a JSON superset) or a text template.

**Why:** one serialiser, one code path, no ambiguity about which format a file uses.

**Exception:** kit `spec.yaml` files are hand-authored YAML, live in git, and get shared. sluss references them by path and never generates them.

---

## D7 — Reuse OpenCode Web, don't build chat

**Chosen:** the dashboard launches into OpenCode's existing web UI.

**Why:** the environment is the product; the chat UI is replaceable. Building one is a category error and an enormous amount of work.

**Known friction:** OpenCode's web UI assumes root path, OAuth callbacks bind to loopback inside the sandbox (prefer API keys or device-code flow), and SSE through a proxy has been reported as slow (hence D5).

**Update:** the root-path assumption is now load-bearing rather than a note. D13 makes
`/s/<scope>/<name>/` the default addressing mode, so whether OpenCode Web tolerates a base path
decides whether the zero-infrastructure deployment works. It is recorded but **unverified** —
`SPIKE.md` assumption 8. `host` mode serves root and is the fallback if the answer is no.

---

## D8 — Dashboard in Svelte, embedded

**Chosen:** SvelteKit with adapter-static, embedded via `go:embed`, served by the same binary.

**Why:** the author knows Svelte. The build produces arch-independent assets, so one `npm run build` feeds both Go targets and the single-binary property survives.

**Rejected:** Go templates + htmx. Simpler and keeps the build at one command, but the author prefers Svelte and the extra step is a Taskfile target, not a wall.

**Why a dashboard at all, given `sbx tui` exists:** sbx's TUI is scoped per app-name and cannot show a cross-profile view. The valuable columns — unmerged commits, dirty worktree — come from the host worktree, which only sluss knows about.

---

## D9 — Scope discipline

The competitive landscape for parallel-agent tooling is crowded and has a high mortality rate (Bloop shut down April 2026, Crystal deprecated February 2026, Codeanywhere sunsetting). Sculptor, Conductor, Claude Squad, Vibe Kanban, Emdash, container-use and others overlap heavily.

This project is justified only by a narrow gap: OpenCode as the agent, browser-reachable from any device, with per-client identity separation. Everything else should be delegated or dropped.

**Practical rule:** before adding a feature, check whether sbx already does it. It probably does.

---

## D10 — Defer profiles to post-MVP

**Chosen:** ship without profile management. Pass `--app-name` and `--kit` manually.

**Why:** the mechanisms belong to sbx, so credential isolation is fully available today by typing the flags. What sluss would add is ergonomics and mistake-prevention, not capability. Deferring cuts Phase 1 from a month to a weekend or two, and time-to-first-use is the single biggest predictor of whether a side project survives.

It is also cleanly additive — adding the resolver later needs no refactor, provided D3's invariant about centralised sbx invocation holds.

**Cost accepted:** you can forget a flag and create a sandbox in the wrong scope, producing commits under the wrong identity. Recoverable, annoying. Shell aliases per client mitigate most of it.

**Revisit when:** any trigger in `PROFILES.md` fires — notably, getting it wrong twice.

**Honest note:** profiles were the strategically interesting part of the concept, the one gap nothing else in the landscape fills. Without them this is convenience glue over sbx. That's a fine thing to build for yourself; it just isn't a product, and the scope now reflects that.

---

## D11 — Name: sluss

**Chosen:** `sluss` — Swedish for airlock or canal lock.

**Why:** an enclosed chamber that things pass through in isolation, which is exactly what an environment is. The verb form *slussa igenom* means to route or channel something through, so the metaphor covers both the isolation and the proxy. Five characters, no diacritics, types cleanly, and reads as a real word rather than an acronym.

**Checked:** no software project by that name found. Phonetic neighbours are `slush` (a dormant npm scaffolding tool) and `slurm` (the HPC scheduler) — close enough to note, not close enough to matter.

**Rejected:**
- *agentctl* — the working name. Generic, and `-ctl` implies a Kubernetes lineage this has nothing to do with.
- *helo* — short and types well, but phonetically identical to "hello", which makes it unsearchable.
- *viv* (vivarium) — the AI namespace already has Viv Labs, and `viur-cli`/`vivid-cli`/`vivify-cli` crowd the search results.
- *kruka* (flowerpot) — fits the kaktus.nu domain nicely, but also means "coward" in colloquial Swedish.

---

## D12 — No state of sluss's own

**Chosen:** derive every sandbox fact per poll from `sbx --app-name X ls --json` plus git commands
against the worktree path sbx reports. Nothing is persisted.

**Why:** state files drift from reality the moment someone runs `sbx rm` by hand, and the fix for
that drift — reconciliation — is a milestone's worth of work to keep a second copy of something sbx
already knows. With one copy there is nothing to reconcile: an external removal is visible within a
tick and its route stops resolving. The repository and worktree come out of the workspaces the
script already mounts, so even that is not stored.

**Supersedes:** `SPEC.md` v0.5 §9 (state files) and the roadmap's M2 reconciliation milestone.

**Cost accepted:** a poll per interval per scope, and a few git processes per sandbox behind it
(capped at eight concurrently). At one person's fleet size this is noise; it would not be at a
hundred sandboxes.

---

## D13 — One port, two addressing modes

**Chosen:** sluss binds a single listener and routes to sandboxes itself, addressing them either by
path (`/s/<scope>/<name>/`) or by host (`<prefix><name>.<domain>`). Sandbox ports stay on loopback.

**Why:** port-per-sandbox is not viable on the actual target. The NAS VM sits on an Incus NAT
bridge, reachable only through Incus *proxy devices* — each one a privileged command on the host,
outside the VM sluss runs in, and NAT-mode devices reject a wildcard listen address. A port per
sandbox would mean a sudo command on the NAS plus a Traefik route for every sandbox created. One
fixed port needs one device and one route, made once and never touched again.

`path` mode is the default because it needs nothing at all: no DNS, no wildcard certificate, no
reverse proxy. `host` mode serves root, which is the fallback if OpenCode Web turns out not to
tolerate a base path (D7).

**Single-label prefix** (`sluss-auth.<domain>`, not `auth.sluss.<domain>`): a wildcard DNS record
and a wildcard TLS certificate each match exactly one label, so a two-label name is covered by
neither, and the existing wildcards already cover every sandbox sluss will ever create.

**Supersedes:** `SPEC.md` v0.5 §7's `*.sluss.localhost` scheme and its per-sandbox port table.

**Rejected:** *writing Traefik routes per sandbox.* It also would not have worked — that Traefik
runs the file provider only — and generating configuration for someone else's proxy is what made
the earlier design non-portable.

---

## D14 — Lifecycle stays in the shell script

**Chosen:** `slussd` creates, stops and destroys sandboxes by running `scripts/sluss` as a
subprocess with the chosen repository as its working directory and `SLUSS_APP_NAME` /
`SLUSS_AGENT` / `SLUSS_WORKTREE_ROOT` in its environment.

**Why:** the script already does worktree lifecycle correctly, including the unwind when
`sbx create` fails and the refusal to destroy dirty or unmerged work. A Go reimplementation would
be a second implementation to keep in step, and the GUI would quietly diverge from the terminal.
Running the same code path is what makes "created from the browser" and "created in a terminal"
identical by construction rather than by testing.

**Cost accepted:** shelling out to bash on every mutation, and parsing exit codes rather than
errors. A non-zero exit is treated as the script's answer and surfaced verbatim, not as a sluss
failure.

**Consequence:** a lifecycle run is detached from the HTTP request that started it. Cancelling the
request would kill the script mid-operation — between `git worktree add` and `sbx create`, where
its own unwind never runs — leaving an orphan worktree and branch.

---

## D15 — Authentication deferred, and what stands in for it

**Chosen:** no authentication. Reachability is the boundary: `lan: false` binds loopback, and the
NAS deployment is LAN- and VPN-only.

**Why:** this inherits the homelab's existing and documented stance for its current opencode VM —
"you are on the LAN is the whole auth story, and it is only reachable over the VPN" — together with
the standing instruction never to port-forward it. Adding a token to sluss alone would not change
the property that anything on that LAN can already reach the agent it protects.

**But the premise has a hole:** a browser makes `127.0.0.1` reachable from any page on the web. So
mutating routes reject a cross-site `Sec-Fetch-Site` and require `application/json` on POST — the
one shape a cross-origin request can take without a preflight. That closes the drive-by path. It is
not authentication and must not be described as such.

**Revisit when:** sluss becomes reachable from outside the VPN, or untrusted devices join the LAN —
the same two conditions the homelab names.

**Supersedes:** `AGENTS.md`'s former "every mutating route is gated by the token from
`~/.config/sluss/token`" rule, and the roadmap's M3.

---

## D16 — Two binaries: `sluss` and `slussd`

**Chosen:** the shell script keeps the name `sluss`; the Go daemon is `slussd`.

**Why:** `scripts/install.sh` installs the script to `~/.local/bin/sluss`, and the daemon was
originally to be called `sluss` too. Two things of the same name on `PATH` make install order decide
which one a bare `sluss start` runs, and a daemon that resolved its script by a bare lookup could
invoke itself. `slussd` resolves the script through `$SLUSS_SCRIPT`, else the installed path, never
by searching `PATH`.

**Cost accepted:** one more name to know, and `cmd/slussd` no longer matches the `cmd/sluss` path
the earlier spec named.
