---
slug: single-binary-sluss
title: One binary named sluss
created: 2026-08-29
updated: 2026-08-29
waypoint: discuss
status: approved
document: done
---

# One binary named sluss — spec

**Problem / why:** sluss ships as two halves with two names. `sluss` is a 669-line bash script
that owns worktree and sandbox lifecycle; `slussd` is a Go binary that serves the dashboard and
shells out to that script for every mutation. Only the script has an install path — the daemon has
to be built and copied by hand — and the split means two things to install, two things to update,
and a `PATH` collision that D16 exists to work around. The Go binary has grown past the script:
it is the thing worth installing, so it should be the only thing installed, and it should be
called `sluss`.

**In scope:**

- **Lifecycle ported to Go.** A new `internal/lifecycle` package reproduces every behaviour of
  `scripts/sluss`: `start` (branch, worktree, `sbx create`, and the unwind when creation fails),
  `list`, `attach` (the per-agent remote-interface handling for opencode / claude / copilot, with
  the agent read back from sbx), `exec`, `stop`, `destroy` (the dirty and unmerged refusals plus
  `--force`), and `path`.
- **One binary, one name.** `cmd/slussd` → `cmd/sluss`; the built artifact is `sluss`. Full CLI
  parity with the retired script plus the daemon's own commands: `start`, `list`, `attach`,
  `exec`, `stop`, `destroy`, `path`, `serve`, `doctor`, `update`, `version`, `help`.
- **The script is deleted.** `scripts/sluss` and `scripts/test-sluss` are removed from the repo,
  and every reference to a "script" in code, comments, docs and config goes with them —
  `internal/script`, `SLUSS_SCRIPT`, `script.ResolveScript`, the doctor's script check, the
  shellcheck gate over them.
- **Install by curl, from GitHub releases.** `scripts/install.sh` is rewritten to detect OS and
  architecture, download the matching release archive, verify its SHA-256 against the release's
  `checksums.txt`, and install `sluss` to `~/.local/bin`. Re-running it upgrades in place.
- **Uninstall by curl.** A new `scripts/uninstall.sh` removes the installed binary and reports
  what it left behind (config, worktrees, sandboxes — it deletes none of them).
- **`sluss update`.** Self-update in Go: resolve the latest release, compare it with the running
  version, download, verify the checksum, and atomically replace the running executable.
- **Releases built by CI.** Pushing a `v*` tag builds and publishes archives for darwin/arm64,
  darwin/amd64, linux/amd64 and linux/arm64, plus `checksums.txt`. The workflow's current
  `if: false` guard is removed.
- **README rewritten** — installation, usage, configuration, and how the OpenCode Web UIs are
  addressed, in enough detail for an agent to set up a deployment from it alone.
- **Decisions recorded.** D14 (lifecycle stays in the shell script) and D16 (two binaries) are
  superseded by new entries, not edited in place.

**Out of scope:**

- **Windows.** Decided 2026-08-29: darwin and linux only. No `.ps1`, no windows targets. The Go
  code should not go out of its way to be unportable, but nothing is written or tested for it.
- **Behaviour changes to lifecycle.** This is a port. Flag parsing, refusal wording, exit codes,
  the `--` forwarding rule, the `--agent` consumption rule, the opencode port-4096 auto-publish
  and the PID-file idempotence of `attach` all carry over as they are. Anything that looks worth
  improving gets noted, not changed.
- **The dashboard.** No UI change, no route change, no API shape change. The mutation endpoints
  keep returning `{exitCode, stdout, stderr}` so the frontend is untouched.
- **Authentication** (D15 stands), **profiles** (D10 stands), **kit schema parsing** (never).
- Package managers, a Homebrew tap, code signing, notarisation, or an `apt`/`nix` path.
- Migration tooling for an existing `~/.local/bin/sluss` script: the installer simply overwrites
  it, which is the intended outcome.

