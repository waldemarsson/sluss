---
slug: frontend-tidy-up
waypoint: review
status: approved
updated: 2026-08-29
findings: H1:fixed M1:fixed M2:fixed M3:disputed L1:fixed L2:fixed L3:fixed
---

**Caveat: this is a self-review.** The same session wrote the code, so it is weaker evidence than
an independent pass — it is likelier to miss an assumption than to miss a typo.

Strengths:
- `api.ts:120` — `command()` exists because the implementation checked what the server actually
  returns rather than assuming. `handleSecretSet`, `handleSecretDelete` and `handleKitWrite` all
  answer 204; a single JSON-parsing helper would have turned every successful save into an error.
- `+page.svelte:193-247` — the five snippets remove ~140 duplicated lines while the 15 existing
  tests pass with zero edits — the file is byte-identical to the formatter commit once whitespace
  is stripped — which is real evidence the refactor was behaviour-preserving rather than an
  assertion that it was. (Re-checked 2026-08-29 after the Prettier config was corrected to the
  house standard; the earlier note said 9 trailing commas differed, an artefact of the derived
  config that set trailingComma "all".)
- `Secrets.svelte:32-40`, `Kits.svelte:46-52` — the ordering guard was proved red-then-green, and
  the ticket covers mutation-triggered refreshes too, not just the reported scope switch.
- `.prettierrc` — config derived from the existing source (tabs, single quotes, 100 cols), which
  held the reformat to 193/-118 across 17 files instead of rewriting everything.

HIGH:
- H1 · `lib/Secrets.svelte:44-52` · `send()` assigns `error = result.message`, then calls
  `refresh(scope)`, whose first statement is `error = ''`. The assignment is dead: a failed secret
  PUT or DELETE can never display its message. The user gets only indirect signal (the name is
  absent, or still present after a failed delete) and never the server's reason. Pre-existing —
  the original `finally { await refresh(scope) }` had the same order — but T4 rewrote exactly
  these lines and the new comment above the call draws attention to the refresh without noticing
  it wipes the error. Fix: have `refresh` take a flag, or capture and restore the mutation's
  message after the refresh settles, and cover it with a test for a refused write.

MEDIUM:
- M1 · `vite.config.ts:31-35` · the comment under the `unit` project still says there is "no
  framework-free module to cover here yet, so `test:unit` carries --passWithNoTests. Drop that
  flag once src/ grows one." Both halves are now false: `api.ts` is that module and the flag is
  gone. A stale comment directly contradicting the three lines above it, in a change whose own T6
  existed to fix a comment that overstated. Fix: replace it with what the suffix split means.
- M2 · `lib/access.svelte.ts:6`, `lib/fleet.svelte.ts:7` · `complete()` was inserted between the
  class's doc comment and the class itself. In both files the block describing `Access` / `Fleet`
  now sits immediately above `complete()` and reads as its documentation. Fix: move `complete()`
  above the class comment, or below the class.
- M3 · `Taskfile.yml:97` · `task check` — described as "What must pass before finishing a change
  (AGENTS.md)" — runs `fmt:check`, `vet`, `test`, the shell checks and `test:script`, but not
  `check:web` or `test:web`. The new Prettier gate is therefore only reachable locally by knowing
  to run `task check:web`. CI does enforce it. Pre-existing omission, but T1 is when the frontend
  first gained a gate worth wiring in. Fix: add `check:web` and `test:web` to `check`, or say in
  the desc that the frontend has its own task.

LOW:
- L1 · `vite.config.ts:43` · the component project now sets `exclude`, which replaces Vitest's
  defaults (`**/node_modules/**`, `**/dist/**`) rather than adding to them. Harmless today because
  `include` is scoped to `src/`, but this session accidentally created `src/routes/node_modules`
  by running npm from the wrong directory — the exact case the default guards. Fix: spread the
  defaults, or keep the exclude list narrow and note why.
- L2 · `lib/Kits.svelte:33-42` · `refreshList()` never clears `error`, so a message from a failed
  list survives a later successful one. Pre-existing; T4 rewrote the function without addressing
  it. `loadSpec` and `refresh` both clear on entry, so the inconsistency is now visible in one file.
