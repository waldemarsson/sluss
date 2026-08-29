---
slug: single-binary-sluss
waypoint: plan
status: approved
updated: 2026-08-29
tasks: T1:done T2:done T3:done T4:done T5:done T6:done T7:done T8:done T9:done T10:done T11:done
---

# Plan: One binary named sluss

**Goal:** replace the `sluss` shell script and the `slussd` binary with a single Go binary called
`sluss`, installable and updatable by curl from GitHub releases.

**Architecture:** a new `internal/lifecycle` package owns worktree and sandbox lifecycle, calling
sbx only through `internal/sbx` (extended with the create/rm/run/exec/stop/ports/settings commands
the script used) and git through one shared runner. It keeps the exact `Runner` / `Result` shape
`internal/script` exposes today, so `internal/server` swaps a type and nothing else — and the same
`Result` drives the CLI's output and exit code, so there is one contract, not two. `cmd/sluss`
grows the script's whole command surface plus `serve`, `doctor` and `update`; `update` and the
rewritten `scripts/install.sh` both resolve a GitHub release, verify SHA-256, and replace the
binary atomically.

## Global constraints

Copied from `spec.md`; every task inherits these.

- Standard library only. No new module dependency without asking.
- **`internal/sbx` is the single chokepoint for every sbx invocation.** Lifecycle calls it with
  the app-name as a required parameter; it never builds its own `exec.Command("sbx", …)`.
- **Destroy stays destructive-by-refusal (D20).** Dirty or unmerged work is refused unless
  `--force` is passed explicitly. Force is never a default and is never remembered.
- **No persistent state (D12).** Every fact comes from sbx or git, per call.
- **This is a port, not a redesign.** Refusal wording, exit codes, flag semantics and message text
  carry over from `scripts/sluss` unchanged. `git log` the deleted file to diff against.
- `internal/proxy`'s `FlushInterval: -1` is untouched by this trail.
- No test shells out to a real `sbx` or a real network. Tests use `internal/sbxstub`, real git
  repos in `t.TempDir()`, and `httptest` / `file://` for release downloads.
- Plain Go, explained: concrete types; an interface only where a test needs a fake; errors wrapped
  with what was attempted; `context.Context` first on anything that shells out.
- darwin and linux only. No Windows targets, no `.ps1`.
- The README carries nothing specific to the author's homelab.

## File structure

- `internal/sbx/sbx.go` — Modify — add the lifecycle commands (`Create`, `Remove`, `Stop`,
  `Exec`, `Run`, `Ports`, `SettingSet`) and an interactive execution path that inherits stdio.
- `internal/lifecycle/lifecycle.go` — Create — `Runner`, `Result`, `StartOpts`, and the
  server-facing `Start` / `Stop` / `Destroy`.
- `internal/lifecycle/args.go` — Create — pure argument handling: `--agent` consumption, `--`
  forwarding, 4096-publish detection, name validation.
- `internal/lifecycle/repo.go` — Create — repo root discovery, worktree path, branch naming, and
  the git calls (`worktree add/remove`, `branch -d/-D`, `show-ref`, `status --porcelain`,
  `merge-base --is-ancestor`, `worktree list`, `rev-parse`).
- `internal/lifecycle/agent.go` — Create — agent read-back from sbx, and `Attach` / `Exec`, the
  two interactive commands.
- `internal/lifecycle/*_test.go` — Create — the ported black-box cases from `scripts/test-sluss`.
- `internal/release/release.go` — Create — resolve the latest GitHub release, download and verify
  an asset, replace the running executable atomically.
- `internal/release/release_test.go` — Create.
- `cmd/sluss/main.go` — Create (from `cmd/slussd/main.go`) — command dispatch for the full
  surface.
- `cmd/sluss/usage.go` — Create — the per-command help text ported from the script.
- `cmd/sluss/main_test.go` — Modify — moved with the package, extended for the new commands.
- `cmd/slussd/` — Delete.
- `internal/script/` — Delete.
- `internal/server/server.go` — Modify — `*lifecycle.Runner`, `lifecycle.StartOpts`,
  `lifecycle.Result`. No route or JSON change.