**Chosen approach:** port lifecycle into Go and land the whole change in one trail, rather than
renaming first and porting later. Renaming first would need the script embedded in the binary as
an interim, which trades one hidden implementation for another and delays the only risky part —
rewriting the destroy refusal and the worktree unwind — without reducing it. Doing it once, with
the script still in git history to diff against, is the smaller total cost. `internal/lifecycle`
keeps the shape `internal/script` already exposes to the server (`Runner` with
`Start`/`Stop`/`Destroy`, returning a `Result` carrying an exit code), so the port is a swap
behind an existing seam rather than a redesign of the server.

Alternatives considered and rejected: **embedding the bash script** with `go:embed` (keeps one
implementation but keeps bash as a runtime dependency and only defers the work); **a
dashboard-shaped CLI** that drops `list`/`attach`/`exec`/`path` (loses the terminal workflow in
daily use).

**Constraints:**

- Standard library only. `update` needs HTTP, JSON, gzip, tar and SHA-256 — all stdlib. No new
  module dependency without asking (AGENTS.md).
- **`internal/sbx` stays the single chokepoint for every sbx invocation.** Lifecycle must call
  sbx through it, with the app-name as a required parameter, never by constructing its own
  `exec.Command("sbx", …)`. This is the project's most important structural rule.
- **Destroy stays destructive-by-refusal (D20).** Uncommitted or unmerged work is refused unless
  `--force` is passed explicitly; the dashboard's confirm and per-sandbox force arming are
  unchanged; force never becomes a default and is never remembered.
- **No persistent state (D12).** Lifecycle derives everything from sbx and git per call.
- **`FlushInterval: -1` in `internal/proxy` is load-bearing.** Untouched by this trail.
- No test may shell out to a real `sbx` or a real remote. Tests use `internal/sbxstub` and, for
  `update` and the installer, a local HTTP server / a stubbed base URL.
- Plain Go, explained: the author is learning Go here. Concrete types, interfaces only where a
  test genuinely needs a fake, errors wrapped with what was attempted, `context.Context` first on
  anything that shells out.
- `task check` must pass, with the shell gates now covering `scripts/install.sh` and
  `scripts/uninstall.sh` only.
- The README must contain nothing specific to the author's homelab — it is a public repo.

**Acceptance criteria:**

- [ ] AC1: `sluss start NAME` creates branch `agent/NAME` and a worktree under
      `$SLUSS_WORKTREE_ROOT/<repo>/NAME`, calls `sbx create` with the resolved agent, and on a
      failed create removes both the worktree and the branch — verified against `sbxstub`.
- [ ] AC2: `sluss start` on an existing worktree starts the sandbox instead of recreating it;
      `--agent` is consumed and never forwarded to sbx; every other argument, and everything
      after `--`, reaches `sbx create` verbatim; opencode gets `--publish 4096` only when the
      caller did not already publish 4096.
- [ ] AC3: `sluss destroy NAME` refuses with a non-zero exit and a readable message when the
      worktree is dirty, and when the branch has commits not merged into HEAD; `--force` removes
      the sandbox, worktree and branch anyway. Both refusal messages match the script's wording.
- [ ] AC4: `sluss attach NAME` reads the agent from `sbx ls --json`, and starts OpenCode Web on
      4096 / Claude Remote Control / Copilot `--remote` accordingly; repeated calls for opencode
      do not start a second server.
- [ ] AC5: `sluss list`, `sluss exec`, `sluss stop` and `sluss path` behave as the script did,
      including running from a worktree as well as the primary checkout, and `attach` running
      from any directory.
- [ ] AC6: `sluss serve` and `sluss doctor` work with no script installed and no `SLUSS_SCRIPT`
      set; `doctor` no longer reports on a script, and reports the lifecycle prerequisites it
      actually has (`sbx`, `git`, scopes, repos, kits, bind address).
- [ ] AC7: the dashboard's start / stop / destroy / create routes produce the same JSON as
      before, with refusals surfaced as a non-zero `exitCode` and the message in `stderr`; the
      existing server tests pass with only the runner type swapped.
- [ ] AC8: `curl -fsSL …/scripts/install.sh | sh` installs a working `sluss` on darwin/arm64,
      darwin/amd64, linux/amd64 and linux/arm64, verifies the SHA-256 before replacing anything,
      refuses on a checksum mismatch, and re-running it upgrades in place.