- L3 · `lib/Secrets.test.ts:9`, `lib/Kits.test.ts:52` · both ordering tests prove a negative by
  waiting `setTimeout(20)`. It is deterministic in practice — the released chain is microtasks and
  drains before any timer — but the constant reads as a flake-prone sleep. `setTimeout(0)` plus a
  line saying why would carry the same guarantee more honestly.

Spec compliance:
- [x] AC1 — `prettier --check` passes; CI gates it before the Chromium download.
- [x] AC2 — five snippets; nine-column table and card rendering both intact, 15 tests unmodified.
- [x] AC3 — red-green proved for both the scope switch and the kit switch.
- [x] AC4 — one rule in `messageFor`; secrets and kits now surface `stderr`, which they dropped before.
- [x] AC5 — met as amended 2026-08-29. Six `access.config` guards gone; the two `fleet.snapshot?.`
      guards remain and are documented as covering the nullable "no poll yet" state.
- [x] AC6 — `MediaQuery` in place, no manual listener, comment names all four breakpoint sites.
- [x] AC7 — red-green proved, and confirmed visually at 390px across all four cards.
- [x] AC8 — `test:unit` runs 7 real tests without the flag.

Verdict: With fixes — H1 is a dead error path on a secrets write and should land before the
branch does; M1 and M2 are minutes of work and both are comment-accuracy regressions in a change
that was largely about comment accuracy.

## Re-review (2026-08-29)
- H1 · held — `Secrets.svelte:47-52` reports the refusal after `refresh(scope)` settles. Proved
  red-green: `shows why a write was refused, alongside the refreshed list` fails with the old
  ordering and passes with the new one.
- M1 · held — `vite.config.ts:26-30`. The stale `--passWithNoTests` paragraph is gone; what
  remains describes the suffix split the code actually implements.
- M2 · held — `access.svelte.ts:16-18`, `fleet.svelte.ts:14-17`. Both class doc comments sit
  against their class again; `complete()` carries its own comment above them.
- M3 · held — `Taskfile.yml:104-107`. `task check --dry` lists `check:web` and `test:web` as its
  final steps. `task check` cannot run to completion in this devcontainer because `shellcheck` is
  not installed (pre-existing, unrelated), so the two new steps were also run directly: green.
- L1 · held — `vite.config.ts:40` spreads `defaultExclude`, so Vitest's `**/node_modules/**` and
  `**/.git/**` survive alongside the unit-suffix exclusion. Collection is unchanged: 8 component
  files before and after.
- L2 · held — `Kits.svelte:34` clears `error` on entry like its two siblings.
- L3 · held — both waits are `setTimeout(…, 0)` with a comment saying why zero is sufficient.

Regression check on the code the fixes touched:
- L2's new `error = ''` in `refreshList()` would have re-created H1 inside `Kits.save()`, which
  set `error` *before* calling it. Both were changed together, and `Kits.svelte:54-58` now reports
  after the reload. Covered by a second red-green test, `shows why a save was refused, after the
  list reloads`.
- No other caller of `refreshList()` exists, so nothing else can be cleared by it: `onMount` runs
  it first and the `$effect` that loads a spec runs after.
- Suite grew 33 → 35 component tests; unit stays 7. No test was modified to accommodate a fix.

New findings introduced by the fixes: none.

Verdict: Yes — all seven findings held, the one interaction between them (L2 re-creating H1 in
Kits) was caught and covered, and every gate is green.

## M3 withdrawn (2026-08-29, at the Document waypoint)
M3 claimed `task check` omitted the frontend gates by oversight. It does not: `AGENTS.md:88`
records keeping `check:web` and `test:web` out of `task check` as a deliberate choice, so a Go
change does not pay for a Chromium launch, with CI running both jobs regardless. The finding was
raised and fixed without reading the working-style section that governs it, which is the case
`AGENTS.md:9` names explicitly — say so in prose and wait, don't silently implement the
alternative. The Taskfile change is reverted; nothing was ungated, because CI never stopped
running either job. Stamped `disputed` rather than `fixed`: the fix was applied and then undone
because the finding itself was wrong.