- `internal/doctor/doctor.go` — Modify — drop `checkScript`; add `git` and worktree-root checks.
- `internal/sbxstub/stub.go` — Modify — cover the new sbx calls.
- `scripts/sluss`, `scripts/test-sluss` — Delete.
- `scripts/install.sh` — Rewrite — POSIX `sh`, GitHub releases, checksum-verified.
- `scripts/uninstall.sh` — Create.
- `scripts/test-install` — Create — black-box tests for both, against a `file://` fake remote.
- `scripts/sluss.fish` — Keep unchanged.
- `.github/workflows/release.yml` — Modify — tag-triggered, four targets, dashboard built first.
- `.github/workflows/ci.yml`, `Taskfile.yml` — Modify — rename, narrowed shell gates.
- `README.md` — Rewrite. `AGENTS.md` — Modify. `docs/DECISIONS.md` — Modify (append D23, D24).

## Public contracts (keep stable across tasks)

- `internal/sbx` — `(*Client).Create(ctx, appName string, opts CreateOpts) error` — `CreateOpts`
  carries `Name`, `Agent`, `Workspaces []string`, `Extra []string`; builds
  `sbx --app-name X create --name N [extra…] AGENT WORKSPACE…` in the script's argument order.
- `internal/sbx` — `(*Client).Remove(ctx, appName, name string, force bool) error`
- `internal/sbx` — `(*Client).Stop(ctx, appName string, names ...string) error`
- `internal/sbx` — `(*Client).Exec(ctx, appName, name string, argv ...string) error` — buffered,
  used for the `sbx exec NAME true` liveness probe and the opencode launcher.
- `internal/sbx` — `(*Client).Ports(ctx, appName, name string) (string, error)`
- `internal/sbx` — `(*Client).SettingSet(ctx, appName, key, value string) error`
- `internal/sbx` — `(*Client).Interactive(ctx, appName string, io Streams, argv ...string) (int, error)`
  — inherits stdio, returns the child's exit code; `Streams` holds `Stdin io.Reader`,
  `Stdout`/`Stderr io.Writer`.
- `internal/lifecycle` — `Result{ExitCode int, Stdout, Stderr string}` with the JSON tags
  `exitCode`/`stdout`/`stderr`, and `(Result).Refused() bool` — **byte-identical to
  `script.Result` today**, because the dashboard consumes it.
- `internal/lifecycle` — `Runner{SbxClient *sbx.Client, WorktreeRoot, DefaultAgent string}`.
- `internal/lifecycle` — `StartOpts{Repo, Name, Agent, AppName string, Extra []string}`.
- `internal/lifecycle` — `(*Runner).Start(ctx, StartOpts) (Result, error)`,
  `(*Runner).Stop(ctx, repo, appName string, names ...string) (Result, error)`,
  `(*Runner).Destroy(ctx, repo, appName, name string, force bool) (Result, error)` — same
  signatures as `script.Runner` so `internal/server` changes only its imports and type names.
- `internal/lifecycle` — `(*Runner).List(ctx, repo, appName string) (Result, error)`,
  `(*Runner).Path(repo, name string) (Result, error)`.
- `internal/lifecycle` — `(*Runner).Attach(ctx, appName, name string, io sbx.Streams, args []string) (int, error)`,
  `(*Runner).ExecIn(ctx, appName, name string, io sbx.Streams, argv []string) (int, error)`.
- `internal/lifecycle` — `Refusal(format string, args ...any) error` plus `AsResult(err error) (Result, bool)`
  — a refusal becomes `Result{ExitCode: 1, Stderr: "error: …"}`; anything else is a real error.
- `internal/release` — `Client{BaseURL, APIURL string, HTTP *http.Client}` — both URLs are fields,
  not constants, so tests point them at `httptest`.
- `internal/release` — `(*Client).Latest(ctx) (string, error)` — the latest release's tag.
- `internal/release` — `(*Client).Update(ctx, currentVersion, execPath string, out io.Writer) error`
  — resolve, compare, download, verify, replace.

## Tasks

### T1: sbx gains the lifecycle commands
**Files:** Modify — `internal/sbx/sbx.go`, `internal/sbx/sbx_test.go`, `internal/sbxstub/stub.go`
**Deliverable:** every sbx invocation the script made is available as a scoped method, including
an interactive path that inherits stdin/stdout/stderr and returns the child's exit code.
**Contract:** `Create`, `Remove`, `Stop`, `Exec`, `Ports`, `SettingSet`, `Interactive`, `Streams`
as listed above.
**Behaviors to cover with tests:** each method builds the exact argv the script did, in the same
order, always prefixed with `--app-name`; an empty app-name returns `ErrNoAppName`; a failing sbx
surfaces the `classify` hints unchanged; `Interactive` returns a non-zero child exit code as a
value, not an error, and returns an error only when sbx could not be run at all.
**Gate:** `go test ./internal/sbx/...` green.