- [ ] AC9: `curl -fsSL …/scripts/uninstall.sh | sh` removes the installed binary, is a no-op with
      a clear message when nothing is installed, and never deletes config, worktrees or sandboxes.
- [ ] AC10: `sluss update` reports "already up to date" when the running version is the latest
      release, otherwise replaces the running binary atomically after verifying the checksum, and
      refuses when the running binary is a symlink or its directory is not writable.
- [ ] AC11: pushing tag `vX.Y.Z` publishes a release with four archives and `checksums.txt`, each
      binary reporting `X.Y.Z` from `sluss version`.
- [ ] AC12: `task check` passes: gofmt, `go vet`, `go test ./...`, and `bash -n` + shellcheck over
      the two remaining shell scripts. No reference to `slussd` or to a lifecycle script survives
      anywhere in the repo outside `docs/DECISIONS.md` history and `.trailmix/`.
- [ ] AC13: the README documents install, update, uninstall, every command, the full config file,
      and both sandbox addressing modes, with no homelab-specific content.

**Edge cases:**

- **A repo is not a git repository** — `start`, `destroy`, `path` and `list`'s worktree section
  must fail with the script's "run … inside a Git repository" message, not a git error.
- **Worktree path exists but is not a directory**, and **branch exists without its worktree** —
  both are distinct refusals in the script and must stay distinct.
- **`sbx ls` fails while resolving an agent** — an error, never a silent fallback. Only a
  successful listing that names no agent falls back to `$SLUSS_AGENT`, with a warning.
- **`attach` and `exec` are interactive** — stdin, stdout and stderr must be inherited, and the
  child's exit code must become sluss's, so `sluss exec NAME false` exits 1.
- **Signals during `exec`/`attach`** — Ctrl-C must reach the child, not just kill sluss.
- **`update` replacing a running binary** — write to a temp file in the same directory, then
  rename over the target, so a partially downloaded binary can never be left in place.
- **The install script running under `sh`, not `bash`** — it is piped to a shell by strangers'
  shells; keep it POSIX and do not rely on `[[`, arrays or `mktemp -d` GNU flags.
- **`~/.local/bin` not on `PATH`** — both scripts must say so, in a shell-agnostic way (the
  current script's advice is fish-only).
- **GitHub API rate limits** — `update` should degrade to a clear message, not a stack trace.
- **A vanished worktree or a sandbox removed by hand** — every read path already has to tolerate
  this (D12) and still must.

**Affected areas (current code):**

- `scripts/sluss`, `scripts/test-sluss` — deleted; their behaviour is the port's specification
  and their black-box cases become Go tests.
- `scripts/install.sh` — rewritten against GitHub releases. `scripts/uninstall.sh` — new.
- `scripts/sluss.fish` — kept; `sluss-cd` still calls `sluss path`.
- `internal/script` — deleted, replaced by `internal/lifecycle`.
- `cmd/slussd` → `cmd/sluss` — grows the lifecycle subcommands and `update`.
- `internal/server` — swaps `*script.Runner` for the lifecycle runner; no route or JSON change.
- `internal/doctor` — drops the script check, gains what replaces it.
- `internal/sbx` — gains the calls lifecycle needs (`create`, `rm`, `exec`, `run`, `stop`,
  `ports`, `settings set`), all scoped by app-name.
- `internal/gitfacts` — has a `git` helper; lifecycle needs more git calls and the two should
  share one runner rather than grow a second.
- `internal/sbxstub` — extended to cover the new sbx calls.
- `.github/workflows/release.yml` — un-paused, tag-triggered, four targets.
- `.github/workflows/ci.yml`, `Taskfile.yml` — binary rename, shell gates narrowed, script test
  target removed.
- `README.md` — rewritten. `AGENTS.md` — the "two binaries" and "lifecycle lives in the script"
  rules replaced.
- `docs/DECISIONS.md` — D23 supersedes D14, D24 supersedes D16.
- `docs/SPEC.md`, `docs/ROADMAP.md` — already marked superseded; updated at the Document waypoint.

**Open questions:**
- None.
