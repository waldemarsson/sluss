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

## D14 — Lifecycle stays in the shell script *(superseded by D23)*

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

## D16 — Two binaries: `sluss` and `slussd` *(superseded by D24)*

**Chosen:** the shell script keeps the name `sluss`; the Go daemon is `slussd`.

**Why:** `scripts/install.sh` installs the script to `~/.local/bin/sluss`, and the daemon was
originally to be called `sluss` too. Two things of the same name on `PATH` make install order decide
which one a bare `sluss start` runs, and a daemon that resolved its script by a bare lookup could
invoke itself. `slussd` resolves the script through `$SLUSS_SCRIPT`, else the installed path, never
by searching `PATH`.

**Cost accepted:** one more name to know, and `cmd/slussd` no longer matches the `cmd/sluss` path
the earlier spec named.

---

## D17 — The dashboard runs SvelteKit 3 before it is released

**Chosen:** the dashboard pins `@sveltejs/kit@3.0.0-next.25` and `@sveltejs/adapter-static@4.0.0-next.4`
— exact pins, not ranges, because a caret over a prerelease silently widens to the stable release —
and with them Vite 8, `@sveltejs/vite-plugin-svelte` 7, TypeScript 6 and Node ≥ 22.17, which
SvelteKit 3 requires as hard peers.

**Why:** babytabs is already on the same prerelease line, and keeping the two frontends on one
major avoids learning the migration twice. The dashboard is also the cheapest possible place to
carry the risk: two components, one prerendered client-only route, no `load`, no form actions, no
server routes — almost none of SvelteKit 3's breaking changes have anything here to break.

**What it cost:** `svelte.config.js` is gone (SvelteKit 3 takes its config through the Vite plugin),
`$lib` became `#lib` backed by the `imports` map in `package.json`, and `jsconfig.json` (now
`tsconfig.json`, see D18) extends `$app/tsconfig` and supplies the `include`/`exclude` that
`svelte-kit sync` used to generate.

**Known noise:** the build prints `Reading config.kit inside adapters is deprecated` from
adapter-static, and the browser test run warns about `transformIndexHtml` from Vitest's own plugin.
Both are upstream prerelease drift, not dashboard code.

**Revisit when:** SvelteKit 3.0.0 goes stable — swap both pins for carets and drop this note.

---

## D18 — The dashboard is TypeScript, like the other two frontends

**Chosen:** the dashboard is TypeScript — `tsconfig.json`, `vite.config.ts`, `lang="ts"` on every
component — with the API payloads hand-mirrored from the Go structs in `web/dashboard/src/lib/api.ts`.

**Why:** it was never a decision. No ADR chose plain JS and neither the `gui-control-plane` spec nor
its plan mentions the language; it was the scaffold default, carried forward. Both sibling frontends
went the other way — babytabs is 63 `.ts` and 60 `.svelte` with zero `.js`, homehub 36 and 25 — and
`jsconfig.json` was visibly a JS-ified copy of babytabs' `tsconfig.json`, same comments and all,
diverging only in the filename, a missing `src/**/*.ts`, and a `checkJs: false` block. So this
restores a convention rather than introducing one, and the frontend is about to grow past the point
where an untyped `JSON.parse` result threading through several pages stays cheap.

**Why now rather than later:** TypeScript 6 is already a devDependency — D17 pulled it in as a hard
peer of SvelteKit 3 — and `lang="ts"` compiles and type-checks on this stack with no config change,
so the migration was file-by-file with `npm run check` green throughout. The cost only rises with
the file count.

**What it cost:** `noUncheckedIndexedAccess` (matching babytabs) does not narrow on a
`list.length > 0` guard, so one index access in `Kits.svelte` binds the element first. That is the
only behavioral edit in the migration; everything else is an annotation or a rename.

**Rejected:** `checkJs: true` plus JSDoc, which the old `jsconfig.json` comment prescribed. Same
checking, more verbose, and it would have made a third convention across three frontends.

**Not generated:** `api.ts` is written by hand. The API is Go in this same repo, so codegen would
cost more than nine small types are worth. When a mirrored struct changes, `npm run check` finds
every use — it catches the misuse, not the drift, so change both sides together.

**Still open:** eslint. Prettier arrived in D21, which also records why eslint did not.

**Amends:** D17's note that `jsconfig.json` supplies the `include`/`exclude` — that file is now
`tsconfig.json`; everything else in D17 stands.

---

## D19 — Dashboard routes prerender to directory indexes

**Chosen:** `trailingSlash: 'always'` in `web/dashboard/src/routes/+layout.ts`, so every route
prerenders to `<route>/index.html`. `slussd` gained no routing code.

**Why:** the dashboard grew from one page to `/`, `/config/secrets` and `/config/kits` (the fleet
is the home page; secrets and kits moved behind a top bar). `internal/server` serves the embedded
build with a bare `http.FileServerFS` and no SPA fallback, and that resolves a directory index but
not a sibling `.html` file — so SvelteKit's default output, `config/secrets.html`, 404s for anyone
who bookmarks the URL or reloads the page. With the trailing slash the build emits
`config/secrets/index.html`, which the file server serves directly and, for the slash-less form,
redirects to.

