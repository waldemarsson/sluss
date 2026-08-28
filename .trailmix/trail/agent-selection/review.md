---
slug: agent-selection
waypoint: review
status: draft
updated: 2026-08-25
findings: M1:fixed M2:fixed M3:fixed L1:fixed L2:fixed L3:fixed L4:fixed L5:fixed M4:fixed L6:fixed
---

# Review — agent-selection

Independent read-only pass by the `reviewer` agent over the uncommitted diff
(`scripts/sluss`, `README.md`) against `spec-plan.md`. Report transcribed verbatim.

**Strengths:** T1's `--agent` loop (`scripts/sluss:306-332`) is a clean, well-tested state
machine that correctly excises `--agent`/`--agent=` while preserving forwarded-arg order
(verified live: `--kit x --agent opencode --cpus 4` → sbx sees `--kit x --cpus 4 opencode ...`
with the resolved agent, not the global, driving `--publish 4096` at `scripts/sluss:362`);
`extract_agent` (`scripts/sluss:235-257`) is a genuinely careful, portable POSIX-ERE awk
one-liner that I independently fuzzed against absent sandboxes, unquoted-null agents,
multiline pretty-printed JSON, and nested `workspaces` objects — all degraded correctly to
empty output rather than misattributing; T2's `run_sbx settings set` fix (`scripts/sluss:424`)
and README updates are accurate and match behavior.

## HIGH

None.

## MEDIUM

- **M1** · `scripts/sluss:262-268,405,424` · a hard `sbx ls` failure (daemon down, not
  authenticated) is indistinguishable from "no agent recorded" — reproduced live: `sbx ls`
  exiting 1 with stderr `daemon not running` still yields `warning: sbx reported no agent for
  whatever; assuming claude`, then proceeds to call `settings set` and `run --remote-control`
  → this conflates "sbx explicitly reports null" (the case T3 designed the warning for) with
  "sbx is broken/unreachable," producing a misleading diagnosis exactly when the
  source-of-truth read fails → capture `run_sbx ls --json`'s own exit status (e.g. via `set -o
  pipefail` locally or a temp var) and fail loudly instead of guessing when the *command*
  failed, reserving the guess+warn path for a genuinely empty/no-match JSON response.
  (Independently reproduces self-review's M2; confirmed by running the diff, not just reading
  it.)
- **M2** · `scripts/sluss:308,361-366` (and pre-existing `publish_args` at 361) · `local -a
  create_args=()` then later `"${create_args[@]}"` under `set -u` is a known bash <4.4 gotcha
  (empty declared arrays can trigger "unbound variable"); macOS ships bash 3.2 as `/bin/bash`,
  and `#!/usr/bin/env bash` resolves there unless a newer bash precedes it on PATH — the target
  host is explicitly macOS per the probe notes → this diff doubles the exposure by adding a
  second empty array with the same pattern; guard with `${create_args[@]+"${create_args[@]}"}`
  or document/enforce a minimum bash version. Not a new regression (the idiom pre-dates this
  diff) but worth fixing now that it's touched twice.
- **M3** · (environment-wide) · shellcheck/shfmt/bats are absent and `bash -n` only checks
  syntax, so none of T1's arg-parsing edge cases or T3's awk extractor logic are protected by a
  repeatable, automated check — all verification (mine and the self-review's) was manual and ad
  hoc; a future edit to `extract_agent` or the `--agent` loop has nothing to regress against.

## LOW

- **L1** · `scripts/sluss:393-405` · `attach_environment` no longer requires being inside a Git
  repo (its only use of `repo_root` was building the metadata path, now gone) — a real,
  untested-elsewhere behavior change, not documented in `attach_usage` or README.
- **L2** · `scripts/sluss:56-58` · doc doesn't say explicitly whether `--agent` is honored or
  silently ignored on a *second* `sluss start NAME --agent X` (worktree already exists) — it is
  silently ignored (parsed, then discarded via the early `if [[ -d "$worktree" ]]` return at
  line 345), matching the "ignored on subsequent starts" rule for other args but not stated for
  `--agent` by name.
- **L3** · `scripts/sluss:306-332` · no `--` escape hatch: a literal `--agent` intended as a
  positional value for `sbx create` (after a `--` separator) is still intercepted by sluss's
  scanner since the loop doesn't special-case `--`. Narrower than the documented "shadows a
  future sbx `--agent`" caveat; worth a one-line note if it ever bites.
- **L4** · `scripts/sluss:233-234` · the extractor comment "Sandbox and agent names cannot
  contain quotes or backslashes" is true of sbx-emitted JSON but not of the raw `NAME` argument
  `attach` forwards into `awk -v want="$1"` — POSIX `-v` does backslash-escape processing on its
  value, so an adversarial/typo'd `sluss attach 'a\tb'` could silently mismatch. Harmless (falls
  back to a warning), self-inflicted, not worth more than a comment fix.
