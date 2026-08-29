---
slug: lifecycle-actions
waypoint: review
status: approved
updated: 2026-08-29
findings: M1:fixed L1:fixed L2:fixed L3:fixed
---

Strengths: `internal/server/server.go:323` parses force as an exact `"true"` and
`internal/server/server_test.go:565` pins `1`/`yes`/empty as non-forcing — the typo case degrades
to the safe path and is tested, not assumed. `internal/script/script.go:87` makes force a required
positional argument, so omission cannot discard work, and `script_test.go:270` covers the false
case explicitly. `handleStart` (`server.go:287`) takes the repo from the snapshot rather than the
body, keeping `handleCreate` the sole path where a client names a directory; `server_test.go:556`
asserts the script's working directory to prove it. `+page.svelte:19` models force as one
`"scope/name"` string, so cross-row arming is impossible by construction rather than by cleanup,
and `+page.test.ts` tests that failure mode directly.

HIGH:
- none.

MEDIUM:
- M1 · internal/server/server.go:287 · `handleStart` has no test for an already-running sandbox,
  which AC7 explicitly covers ("the route is harmless if called for one anyway"). The UI half is
  tested; the route half is not. The script's resume path is `sbx exec NAME true`, so it is
  believed harmless — but that belief is currently unverified at the route level. Fix: add a
  subtest posting start to the running fixture (`auth`) and assert 200 plus `start auth` in argv.

LOW:
- L1 · web/dashboard/src/routes/+page.svelte:113 · The comment says the checkbox "is disarmed
  afterwards either way", but a dismissed confirm returns at line 118 before `forcing = ''`, so
  force stays armed after a cancel. The behavior is defensible (the user's intent survives a
  mis-tap on the dialog) and safe (the next confirm still names the cost), but the comment
  overstates it. Fix: say "after a destroy runs", and add a test pinning the cancel case so the
  choice is deliberate.
- L2 · web/dashboard/src/routes/+page.svelte:101 · `stop()` still builds its busy key with an
  inline `` `${sandbox.scope}/${sandbox.name}` `` while `start()` and `destroy()` use the new
  `keyOf()` helper introduced two lines above. Harmless, but the file now has two spellings of one
  concept. Fix: use `keyOf(sandbox)` in `stop()` too.
- L3 · web/dashboard/src/routes/+page.svelte:250 · The force checkbox is not disabled while
  `busy !== ''`, unlike every button beside it. Ticking it during an in-flight destroy has its
  value wiped by `forcing = ''` when that destroy returns. Fails safe (toward not forcing) and is
  a narrow window, but it is a silent state loss on a safety control. Fix: add
  `disabled={busy !== ''}`.

Spec compliance:
- [x] AC1 — start action present (`+page.test.ts`, both layouts) and the route runs `start <name>`;
      the "returns to running within one poll" half is integration-level and not runnable here
      (AGENTS.md: no test may shell out to a real sbx).
- [x] AC2 — `TestLifecycleOnAnUnknownSandbox` asserts 404 and no script run.
- [x] AC3 — `TestStartResumesAKnownSandbox` asserts the script's cwd is the configured repo.
- [x] AC4 — `TestDestroyForceIsExplicit/absent` (409) and the component test rendering stderr
      verbatim.
- [x] AC5 — `TestDestroyArgv/forced` plus `TestDestroyForceRemovesDirtyWork` against the real
      script, which verifies the worktree and `agent/web` branch are actually gone.
- [x] AC6 — "a dismissed confirm destroys nothing" asserts zero fetches.
- [~] AC7 — the "not offered when running" half is tested in both layouts; the "route is harmless"
      half is untested. See M1.

Verdict: With fixes — no defects in the shipped behavior; M1 closes a stated AC's untested half,
and the three LOWs are consistency and comment-accuracy issues on a safety-relevant control.

## Re-review (2026-08-29)
- M1 · held — `internal/server/server_test.go:556` posts start to the running `auth` fixture and
  asserts 200 plus `start auth` in the recorded argv. Verified in isolation:
  `go test ./internal/server/ -run TestStartOnARunningSandboxIsHarmless -v` → PASS. AC7 is now
  covered on both halves.
- L1 · held — `+page.svelte:113` now states the actual rule (a destroy that runs disarms; a
  dismissed confirm leaves it armed, and the next confirm still names the cost), and
  `+page.test.ts` pins it with "force stays armed when the confirm is dismissed", asserting zero
  fetches and a still-checked box. Verified in isolation → 1 passed.
- L2 · held — `stop()` at `+page.svelte:101` now uses `keyOf(sandbox)`; the inline spelling is
  gone from the script block, leaving `keyOf` the only one.
- L3 · held — `disabled={busy !== ''}` added to the force checkbox in both the table
  (`+page.svelte:252`) and card (`+page.svelte:307`) layouts, matching the buttons beside it. The
  mid-flight wipe window is closed.
- No new findings. The fixes are additive: no existing assertion was weakened or removed, and the
  web suite went 25 → 26 tests with no reclassification.

Gates after fixes: `task fmt:check`, `task vet` clean; `task test` all packages ok;
`task check:web` 263 files 0 errors; `task test:web` 26 passed. `shellcheck` still not installed
in this container — no shell script is touched by this trail (`git diff --stat -- scripts/` empty).

Verdict: Yes — all four findings verified fixed, no regressions, every AC covered by a runnable
test except the integration halves that require a real sbx on the host Mac.