**Rejected:** *an SPA fallback handler in `internal/server`* — serve `index.html` for any unmatched
path that is not `/api/` or `/s/`. It works, but it is a handler plus its edge cases in place of one
config line, and it makes a genuine 404 indistinguishable from a route.

**This is load-bearing and quiet.** Nothing fails at build time if the option is removed: the
dashboard still builds, `npm run check` still passes, and client-side navigation still works,
because SvelteKit routes in the browser. Only a cold request for a nested URL breaks, which is
exactly the path a phone bookmark takes. `TestBuiltDashboardNestsConfigRoutes` in
`web/embed_test.go` asserts the built tree still has the nested indexes, and
`TestStaticAssets` covers both URL forms through the real server; those two tests are the guard.

**Consequence for new routes:** any route added under `src/routes/` inherits this and needs
nothing. A route that must be reachable cold and is *not* linked from another page still needs to
be prerendered — the crawler is what discovers them.

---

## D20 — Force destroy is available from the dashboard, behind a confirm

**Chosen:** `DELETE /api/sandboxes/{scope}/{name}?force=true` passes `--force` to
`scripts/sluss destroy`. The dashboard shows a per-sandbox `force` checkbox, confirms every
destroy whether or not it is ticked, names what force discards in the prompt, and clears the
checkbox once the destroy returns.

**Why:** this reverses an earlier instruction in `AGENTS.md` — that slussd never passes `--force`
and that "a confirmation prompt is the wrong affordance". What changed is the deployment, not the
judgement about how costly a lost afternoon is. The refusal has no terminal escape hatch when the
dashboard is the only thing in reach: a dirty sandbox opened from a phone over the VPN could be
seen, stopped and connected to, but not removed. The earlier reasoning assumed a terminal is
always available, and on the NAS deployment it is not.

The unforced path is unchanged and is still the default. Force is opt-in per destroy, spelled
exactly `true`, and never sticky.

**Rejected:**
- *Two-stage "try, then confirm the refusal"* — attempt unforced, then offer "destroy anyway" with
  the script's own message as the warning. It cannot disagree with the script about what would be
  lost, which is genuinely better, but it costs a round trip and a second dialog on a phone. The
  checkbox arms the same capability in one gesture and the confirm still names the cost.
- *A `force` flag remembered per session* — one tick, then every later destroy is forced. This is
  the failure mode the per-sandbox reset exists to prevent.
- *Leaving it terminal-only* — the status quo, and the thing that made a sandbox undeletable from
  the deployment sluss was built for.

**Consequence:** the confirm and the per-sandbox reset are correctness requirements with tests
against them, not UI polish. `internal/script`'s `Destroy` takes `force` as an explicit parameter
so a caller cannot discard work by omission, and its false case is covered by a test.


---

## D21 — Prettier, copied from the sibling frontends rather than configured here

**Chosen:** `web/dashboard/.prettierrc.json` is byte-identical to the file in
`waldemarsson/babytabs` and `waldemarsson/homehub` — tabs, single quotes,
`trailingComma: "none"`, 100 columns, `prettier-plugin-svelte` — at the same dependency
versions. `.prettierignore` follows babytabs' shape. `task fmt:web` writes, `task check:web`
checks, and CI fails on unformatted code. This closes the "still open" note in D18.

**Why:** the three frontends share a devcontainer lineage and the same
SvelteKit-SPA-in-a-backend shape, so a formatter that disagreed between them would fight
anyone moving code across, and every copied snippet would arrive as diff noise. The previous
instruction in `AGENTS.md` — "the dashboard has no formatter… match the surrounding style by
hand" — was a reasonable answer while no config existed anywhere, but the siblings did have
one; only this repo hadn't asked them.

Adopting the house file rather than a locally-derived one also happened to be cheaper: a config
inferred from sluss's own source set `trailingComma: "all"` (Prettier's default) and rewrote 19
files, where the house `"none"` rewrites 12, because sluss was already written without them.

**Rejected:**
- *A config derived from sluss's existing source.* Defensible in isolation and it is what was
  written first, but it encodes one repo's accumulated history as if it were a standard, and it
  diverged from the siblings in exactly the setting they had bothered to set.
- *ESLint alongside it.* babytabs runs `eslint` with `eslint-plugin-svelte`,
  `typescript-eslint` and `eslint-config-prettier`, so the convention argument points at adopting
  it too. Deliberately deferred on 2026-08-29: `svelte-check` already gates the type-level
  problems, and the rule-level ones can wait for a frontend larger than ~1100 lines. This is a
  known, chosen divergence from the siblings, not an oversight — revisit it rather than
  rediscovering it.

**Consequence:** never hand-format a dashboard file. A formatting-only reformat is its own commit
(the one that introduced this decision), so a later diff stays readable.