- **L5** · (matches self-review L4) · pre-existing `.sluss/<name>.agent` files are orphaned, not
  cleaned up by `destroy_environment` (confirmed: no reference to the old path remains anywhere
  in the script). Accepted by spec-plan as inert; fine.

## Spec compliance

- [x] **T1** — `--agent`/`--agent=` consumed, never forwarded to `sbx create`; precedence
  `--agent` > `$SLUSS_AGENT` > `opencode`; resolved agent drives `--publish 4096`;
  `start_usage`+README updated (all verified live).
- [x] **T2** — `settings set claude.remoteControl true` now goes through `run_sbx` (`--app-name`
  now applies).
- [x] **T3** — metadata file and all three of its use sites removed; `environment_agent` reads
  back via `sbx ls --json`/`extract_agent`; falls back to `$SLUSS_AGENT` with a stderr warning —
  but the warning path also silently absorbs true `sbx` command failures (M1).
- [x] **Gates** — `bash -n scripts/sluss` passes; absence of shellcheck/shfmt/bats is an accepted
  environment constraint, flagged again above as a testing gap (M3).

## Verdict

**With fixes.** No HIGH-severity bugs and the core T1/T2/T3 behavior is correct and verified by
both this review and the implementer's own testing, but M1 (masking real sbx failures as "no
agent") and M2 (empty-array-under-`set -u` risk on the stated macOS target) should be addressed
before treating this as done, and M3 (no repeatable test for the new arg-parsing/awk logic) is
worth at least a lightweight manual-test checklist committed alongside the script.

---

# Appendix — implementer's prior inline self-review

Superseded by the independent pass above; retained for the behaviours it recorded as tested.
Its finding ids are NOT the canonical ones — the ids above are.

## Verified behaviours

- `--agent` / `--agent=` / absent; `SLUSS_AGENT` inheritance; `--agent` overriding it.
- Resolved agent drives the `--publish 4096` default: `SLUSS_AGENT=claude sluss start fox
  --agent opencode` still publishes 4096. This was the latent bug in keying off the global.
- Pass-through args reach `sbx create` unchanged and in order; `--agent` never does.
- Bare `--agent` exits 2 with a message.
- attach dispatches opencode/claude/copilot correctly from `sbx ls --json`.
- Unknown sandbox warns on stderr, then falls back.
- `--app-name` propagates to `ls`, `settings`, `run`, `ports`.

---

## Re-review (2026-08-25)

Delta pass by the `reviewer` agent over the eight fixes. Transcribed verbatim.

**Strengths:** all eight prior findings are genuinely fixed and I verified each by
mutation-testing the fix back out (reverting `environment_agent`'s exit-status capture,
reverting the `${arr[@]+"${arr[@]}"}` guard, reverting `extract_agent` to `-v want="$1"`) and
re-running `scripts/test-sluss`; `task check` was independently confirmed (via a throwaway
Taskfile) to abort with a non-zero exit when a chained sub-task fails, so the new test gate is
real, not decorative.

- **M1** · held · `environment_agent` (`scripts/sluss:266-280`) captures `run_sbx ls --json`'s
  own exit status and `fail`s + explicit `return 1` before any guess; only a successful-but-empty
  listing falls to the warn-and-guess path. Reverting the capture makes `test-sluss` fail exactly
  the expected assertion — confirms the test discriminates correctly.