### T2: lifecycle argument handling
**Files:** Create — `internal/lifecycle/args.go`, `internal/lifecycle/args_test.go`
**Deliverable:** the script's parsing rules as pure functions, testable without git or sbx.
**Contract:** internal to the package.
**Behaviors to cover with tests:** `--agent A`, `--agent=A` mid-list and the resulting forwarding
order; a bare `--agent` with no value is a usage error (exit 2); `--` ends scanning and everything
after it forwards untouched; 4096-publish detection across `--publish 4096`, `-p 4096`,
`--publish=host:4096`, `4096/tcp`, `4096/tcp6`, and correctly *not* matching `14096:4096`'s host
half or an unrelated `--publish 5173`; name validation requires at least two characters of
letters, digits, dots and hyphens and rejects a leading dot or hyphen.
**Gate:** `go test ./internal/lifecycle/...` green.

### T3: repo discovery, worktree paths and start
**Files:** Create — `internal/lifecycle/repo.go`, `internal/lifecycle/lifecycle.go`, tests
**Deliverable:** `Start` creates the branch and worktree, calls `sbx create`, and unwinds both on
failure; an existing worktree resumes instead.
**Contract:** `Runner`, `Result`, `StartOpts`, `Refusal`, `AsResult`, `(*Runner).Start`.
**Behaviors to cover with tests:** against a real git repo in `t.TempDir()` and `sbxstub` —
default agent publishes 4096, `--agent` selects the agent and drops the auto-publish,
`DefaultAgent`/`SLUSS_AGENT` is inherited and overridden, an already-published 4096 is not
doubled, the worktree lands at `<root>/<repo>/<name>` on branch `agent/<name>`, both workspaces
(worktree and the repo's git dir) are passed to sbx in that order; a failed `sbx create` removes
the worktree *and* the branch and reports the failure; an existing worktree probes the sandbox
instead of recreating; a pre-existing non-directory at the worktree path, and a branch that exists
without its worktree, are two distinct refusals; running outside a git repository is a usage
refusal naming the repository, not a raw git error.
**Gate:** `go test ./internal/lifecycle/...` green.

### T4: stop, destroy, list, path
**Files:** Modify — `internal/lifecycle/lifecycle.go`, `internal/lifecycle/repo.go`, tests
**Deliverable:** the remaining non-interactive commands, with destroy's two refusals intact.
**Contract:** `(*Runner).Stop`, `(*Runner).Destroy`, `(*Runner).List`, `(*Runner).Path`.
**Behaviors to cover with tests:** destroy succeeds on a clean, merged worktree and calls
`sbx rm --force NAME` then removes worktree and branch with the non-force git flags; a dirty
worktree is refused with the script's "has uncommitted changes; commit them or use --force"
wording and sbx is never called; an unmerged branch is refused naming the primary branch; a
missing worktree is refused; `--force` removes sandbox, worktree and branch past both refusals
using `worktree remove --force` and `branch -D`; `stop` with no names is an error; `list` prints
the sbx listing and, inside a repository, the worktree list, and still works outside one; `path`
prints the worktree path without touching sbx.
**Gate:** `go test ./internal/lifecycle/...` green.

### T5: agent read-back, attach and exec
**Files:** Create — `internal/lifecycle/agent.go`, tests
**Deliverable:** `Attach` and `ExecIn`, with the agent resolved from sbx rather than from
anything host-side.
**Contract:** `(*Runner).Attach`, `(*Runner).ExecIn`.
**Behaviors to cover with tests:** the agent comes from `sbx ls --json` scoped by app-name;
opencode starts OpenCode Web on 4096 through the PID-file guard so a second attach starts no
second server, then prints the port mapping; claude sets `claude.remoteControl true` then runs
with `--remote-control`; copilot runs with `--remote`; any other agent runs bare; names match
exactly, so `gh` never resolves `gh-remote`; a name containing a backslash resolves correctly (the
awk `-v` escaping bug the script guards — the Go port must not reintroduce an equivalent); a
successful listing that names no agent warns and falls back to the default; a *failed* `sbx ls` is
an error that blames sbx and runs no agent; `ExecIn` with no command opens a shell, with a command
runs it, and both return the child's exit code.
**Gate:** `go test ./internal/lifecycle/...` green.

### T6: cmd/sluss
**Files:** Create — `cmd/sluss/main.go`, `cmd/sluss/usage.go`, `cmd/sluss/main_test.go`;
Delete — `cmd/slussd/`
**Deliverable:** one binary with the full surface: `start`, `list`/`ls`, `attach`/`run`, `exec`,
`stop`, `destroy`/`rm`, `path`, `serve`, `doctor`, `version`, `help`. (`update` arrives in T8.)
**Contract:** the CLI itself; `SLUSS_APP_NAME`, `SLUSS_AGENT` and `SLUSS_WORKTREE_ROOT` keep their
meanings, and lifecycle commands fall back to the config file's `worktreeRoot` when the
environment variable is unset.
**Behaviors to cover with tests:** every command and alias dispatches; `sluss` with no arguments
prints usage and exits 2; an unknown command exits 2 naming it; `COMMAND --help` and
`help COMMAND` both print that command's help and exit 0, for every command; a refusal prints
`error: …` to stderr and exits 1; usage errors exit 2; a missing `SLUSS_APP_NAME` is a clear
message, not an `ErrNoAppName` dump.
**Gate:** `go test ./cmd/... ./internal/...` green and `go build ./...` clean.

### T7: swap the server and doctor, delete the script
**Files:** Modify — `internal/server/server.go`, `internal/server/server_test.go`,
`internal/doctor/doctor.go`, `internal/doctor/doctor_test.go`; Delete — `internal/script/`,
`scripts/sluss`, `scripts/test-sluss`
**Deliverable:** nothing in the repo references a lifecycle script or the name `slussd`.
**Contract:** the HTTP API is unchanged — same routes, same `{exitCode, stdout, stderr}`.
**Behaviors to cover with tests:** the existing server tests pass with only the runner type
swapped, including the create-route guard, the `?force=true` exactness, the cross-site rejection
and refusals surfacing as a non-zero `exitCode` with the message in `stderr`; `doctor` no longer
reports on a script, reports `git` present or missing and the worktree root writable or not, and
still fails the process on any `fail` check; `doctor` and `serve` both work with no `SLUSS_SCRIPT`
set and no `~/.local/bin/sluss` present.
**Gate:** `go test ./...` green; `rg -n 'slussd|SLUSS_SCRIPT|scripts/sluss\b'` returns nothing
outside `docs/DECISIONS.md` and `.trailmix/`.

### T8: release resolution and `sluss update`
**Files:** Create — `internal/release/release.go`, `internal/release/release_test.go`;
Modify — `cmd/sluss/main.go`, `cmd/sluss/usage.go`
**Deliverable:** `sluss update` replaces the running binary from the latest GitHub release.
**Contract:** `release.Client` with `BaseURL`/`APIURL` fields, `Latest`, `Update`.
**Behaviors to cover with tests:** against `httptest` serving a fake release JSON, a real
`.tar.gz` built in the test, and a `checksums.txt` — an up-to-date version reports so and
downloads nothing; a newer release is downloaded, its SHA-256 checked against `checksums.txt`, and
the target replaced; a checksum mismatch aborts and leaves the original binary byte-identical; a
truncated or non-gzip download aborts the same way; the replacement is atomic (temp file in the
target's own directory, then rename), so an interrupted download never leaves a partial binary; a
symlinked target is refused with the script's "is a symlink; update the checkout it points at
instead" reasoning; an unwritable target directory is refused with a message naming it; a non-200
or rate-limited API response is a readable message, not a stack trace.
**Gate:** `go test ./internal/release/... ./cmd/...` green.

### T9: install and uninstall scripts
**Files:** Rewrite — `scripts/install.sh`; Create — `scripts/uninstall.sh`, `scripts/test-install`
**Deliverable:** `curl -fsSL …/scripts/install.sh | sh` installs or upgrades a verified binary;
the uninstaller removes it and nothing else.
**Contract:** `SLUSS_INSTALL_DIR` (default `~/.local/bin`), `SLUSS_VERSION` (default `latest`),
`SLUSS_BASE_URL` (default the GitHub releases URL, overridden by tests). Asset names are stable
and version-free — `sluss_<os>_<arch>.tar.gz` — so `latest/download/<name>` resolves without an
API call.
**Behaviors to cover with tests:** `scripts/test-install` serves a fake remote over `file://` and
asserts — `uname` output maps to the four supported targets (`Darwin`+`arm64`, `Darwin`+`x86_64`,
`Linux`+`x86_64`, `Linux`+`aarch64`), an unsupported pair exits non-zero naming it; the archive's
checksum is verified before anything is replaced and a mismatch leaves an existing installation
untouched; a truncated archive and an HTML error page both fail cleanly; installing over an
existing binary upgrades it atomically; `SLUSS_VERSION` pins a tag; a missing `curl` and a missing
`sha256sum`/`shasum` are named as prerequisites; the PATH advice appears only when the directory
is not on `PATH`, and is shell-agnostic; uninstall removes the binary, is a no-op with a clear
message when nothing is installed, and prints — without deleting — where config, worktrees and
sandboxes remain.
**Gate:** `bash -n` and `shellcheck` clean on both scripts, and `./scripts/test-install` green.

### T10: build, CI and release workflow
**Files:** Modify — `Taskfile.yml`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`
**Deliverable:** a `v*` tag publishes four archives and `checksums.txt`; CI gates the new layout.
**Behaviors to cover with tests:** verified by running the gates, not by unit tests — `task
build` produces `dist/sluss`; `task check` runs gofmt, vet, `go test ./...`, `bash -n` and
shellcheck over `install.sh`/`uninstall.sh`/`test-install`, and `./scripts/test-install`; the
`test:script` target is gone. The release workflow must **build the dashboard before the Go
binary** — it does not today, which would ship a binary embedding the `.gitkeep` placeholder
instead of the UI; a workflow step asserts the built binary serves a non-placeholder index.
**Gate:** `task check` green locally; `act` is not required — the workflow is reviewed by reading,
and proven by the first tag.

### T11: documentation
**Files:** Rewrite — `README.md`; Modify — `AGENTS.md`, `docs/DECISIONS.md`
**Deliverable:** a README that someone — or an agent — can set up a deployment from without
reading the source.
**Behaviors to cover with tests:** none; reviewed by reading. It must cover: what sluss is;
install, update and uninstall by curl including the environment overrides and the checksum step;
every command with its options and at least one example, ported from the script's per-command
help; the complete `config.json` with every key, its default and its effect; running as a
long-lived service; **how the sandbox web UIs are exposed** — path mode's
`http://<host>:<port>/s/<scope>/<name>/` and host mode's `https://<prefix><name>.<domain>/`, what
each needs (nothing, versus wildcard DNS and a certificate), which agents are proxied (opencode)
and which are deep-linked instead (claude, copilot); the security posture (D15) stated plainly as
*not* authentication; and development gates. `AGENTS.md` loses "Two binaries, two names" and
"lifecycle lives in the script" and gains what replaced them. `docs/DECISIONS.md` gains **D23 —
lifecycle moves into Go** (superseding D14, recording that the GUI and CLI now share one code path
in-process rather than by subprocess, and that the cost is a rewritten implementation reviewed
against the script's tests) and **D24 — one binary named `sluss`** (superseding D16, recording
that the `PATH` collision D16 avoided cannot occur once only one thing is installed).
**Gate:** `task check` green; README links resolve; no homelab-specific content.

## Tests

- AC1 → T3 (start creates branch, worktree, sbx create; unwind on failure)
- AC2 → T2 + T3 (`--agent` consumption, `--` forwarding, 4096 auto-publish, resume)
- AC3 → T4 (destroy refusals and `--force`)
- AC4 → T5 (agent read-back and per-agent attach)
- AC5 → T4 + T5 (list, exec, stop, path; running from a worktree; attach from anywhere)
- AC6 → T7 (serve and doctor with no script; doctor's replacement checks)
- AC7 → T7 (server tests pass with only the runner type swapped)
- AC8 → T9 (install script: os/arch, checksum, upgrade in place)
- AC9 → T9 (uninstall script)
- AC10 → T8 (update: up-to-date, checksum, atomic replace, symlink and permission refusals)
- AC11 → T10 (release workflow: four archives, checksums, version reported)
- AC12 → T10 + T7 (`task check` green; no `slussd` or script references survive)
- AC13 → T11 (README coverage)

## Risks / trade-offs

- **The port is the risk.** ~600 lines of bash with subtle rules — awk-based agent extraction,
  `${arr[@]+"${arr[@]}"}` empty-array guards, exit-code propagation through `return` — become Go.
  Mitigation: `scripts/test-sluss` already encodes the sharp edges as black-box assertions, and
  T2–T5 port them case by case before the script is deleted in T7. The script stays in git
  history to diff against.
- **Destroy is where a port bug costs real work.** The two refusals are the only thing standing
  between a dashboard click and a discarded afternoon. They get tests before the implementation.
- **Interactive commands lose a shell's signal behaviour.** `attach` and `exec` ran as bash
  children before; in Go, Ctrl-C must reach the child and not merely kill sluss. sluss should
  ignore SIGINT for the duration of an interactive child and return that child's exit code.
- **`git describe` versus release tags.** `task build` stamps `git describe`, the workflow stamps
  the tag. `update` compares its own `version` string to a release tag, so a locally built `dev`
  binary always looks out of date — acceptable, and worth one line in the update output.
- **Asset names without a version** make `latest/download/<name>` work with no API call in the
  installer, at the cost of archives that do not self-identify once downloaded. `sluss version`
  covers it.
- **No API-less path for `update`.** Unlike the installer, `update` needs the latest *tag* to
  compare against, so it calls the GitHub API and inherits its unauthenticated rate limit. It
  degrades to a message telling the user to re-run the install script.
- **`sbx create`'s argument order is a guess in one place:** the script passes
  `--name N [publish] [extra…] AGENT WORKTREE GITDIR`. The port must reproduce it exactly rather
  than tidy it, since sbx's tolerance for reordering is unverified.

## Open questions
- None.

## Amendments
- 2026-08-29 — **`internal/server` takes a `Lifecycle` interface, not `*lifecycle.Runner`.** The
  plan assumed the server tests would pass with only a type swap. They would not: they faked the
  runner with a shell script and asserted on its recorded argv, and that mechanism no longer
  exists. An interface declared where it is consumed lets the route tests keep asserting what they
  actually mean — what the routes asked lifecycle to do — without wiring a real git repository and
  a real sbx into 1034 lines of HTTP tests. Routes, status codes and JSON are unchanged.
- 2026-08-29 — **`sbx.Remove` has no `force` parameter.** The plan's contract carried one, but the
  retired script always passed `sbx rm --force` regardless of its own `--force`: sbx's flag means
  "do not prompt", not "discard work". The parameter would have been dead. Whether the worktree
  may be discarded is still decided in `lifecycle.Destroy` (D20), unchanged.
- 2026-08-29 — **`Runner.Agent`, not `Runner.DefaultAgent`.** The package-level constant
  `DefaultAgent` took that name.
- 2026-08-29 — **`release.Client.Update` takes an extra `version` parameter** —
  `(ctx, current, version, execPath, out)`, not the contract's
  `(ctx, currentVersion, execPath, out)`. It is what lets `SLUSS_VERSION` pin a tag and skip the
  API call entirely; covered by `TestUpdateWithAnExplicitVersionSkipsTheAPI`.
- 2026-08-29 — **`gitfacts.Run` and `gitfacts.OK` are exported and shared** rather than lifecycle
  growing a second git runner. A new package for one function would have gone against AGENTS.md's
  flat-package rule.
- 2026-08-29 — **T8's `internal/release` landed with T6**, because `cmd/sluss` dispatches `update`
  and would not build without it. Both gates were run.
- 2026-08-29 — **The command line now requires an sbx scope.** The script fell back to sbx's own
  default scope when `SLUSS_APP_NAME` was unset; `internal/sbx` requires an app-name by the
  chokepoint rule, so `sluss` resolves one from the environment, then the configuration file, and
  reports a readable error naming both if there is none. Recorded in D23.
- 2026-08-29 — **`exists()` uses `Lstat`, so a dangling symlink counts as occupied.** The script's
  `[[ -e ]]` followed symlinks, so a dangling one was *not* occupied and `git worktree add` was
  attempted. sluss now refuses with "worktree path already exists" instead. A deliberate
  divergence from the port — a clearer message than the git failure it replaces — recorded in D23.
- 2026-08-29 — **AC12's literal wording is not met, by intent.** `docs/SPEC.md` and `docs/SPIKE.md`
  keep `slussd` and `scripts/sluss` mentions where they describe historical state, which is what
  T11 asked for ("corrected names rather than a rewrite"). AC12 should have said "outside
  documents that are explicitly history".
- 2026-08-30 — **Post-review: `scripts/install.sh` rejects a non-regular `sluss` entry.** Review
  finding M1: `[ -f ]` follows symlinks, so an archive matching its own `checksums.txt` but
  containing a symlink named `sluss` would have been `chmod 0755`'d and executed — a divergence
  from `internal/release.extract`, which guards with `tar.TypeReg`. The archive is now unpacked
  into its own directory and `-L` is tested before `-f`. Covered by a new `scripts/test-install`
  case, mutation-tested to confirm it fails without the guard.
