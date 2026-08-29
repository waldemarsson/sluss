# AGENTS.md

Instructions for coding agents working on sluss.

## Read first

- `.trailmix/trail/gui-control-plane/spec.md` and `plan.md` — **the current truth**: what is being
  built, in what order, and under which constraints. Where they disagree with anything in `docs/`,
  they win.
- `docs/DECISIONS.md` — settled choices and rejected alternatives. **Do not relitigate these.** If you think one is wrong, say so in prose and wait; don't silently implement the alternative. D3, D4, D5, D6 and D9 carry forward unchanged; the `*.sluss.localhost` port-per-sandbox routing scheme is superseded by the spec's single-port design.
- `docs/SPIKE.md` — assumptions verified before coding. If an answer is blank, that assumption is **unverified** — flag it rather than coding around it.
- `docs/SPEC.md`, `docs/ROADMAP.md` — **superseded** by the spec above; rewritten at the Document waypoint. History, not instructions.
- `docs/PROFILES.md` — a deferred feature. **Do not implement anything in it** unless explicitly asked.

## Two binaries, two names

`sluss` is the shell script (`scripts/sluss`, installed to `~/.local/bin/sluss`) and remains the
lifecycle tool. `slussd` (`cmd/slussd`) is the Go daemon serving the dashboard and the reverse
proxy. The script is **unchanged** by daemon work: `slussd` invokes it as a subprocess, resolved
explicitly through `$SLUSS_SCRIPT` or the installed path — never by a bare `PATH` lookup, which
could find `slussd` itself.

## Where things run

sbx needs hardware virtualisation and its own host daemon, so **it does not run in the devcontainer.**
The container is for writing, building and unit-testing Go. Anything that touches a real sandbox —
`docs/SPIKE.md`, `slussd doctor`, `slussd serve` against a live sbx — runs on the host Mac.
Don't write a test that shells out to a real `sbx`.

## Context you need

The author is an experienced .NET/C# developer **learning Go on this project**. That changes how you should work:

- **Explain idioms, don't just produce code.** When you use something Go-specific — pointer vs value receivers, `context.Context` threading, `errgroup`, interface placement, channel patterns — add a one-line comment or a note in your reply explaining why.
- **Prefer obvious over clever.** If there's a concise idiomatic form and a plainer one, and the plainer one is only slightly longer, use the plainer one.
- **Never leave code that "just works" unexplained.** The author must be able to review it. Code they nod along to without understanding is worse than no code.

## Go style for this project

**Write plain Go.** The most common failure mode for agents in Go is producing Java-shaped code.

- Concrete types by default. Add an interface only when there are two implementations or a test genuinely needs a fake. If you can't explain why an interface exists, delete it.
- Define interfaces where they're **consumed**, not next to the implementation.
- Flat package structure. Start with fewer packages than feels right; split when a file becomes annoying. No `pkg/`, no `service` layer, no repository pattern.
- Wrap errors with context: `fmt.Errorf("creating worktree for %s: %w", id, err)`. Every error should say what was being attempted.
- No `panic` outside `main` and genuine programmer errors.
- `context.Context` as the first parameter on anything that shells out or does I/O.
- Pointer receivers by default unless the type is small and immutable.
- Table-driven tests.
- Standard library first. Justify every new dependency in the PR description.

Approved dependencies: none currently in use — `slussd` is standard library only (`flag`,
`net/http` including its method-and-wildcard mux patterns, `encoding/json`, `go:embed`).
`spf13/cobra` and `charmbracelet/lipgloss` stay pre-approved if the CLI outgrows `flag`. Anything
else, ask. The dashboard is SvelteKit with adapter-static, embedded into the binary. Its tooling follows
babytabs and homehub rather than being chosen here; Prettier and prettier-plugin-svelte were added
on that basis, at the versions those repos pin.

## Architecture rules

**sluss is a thin layer over sbx.** Before implementing anything, check whether sbx already does it — isolation, secrets, network policy, agent install, kits, skills, MCP wiring are all sbx's job.

**Centralise all sbx invocation in `internal/sbx`.** Every sbx command takes an app-name as a required parameter — never an option with a default, never a call constructed outside that package. In the MVP the app-name comes from environment state; later it will come from a profile resolver. That change is small only if there is exactly one chokepoint. This is the most important structural rule in the project.

**Never generate a kit `spec.yaml`.** Kits are hand-authored, live in git, get shared. sluss references paths only.