---

## D22 — Test projects split on a filename suffix, not a directory

**Chosen:** Vitest's `unit` project collects `src/**/*.unit.test.{js,ts}` and runs on node;
`component` collects everything else under `src/lib` and `src/routes` and runs in real headless
Chromium. `test:unit` no longer carries `--passWithNoTests`.

**Why:** the split used to be by path — `unit` excluded `src/lib/**` and `src/routes/**`, which
left it with nothing to run. That held while every module in `src/lib` was a component or a rune
module that needs a DOM. `lib/api.ts` broke it: it is framework-free, it owns the rule that turns
a failed response into one displayed line, and that rule is worth testing directly rather than
through three components. A directory can no longer say which kind of test a file is, so the
filename does.

**Rejected:**
- *Leaving `api.ts` to the component project.* It would run in Chromium for no reason, and
  `test:unit` would keep `--passWithNoTests`, which silently tolerates a broken glob.
- *A separate directory for framework-free modules.* It splits `api.ts` from the types it
  belongs with to satisfy a test runner.

**Consequence:** `--passWithNoTests` is gone, so an empty `unit` project now fails loudly. The
component project's `exclude` spreads Vitest's `defaultExclude` rather than replacing it, or
assigning it would drop the built-in `**/node_modules/**` guard.

---

## D23 — Lifecycle moves into Go, and the shell script is retired

**Supersedes D14.**

**Chosen:** worktree and sandbox lifecycle is `internal/lifecycle`, written in Go. `scripts/sluss`
is deleted. The dashboard and the command line call the same functions in the same process.

**Why:** D14 kept lifecycle in the script so the GUI and the terminal could not diverge, and paid
for it by shelling out to bash on every mutation. That trade stopped making sense once the binary
became the thing worth installing: the script needed its own install path, its own update
mechanism and its own name, and `slussd` had to resolve it by an explicit path to avoid finding
itself. One implementation in one process gets D14's actual goal — the browser and the terminal
running identical code — with less machinery, and removes bash and a second install artefact from
the runtime.

**What it cost:** roughly 600 lines of careful bash were rewritten. The refusal wording, the exit
codes, the `--agent` and `--` parsing rules, the 4096 auto-publish and the create-failure unwind
were ported unchanged rather than improved, and `scripts/test-sluss`'s black-box cases were
carried over as Go tests before the script was deleted — including the ones guarding a `gh` /
`gh-remote` prefix match and a backslash in a sandbox name. Decoding `sbx ls --json` properly
rather than with awk removes that second class of bug structurally.

**What improved:** a refusal is now a typed error (`lifecycle.Error` with a `Kind`) rather than an
exit code parsed out of a subprocess, so the exit status is decided where the reason is known. The
`Result{exitCode, stdout, stderr}` shape is kept exactly, because the dashboard consumes it.

**One smaller divergence, recorded rather than hidden:** the script tested the worktree path with
`[[ -e ]]`, which follows symlinks, so a dangling symlink there was not "occupied" and
`git worktree add` was attempted against it. The Go port uses `Lstat`, so a dangling symlink is
occupied and `start` refuses with "worktree path already exists" — a clearer message than the git
failure it replaces.

**Consequence:** an unscoped sbx call is no longer possible. The script fell back to sbx's default
scope when `SLUSS_APP_NAME` was unset; `internal/sbx` requires an app-name, so the command line
resolves one from the environment, then the configuration file, and reports a readable error if
there is none. That is the chokepoint rule (D3) applied consistently, and it is a deliberate
behaviour change.

---

## D24 — One binary, named `sluss`

**Supersedes D16.**

**Chosen:** there is one artefact, `cmd/sluss`, built as `sluss`. `slussd` is gone.

**Why:** D16 existed because two things of the same name on `PATH` would make install order decide
which one a bare `sluss start` ran, and a daemon resolving its script by a bare lookup could
invoke itself. Both hazards are properties of there being two things. With one binary there is
nothing to collide with, and the name people already type is the one that should be installed.

**What came with it:** releases are built by CI from a `v*` tag for darwin/arm64, darwin/amd64,
linux/amd64 and linux/arm64, as `sluss_<os>_<arch>.tar.gz` plus `checksums.txt`. Asset names carry
no version so `releases/latest/download/<name>` resolves without a GitHub API call — which keeps
`scripts/install.sh` free of the unauthenticated rate limit. `sluss update` does need the latest
tag, so it does call the API, and degrades to "reinstall with the install script" when limited.

**Rejected:**
- *Embedding the script in the binary with `go:embed`.* One name, but bash stays a runtime
  dependency and the rewrite is only postponed.
- *Keeping `slussd` and adding an installer for it.* Leaves two things to install and two to
  update, which is the problem.

**Consequence:** the release workflow now builds the dashboard before the Go binary. It did not
before, which would have shipped a binary embedding `web/dashboard/build/.gitkeep` instead of the
UI — a release that looks like it worked. A workflow step asserts the built binary carries a real
dashboard.