- **M2** · held · both `create_args` and `publish_args` expansions (`scripts/sluss:381,384-385`)
  use `${arr[@]+"${arr[@]}"}`. Correct idiom. Note (not a new finding): this container's bash 5.2
  doesn't manifest the bash-3.2 unbound-array bug either way, so `test-sluss` can't and doesn't
  discriminate M2 here — same residual, target-host-only risk already acknowledged in the
  original review.
- **M3** · held · `scripts/test-sluss` (new, 238 lines, 29 assertions) is wired via
  `task test:script` into `task check`; genuinely fails the build on a failing assertion.
- **L1** · held · `attach_usage` and README both state attach runs from any directory; matches
  code (`attach_environment` no longer calls `repo_root`).
- **L2** · held · `start_usage`/README explicitly state `--agent` is ignored when restarting an
  existing sandbox; matches code (parsed unconditionally, but discarded by the pre-existing
  `[[ -d "$worktree" ]]` early return).
- **L3** · held · `--` correctly ends sluss's own scanning and forwards the remainder in original
  order (`scripts/sluss:322-326`); verified by reading and by the new test, no
  swallowing/reordering found.
- **L4 (code)** · held · `extract_agent`'s `SLUSS_WANTED_NAME`/`ENVIRON[...]` approach is correct
  and portable — independently confirmed with mawk that `ENVIRON` does not backslash-process its
  value (unlike `-v`), and `ENVIRON` is POSIX/BSD-awk/onetrue-awk/mawk/gawk-universal.
- **L5** · held · README documents `.sluss/<name>.agent` as removable legacy state; `grep`
  confirms zero remaining code references. Acceptable resolution, agreed.

### New findings introduced by the fixes

(Reported as N1/N2; tracked in frontmatter as **M4** and **L6**, since finding ids are H/M/L.)

- **N1 (MEDIUM)** · `scripts/test-sluss:205-208` · the new "backslash in a sandbox name" test is
  a false positive: it only asserts the not-found warning fires, but no fixture sandbox has a
  name containing a literal backslash, so the correct (`ENVIRON`) and the old buggy
  (`-v want="$1"`) implementations produce byte-identical output for this case → I verified this
  by reverting `extract_agent` to `awk -v want="$1"` and re-running the whole suite: **all 29
  assertions still pass**, including this one → the test currently gives false assurance that L4
  is regression-protected when it is not → fix: add a fixture sandbox literally named with a
  backslash sequence (e.g. `"a\\tb"`) and assert `sluss attach 'a\tb'` resolves and dispatches to
  *its* agent, not just that a warning is printed for a nonexistent name.
- **N2 (LOW)** · `scripts/test-sluss:47` · `export PATH="$work/bin:$PATH"` in `setup()` is never
  restored between test blocks, so `PATH` accumulates prefixes pointing at already-`rm -rf`'d
  mktemp dirs across the run; harmless (each setup's fresh stub always wins) but untidy —
  snapshot the original `PATH` once and restore it, or reset from it, each `setup`.

### Verdict

**With fixes.** All eight originally-reported findings are correctly and verifiably fixed, with
no regressions found in the touched code. The one new issue (N1) is confined to the new test
file, not production behavior — the L4 production fix is independently confirmed correct — but it
directly matches the "test that silently passes for the wrong reason" risk called out for
scrutiny, so it should be fixed before treating `test-sluss` as a trustworthy regression backstop
for L4. N2 is cosmetic.

### Resolution of M4 (N1) and L6 (N2) — implementer

Both fixed. The fixture gained a sandbox named `a\\tb` (valid JSON, decoding to a literal
backslash) with a distinct agent, and the test now asserts the *dispatch* rather than a warning.
`PATH` is snapshotted once at startup and each `setup` resets from that snapshot.

The suite was then mutation-tested against three reverted fixes to prove the assertions
discriminate:

| Reverted fix | Result |
|---|---|
| L4 — `ENVIRON` back to `awk -v want="$1"` | 2 of 30 fail |
| M1 — exit-status capture removed | 1 of 30 fails |
| T1 — publish default keyed off the global agent | 3 of 30 fail |

Restored: 30 assertions pass. One caveat: under the M1 mutation only the stderr assertion
discriminates — `assert_status … 1` passes coincidentally, because the stubbed sbx fails every
call, so the fallback path exits non-zero anyway.