**sluss parses no YAML.** Config is JSON. Kit `spec.yaml` is read and written as opaque text —
sbx is the validator, and no Go struct models the kit schema.

**Every multi-step operation needs an unwind path.** Worktree created but sandbox creation failed — clean up the worktree. Write the unwind at the same time as the happy path, not later. Worktree lifecycle and its unwind live in `scripts/sluss`; don't reimplement either in Go.

**sluss holds no persistent state.** Every sandbox fact is derived per poll from
`sbx --app-name X ls --json` plus git commands against the worktree path sbx reports. There are no
state files and nothing to reconcile — an `sbx rm` by hand becomes visible within one tick. Every
read path must tolerate a missing sandbox or a vanished worktree and report it rather than crash.

## Things to be careful with

**The reverse proxy.** `FlushInterval: -1` is load-bearing — it's the difference between a usable web UI and one that feels broken. Do not remove it, do not add buffering middleware, do not set `Content-Length` on streamed responses. If you touch `internal/proxy`, say so explicitly in your summary.

**Error messages from sbx.** These are the bulk of the work and the difference between a script and a tool. "not logged in", "daemon not running", "name already exists", "kit fetch failed", "network policy blocked the install hook" each need a readable message with a suggested fix. Don't dump raw stderr.

**Destroy is destructive.** `scripts/sluss destroy` refuses uncommitted or unmerged work without `--force`, and that refusal is the default path everywhere: `slussd` surfaces it unchanged and passes `--force` only when the request asks for it with `?force=true` — exactly that value, so a typo cannot arm it. The dashboard confirms **every** destroy and arms force per sandbox, disarming it again afterwards, so a box left ticked on one row can never force another. Do not make force the default, do not remember it across destroys, and do not remove the confirm (D20).

**Dashboard auth is deferred by decision.** Do **not** add a token gate to mutating routes. Reachability is the whole auth story: `lan: false` binds loopback, and the NAS deployment is LAN- and VPN-only, inheriting the homelab's documented and accepted stance for its existing opencode VM. Revisit only under the conditions homelab names — reachable from outside the VPN, or untrusted devices on the LAN.

## Working style

- Small changes. One concern per commit.
- Run `task check` (gofmt + `go vet` + `go test` + shellcheck + the script's black-box tests) before finishing.
- Dashboard work has its own gates, deliberately kept out of `task check` so a Go change does
  not pay for a browser launch: `task check:web` (Prettier + svelte-check) and `task test:web`
  (Vitest `unit` on node for `*.unit.test.ts`, `component` in a real headless Chromium for
  everything else — D22). CI runs both. The
  component project needs Chromium in `~/.cache/ms-playwright`; the devcontainer installs it
  in postCreate, so on a fresh container run `task test:web` only after the rebuild finishes.
- **The dashboard is Prettier-formatted — don't hand-format it.** `web/dashboard/.prettierrc.json`
  is byte-identical to the one in babytabs and homehub: tabs, single quotes, no trailing commas,
  100 columns. `task fmt:web` writes, `task check:web` checks, and CI fails on unformatted code.
  ESLint is still not a dependency, so `svelte-check` stays the only rule-level check.
- When a spec detail is ambiguous, ask rather than guessing. This spec was iterated a lot; the gaps that remain are usually genuine uncertainty, not oversight.
- If you discover sbx behaves differently to what the spec assumes, **stop and report it**. Several design decisions rest on sbx's actual behaviour; a wrong assumption should change the spec, not get worked around in code.
- The plan's tasks are the unit of work: one task, one green gate, one commit. Don't gold-plate a
  task to make the next one easier.
- Profiles are deferred by an explicit decision (D10). If a task seems to need them, it probably doesn't — ask.
- Two build gotchas worth knowing before you trip on them: `go:embed` cannot reach outside its own
  package directory, which is why the dashboard is embedded from `web/embed.go` rather than
  `internal/server`; and adapter-static empties `web/dashboard/build/` on every build, so the
  `.gitkeep` that keeps the embed compiling from a fresh clone lives in `web/dashboard/static/` and
  is copied back in by the build.
- `web/dashboard/src/app.css` holds the styles shared across routes, but a Svelte component that
  styles the same element itself wins: scoped rules compile to `textarea.svelte-xxxx`, which
  outranks a bare `textarea` selector at any breakpoint. Restate the shared rule in the component
  rather than assuming app.css reaches it, and check the built CSS under
  `web/dashboard/build/_app/immutable/assets/` when a shared style appears not to apply.
