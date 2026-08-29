---
slug: single-binary-sluss
waypoint: review
status: approved
updated: 2026-08-29
findings: L1 L2:fixed L3:fixed L4
---

# Review: One binary named sluss

**Note on this review:** it was written by the same agent that implemented the change, not by a
separate reviewer — the session was instructed not to spawn subagents. Treat the findings as a
self-audit; the evidence below is real, the independence is not.

**Verdict: ship, with L1 and L4 accepted as known.** All eleven tasks are complete, every gate is
green, and the two behaviour changes that a user would notice are recorded as decisions rather
than left implicit.

## Evidence

| Gate | Command | Result |
|---|---|---|
| Go | `task check` (gofmt, `go vet`, `go test ./...`, `sh -n`, `bash -n`, shellcheck, `scripts/test-install`) | green |
| Dashboard | `task check:web` | Prettier clean, svelte-check 265 files 0 errors |
| Dashboard | `task test:web` | 8 files, 35 tests passed |
| Install scripts | `./scripts/test-install` | all checks passed |
| Binary | `task build` → `dist/sluss` | builds, embeds the real dashboard (`_app/immutable` present) |

Beyond the suites, the built binary was driven end to end against a fake `sbx` on `PATH` and a
real git repository:

- `start auth --cpus 4` → `sbx create --name auth --publish 4096 --cpus 4 opencode <worktree> <gitdir>`, branch and worktree created.
- `start auth` again → one call, `sbx exec auth true`; no second create.
- `list`, `path` → correct; `path` invoked sbx not at all.
- `destroy dirty` → `error: agent/dirty has uncommitted changes; commit them or use --force`, exit 1, **sbx call log empty**.
- `destroy ahead` → `error: agent/ahead has commits not merged into main; …`, exit 1, sbx untouched.
- `destroy dirty --force` → removed, and the *other* task's worktree and branch survived.
- `scripts/install.sh` against a `file://` release installed the real binary, which then reported its version; `scripts/uninstall.sh` removed it and listed what it kept.

## Findings

**L1 — `sluss update` always looks stale on a locally built binary.** `task build` stamps
`git describe` (`v0.1.0-10-g275c8e0-dirty`), which never equals a release tag, so `update` on a
developer build downloads the latest release over it. Correct behaviour for an installed binary
and harmless for a built one, but worth knowing before someone runs it in a checkout. Named in
the plan's risks; not fixed.

**L2 — `doctor` created the worktree root as a side effect.** *Fixed.* `checkWorktreeRoot` called
`os.MkdirAll` before probing. A diagnostic that changes the filesystem is a surprise; it now walks
up to the nearest existing ancestor, probes that, and reports "does not exist yet; … will be
created". Covered by `TestWorktreeRootCheckCreatesNothing`.

**L3 — `ignoreInterrupt` took and returned a `context.Context` it never touched, and its comment
claimed it restored the default handler.** *Fixed.* It is now a plain `ignoreInterrupt()` whose
comment says what actually happens: the change is process-wide and never undone, which is fine
because sluss reports the child's exit code and exits.

**L4 — `Runner.Stop` accepts a `repo` it does not use.** Kept deliberately, so every lifecycle
method has the same shape and callers need not know which ones care about a repository. Now
documented at the method rather than left to be discovered.

## Behaviour changes a user will notice

1. **A scope is now required.** The retired script fell back to sbx's default scope when
   `SLUSS_APP_NAME` was unset; `internal/sbx` requires an app-name, so the command line resolves
   one from the environment, then `appNames[0]`, and otherwise reports an error naming both. This
   is the chokepoint rule (D3) applied consistently — recorded in D23 and in the plan's
   amendments.
2. **`~/.local/bin/sluss` is replaced.** An existing installation is the shell script; the
   installer overwrites it with the binary. Intended.

## Checked and found correct

- **Destroy (D20).** Both refusals precede every sbx call, `--force` maps to `worktree remove --force` + `branch -D` and nothing else, `?force=true` remains exact, and the dashboard's confirm and per-sandbox arming are untouched.
- **The create unwind.** A failed `sbx create` removes the worktree *and* the branch, and reports sbx's own reason rather than the cleanup's.
- **The ported parsing rules.** `--agent` / `--agent=` consumption, `--` forwarding, and the 4096-publish detection are covered case by case, including `14096:4096` (publishes 4096, so no auto-publish) versus a bare `14096` (does not).
- **Agent read-back.** Exact name matching, so `gh` never resolves `gh-remote`; a failed `sbx ls` is an error that blames sbx and runs no agent. Decoding JSON removes the backslash-escaping class of bug the script had to hand-guard.
- **`internal/proxy` is untouched.** `FlushInterval: -1` is unchanged; no file in that package was modified.
- **The HTTP contract.** No route, status code or JSON field changed; the dashboard was not modified beyond comments naming a binary that no longer exists.
- **Release workflow.** Now builds the dashboard before the Go binary — it did not before, which would have shipped a binary embedding `.gitkeep` — and asserts the version and a real embedded dashboard before publishing.

## Not verified here

- **AC11 (a real release).** The workflow is reviewed by reading; it can only be proven by pushing a `v*` tag. The archive layout it produces is exercised locally by `scripts/test-install` and by `internal/release`'s tests against `httptest`.
- **Anything touching a real `sbx`.** By design (AGENTS.md): sbx does not run in the devcontainer.
