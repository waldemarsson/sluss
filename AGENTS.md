# AGENTS.md

Instructions for coding agents working on sluss.

## Read first

- `docs/SPEC.md` — what we're building and why
- `docs/ROADMAP.md` — milestone order, gates, and what's still open. Tells you what work is in scope *now*.
- `docs/DECISIONS.md` — settled choices and rejected alternatives. **Do not relitigate these.** If you think one is wrong, say so in prose and wait; don't silently implement the alternative.
- `docs/SPIKE.md` — assumptions verified before coding. If an answer is blank, that assumption is **unverified** — flag it rather than coding around it.
- `docs/PROFILES.md` — a deferred feature. **Do not implement anything in it** unless explicitly asked.

## Where things run

sbx needs hardware virtualisation and its own host daemon, so **it does not run in the devcontainer.**
The container is for writing, building and unit-testing Go. Anything that touches a real sandbox —
`docs/SPIKE.md`, `sluss doctor`, `sluss env` against a live sbx — runs on the host Mac.
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

Approved dependencies: `spf13/cobra`, `charmbracelet/lipgloss`. Anything else, ask.

## Architecture rules

**sluss is a thin layer over sbx.** Before implementing anything, check whether sbx already does it — isolation, secrets, network policy, agent install, kits, skills, MCP wiring are all sbx's job.

**Centralise all sbx invocation in `internal/sbx`.** Every sbx command takes an app-name as a required parameter — never an option with a default, never a call constructed outside that package. In the MVP the app-name comes from environment state; later it will come from a profile resolver. That change is small only if there is exactly one chokepoint. This is the most important structural rule in the project.

**Never generate a kit `spec.yaml`.** Kits are hand-authored, live in git, get shared. sluss references paths only.

**sluss parses no YAML.** State and config are JSON. The `.sbxenv.yaml` overlay is emitted only.

**Every multi-step operation needs an unwind path.** Worktree created but sandbox creation failed — clean up the worktree. Write the unwind at the same time as the happy path, not later.

**Reconciliation is required.** State files drift from reality the moment someone runs `sbx rm` by hand. Every read path must tolerate a missing sandbox and report it rather than crash.

## Things to be careful with

**The reverse proxy.** `FlushInterval: -1` is load-bearing — it's the difference between a usable web UI and one that feels broken. Do not remove it, do not add buffering middleware, do not set `Content-Length` on streamed responses. If you touch `internal/proxy`, say so explicitly in your summary.

**Error messages from sbx.** These are the bulk of the work and the difference between a script and a tool. "not logged in", "daemon not running", "name already exists", "kit fetch failed", "network policy blocked the install hook" each need a readable message with a suggested fix. Don't dump raw stderr.

**Destroy is destructive.** `env destroy` must refuse when the worktree has unmerged commits, requiring `--force`. A confirmation prompt is the wrong affordance when the cost is losing an afternoon of agent work.

**Dashboard auth.** Every mutating route is gated by the token from `~/.config/sluss/token`. The dashboard has destroy buttons and is LAN-reachable. Do not ship a route without the gate.

## Working style

- Small changes. One concern per commit.
- Run `task check` (gofmt + `go vet` + `go test`) before finishing.
- When a spec detail is ambiguous, ask rather than guessing. This spec was iterated a lot; the gaps that remain are usually genuine uncertainty, not oversight.
- If you discover sbx behaves differently to what `SPEC.md` assumes, **stop and report it**. Several design decisions rest on sbx's actual behaviour; a wrong assumption should change the spec, not get worked around in code.
- Milestones M1a–M1c are deliberately crude — no reconciliation, no tests, no dashboard. Don't gold-plate it. Its only purpose is finding out whether the daily loop is worth the remaining work.
- Profiles are deferred by an explicit decision (D10). If a task seems to need them, it probably doesn't — ask.
