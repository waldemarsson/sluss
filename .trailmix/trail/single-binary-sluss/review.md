---
slug: single-binary-sluss
waypoint: review
status: draft
updated: 2026-08-30
findings: M1:fixed M2:fixed L1:fixed L2:fixed L3:fixed
---

# Review: One binary named sluss

Independent read-only review of `git diff main..HEAD` on `single-binary-sluss` (5 commits, 54
files, +5500/−2203), against the approved `spec.md` and `plan.md`. This **replaces** an earlier
self-review written by the implementing agent; where the two disagree, this one stands.

**Verdict: ready to proceed — with fixes.** No functional or correctness defect was found in the
ported lifecycle, in destroy's safety behaviour, or in the server's request/response contract.

## What was verified, not assumed

- **The port is faithful.** argv order, refusal wording, `--agent` / `--` / 4096-publish parsing,
  and the destroy refusal ordering were traced line by line against `git show main:scripts/sluss`
  and match.
- **The rewritten server tests are not vacuous.** `fakeRunner` was traced through `runner(t)` →
  `server.New` → `s.runner.Start/Stop/Destroy` in the handlers. The assertions genuinely run.
- **`internal/sbx` is the sole chokepoint** — no stray `exec.Command("sbx", …)` anywhere.
- **`internal/proxy` is untouched**, and `?force=true` remains exact.
- **Checksum verification precedes any write** in both `scripts/install.sh` and `internal/release`.
- **All 5 commits and HEAD** build, vet and test clean — reproduced independently, not trusted.

## Findings

### HIGH
None.

### MEDIUM

**M1 — `scripts/install.sh` accepted a non-regular `sluss` entry.** *Fixed.* The post-extraction
check `[ -f "${tmp}/sluss" ]` follows symlinks and rejected no other entry type, so an archive that
matched its own `checksums.txt` — a compromised CI, or a build-tooling bug — could ship a symlink
named `sluss` pointing anywhere; it would pass `-f`, be `chmod 0755`'d and then executed by the
sanity check. `internal/release.extract` guards exactly this with `tar.TypeReg`; the shell path had
no equivalent, so the two implementations of one job diverged in hardening. `scripts/test-install`
had no case covering it either.

*Fix:* the archive is unpacked into its own directory (so a hostile archive cannot overwrite the
asset or the checksums it was verified against), and `-L` is tested before `-f`. A new
`test-install` case covers it, and was mutation-tested — with the guard removed the suite fails,
and the failure output shows the old code reaching `chmod` on the link's target.

**M2 — `plan.md`'s Amendments section said "None".** *Fixed.* The self-review claimed the
mandatory-app-name behaviour change was "recorded in D23 and in the plan's amendments". Only the
first was true: the edit that was supposed to write the amendments never applied, and the failure
was not noticed. Ten amendments are now recorded, including the ones this review surfaced.

### LOW

**L1 — `exists()` uses `Lstat`, diverging from the script's `[[ -e ]]`.** *Fixed (documented.)* A
dangling symlink at the worktree path now counts as occupied and `start` refuses, where the script
would have attempted `git worktree add`. Better behaviour, but a divergence from "port, not
redesign" that was only in a code comment. Now recorded in D23 and in the amendments.

**L2 — `release.Client.Update`'s signature deviates from the plan's public contract.** *Fixed
(documented.)* It takes an extra `version` parameter, which is what lets `SLUSS_VERSION` pin a tag
and skip the API call. Functionally fine and covered by tests, but it changed a section the plan
labelled "keep stable across tasks". Now an amendment.

**L3 — AC12's literal wording is not met.** *Fixed (documented.)* `docs/SPEC.md` and
`docs/SPIKE.md` still mention `slussd` and `scripts/sluss` where they describe historical state,
which is what T11 actually asked for. The acceptance criterion should have excluded documents that
are explicitly history. Now an amendment; no code change.

## Acceptance criteria

| AC | Result |
|---|---|
| AC1 start, create, unwind | pass |
| AC2 `--agent` / `--` / 4096-publish / resume | pass |
| AC3 destroy refusals and `--force`, wording verbatim | pass |
| AC4 attach per agent, read-back, idempotent PID file | pass |
| AC5 list, exec, stop, path | pass |
| AC6 serve and doctor with no script | pass — `checkWorktreeRoot` creates nothing |
| AC7 server JSON and tests unchanged | pass — wiring verified genuine |
| AC8 install script | pass, with M1's hardening gap (now fixed) |
| AC9 uninstall script | pass |
| AC10 `sluss update` | pass |
| AC11 release workflow | not independently verified — needs a real tag |
| AC12 `task check` green | pass; literal "no stray reference" wording not met (L3) |
| AC13 README coverage, no homelab content | spot-checked; no homelab strings |

## Not covered

- A real release. The workflow is reviewed by reading; only a `v*` tag proves it.
- Anything touching a real `sbx` — by design (AGENTS.md).
- `README.md` was spot-checked rather than read in full.
