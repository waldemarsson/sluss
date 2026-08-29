---
slug: frontend-tidy-up
title: Dashboard frontend tidy-up: formatter gate, snippet de-duplication, boundary normalisation
created: 2026-08-29
updated: 2026-08-29
waypoint: spec-plan
status: approved
document: done
tasks: T1:done T2:done T3:done T4:done T5:done T6:done T7:done
---

# Dashboard frontend tidy-up — spec + plan (merged)

Eight cleanup items found by reading `web/dashboard/src` end to end. Scope was agreed item by
item in conversation; what this artifact settles is sequencing, contracts and gates. No Go code
is touched.

## Spec

**Problem / why:** the dashboard has no formatter or linter while the Go side gates `gofmt` in
CI, the largest component writes the same five sandbox controls twice, three components carry
drifted copies of one error-extraction rule, and a scope switch can render another scope's
secrets.

**In scope / out of scope:**
- in: the eight items below, all inside `web/dashboard/` plus `Taskfile.yml` and
  `.github/workflows/ci.yml` for the new gate.
- out: ESLint (declined — `svelte-check` plus Prettier is judged enough at ~1100 lines); any
  change to the API surface, to `internal/server`, or to what the dashboard displays; a light
  theme; new runtime dependencies.

**Chosen approach:** two pull requests. PR 1 is the formatter alone — a mechanical whole-tree
reformat that reviews by skimming. PR 2 is the seven code items, landing on already-formatted
files so every diff is logic. De-duplication uses Svelte 5 `{#snippet}` in place rather than
extracted components, because a snippet renders only where it is called, so the existing
"one of everything in the DOM" invariant survives without prop-drilling `busy`, `forcing` and
three handlers into a child.

**Constraints:** inherited by every task.
- Svelte 5 runes, TypeScript, the `#lib/*` alias, the existing dark palette, no CSS framework.
- `prerender = true` and `ssr = false` stay; the build must keep producing `web/dashboard/build/`
  for `go:embed` (`web/embed.go`).
- Formatter config must match what is already on disk, so PR 1 stays small: tabs, single quotes,
  `printWidth: 100`. Verified against the current source — 21 tab-indented files, 50 single-quoted
  imports and zero double-quoted, and only 8 lines wider than 100 columns.
- Prettier and `prettier-plugin-svelte` are `devDependencies` and need a justification line in the
  PR description (AGENTS.md, "Standard library first / justify every new dependency").
- `MediaQuery` requires Svelte ≥ 5.7.0; 5.56.10 is installed.
- The 15 existing tests in `src/routes/+page.test.ts` must pass **unmodified** through T5. A test
  that needs editing means the refactor changed behavior, which is the failure signal, not a
  chore.
- No behavior changes anywhere except T2 (the bug) and T7 (the wrap). T3–T6 are refactors and
  must be invisible to the suite.

**Acceptance criteria:**
- [ ] AC1: `npx prettier --check .` passes in `web/dashboard`, and CI fails if it does not.
- [ ] AC2: the five sandbox controls are written once in `+page.svelte`; the desktop table still
      has nine columns and the phone still renders cards, with one of the two in the DOM.
- [ ] AC3: switching scope in Secrets, or kit in Kits, never renders a response belonging to a
      previous selection.
- [ ] AC4: one error-extraction rule serves all five call sites, and it prefers the script's
      `stderr` over the generic message everywhere — not only on the dashboard route.
- [ ] AC5: `access.config` and `fleet.snapshot` are normalised at the boundary; no consumer
      carries `?? []` or `?.length ?? 0` against them.
- [ ] AC6: the breakpoint's four sites are accurate in comments; `+page.svelte` holds no manual
      `matchMedia` listener.
- [ ] AC7: the force checkbox occupies the same position on every sandbox card at 390px.
- [ ] AC8: `test:unit` runs at least one real test and no longer needs `--passWithNoTests`.

**Edge cases:**
- Prettier reformatting the prose-heavy comments these files depend on. Prettier preserves
  comment text and does not reflow it, so the risk is confined to markup re-wrapping.
- A scope or kit whose fetch rejects rather than resolving non-OK — the ordering guard must not
  leave `busy`/`error` stuck.
- `Access.load()` swallows failure by design; normalisation must not turn a failed fetch into a
  populated-looking config.
- A kit list that comes back empty after a save, which would strand `selected`.

## Plan

**Architecture:** unchanged. This is a refactor plus one bug fix and one CSS fix; no new module
boundaries beyond a request helper joining the existing `lib/api.ts`, which today is types only.

### File structure
- `web/dashboard/.prettierrc` — Create — formatter config: tabs, single quotes, `printWidth: 100`,
  `prettier-plugin-svelte`.
- `web/dashboard/.prettierignore` — Create — `build/`, `.svelte-kit/`, `node_modules/`, coverage
  and vitest attachment dirs.
- `web/dashboard/package.json` — Modify — `prettier` + `prettier-plugin-svelte` devDependencies,
  `format` and `format:check` scripts; drop `--passWithNoTests` from `test:unit` in T4.
- `.github/workflows/ci.yml` — Modify — a format check step in the `frontend` job, before the
  type check.
- `Taskfile.yml` — Modify — `fmt:web` and a `format:check` call inside `check:web`, mirroring the
  Go side's `fmt` / `fmt:check` pair.
- `web/dashboard/src/lib/api.ts` — Modify — add the request helper and its error-message rule to
  the existing type declarations.
- `web/dashboard/src/lib/api.test.ts` — Create — unit tests for the helper; the first test in the
  `unit` project.
- `web/dashboard/src/lib/access.svelte.ts` — Modify — normalise the config at the boundary.
- `web/dashboard/src/lib/fleet.svelte.ts` — Modify — normalise the snapshot at the boundary.
- `web/dashboard/src/lib/Secrets.svelte` — Modify — ordering guard, then the shared helper.
- `web/dashboard/src/lib/Kits.svelte` — Modify — ordering guard, then the shared helper.
- `web/dashboard/src/routes/+page.svelte` — Modify — snippets, `MediaQuery`, the shared helper,
  dropped guards, card wrap.
- `web/dashboard/src/routes/config/secrets/+page.svelte` — Modify — drop the `?? []`.

### Public contracts
Only what later tasks depend on.

- `lib/api.ts` exports a request helper taking a URL and optional `RequestInit`, returning a
  discriminated result carrying either the parsed body or a single ready-to-display message.
  The message rule, in order: the script's `stderr`, then `error`, then `failed with <status>`,
  trimmed. A thrown fetch becomes a message too, so no caller writes its own `try`/`catch`.
- `Access.config` and `Fleet.snapshot` expose fully-populated shapes — arrays are always arrays.
  `AccessConfig` and `Snapshot` stop being optimistic about the wire and the normalisation, not
  the consumers, owns the defaults.
- `+page.svelte` exposes two snippets internally, one for the work flags and one for the
  lifecycle controls, each taking a `Sandbox`. Not a cross-file contract — named here because T6
  and T7 render through them.

### Tasks

#### T1: formatter and its gate
**Files:** Create — `.prettierrc`, `.prettierignore`; Modify — `package.json`,
`.github/workflows/ci.yml`, `Taskfile.yml`; reformat all of `web/dashboard/src`.
**Deliverable:** PR 1, standalone. Config lands, the tree is reformatted once, and CI fails on
unformatted code.
**Behaviors to cover with tests:** no new tests — the gate is the check itself. Confirm the
existing suite passes untouched after the reformat, which is what proves it was mechanical.
**Gate:** `npx prettier --check .`, `npm run check`, `npm run test:unit`, `npm run test:component`,
`npm run build` — all green, and `git diff --stat` shows only formatting.

#### T2: scope and kit response ordering (the bug)
**Files:** Modify — `src/lib/Secrets.svelte`, `src/lib/Kits.svelte`.
**Deliverable:** a response that resolves after the selection has moved on is discarded. Red test
first: this is the only defect in the set, so a failing test that reproduces it precedes the fix.
**Behaviors to cover with tests:** a slow response for scope A resolving after a switch to scope B
leaves B's names on screen; the same for a kit's spec; a rejected fetch still clears `busy` and
reports; the fast-path ordering (responses arriving in order) is unchanged.
**Gate:** `npm run test:component` — the new tests fail before the fix and pass after; every
existing test still passes.

#### T3: normalise at the boundary
**Files:** Modify — `src/lib/access.svelte.ts`, `src/lib/fleet.svelte.ts`,
`src/routes/+page.svelte`, `src/routes/config/secrets/+page.svelte`.
**Deliverable:** the eight guard sites disappear; a config or snapshot missing keys still yields
usable state.
**Behaviors to cover with tests:** a config body with keys missing entirely still renders the
"no repositories configured" state rather than throwing; a failed config fetch still sets
`loaded` and reports nothing configured; a snapshot without `scopeErrors` renders the fleet.
**Gate:** `npm run check` (no new optional-chaining needed) and `npm run test:component`.

#### T4: one request helper
**Files:** Modify — `src/lib/api.ts`, `src/lib/Secrets.svelte`, `src/lib/Kits.svelte`,
`src/routes/+page.svelte`, `package.json`; Create — `src/lib/api.test.ts`.
**Deliverable:** the five call sites share one rule, and secrets and kits now surface a refusal's
`stderr` instead of dropping it. `test:unit` loses `--passWithNoTests`.
**Behaviors to cover with tests:** `stderr` wins over `error`; `error` wins over the status
fallback; a body that is not JSON falls back to the status message; whitespace is trimmed; a
thrown fetch becomes a message rather than an exception. These are the helper's own unit tests;
the component tests covering the existing refusal display must still pass.
**Gate:** `npm run test:unit` (now meaningful), `npm run test:component`, `npm run check`.

#### T5: de-duplicate the sandbox row
**Files:** Modify — `src/routes/+page.svelte`.
**Deliverable:** work flags and lifecycle controls declared once as snippets, rendered from both
the table and the card branch. Roughly 140 duplicated lines become one declaration plus two call
sites.
**Behaviors to cover with tests:** none added. The 15 existing tests in `+page.test.ts` already
cover the nine-column desktop table, the phone cards, start/stop by status, all five force and
destroy behaviors, and the stderr display. They must pass **unmodified** — that is the gate.
**Gate:** `npm run test:component` with zero edits to `+page.test.ts`, plus `npm run check`.

#### T6: MediaQuery, and the breakpoint comment
**Files:** Modify — `src/routes/+page.svelte`.
**Deliverable:** `new MediaQuery('min-width: 48rem')` from `svelte/reactivity` replaces the
`$state` seed, the `onMount` listener and its teardown. The comment claiming the constant is
"shared with the media queries below" is corrected: it is restated at four sites
(`+page.svelte`, `app.css:81`, `Secrets.svelte:133`, `Kits.svelte:121`), which the comment should
name so the next change finds them.
**Behaviors to cover with tests:** the existing viewport tests already assert the table above the
breakpoint and cards below, including a resize. They must keep passing; the eager first read must
still hold, so a desktop render never flashes the card layout.
**Gate:** `npm run test:component`, `npm run check`.

#### T7: the force checkbox lands in one place
**Files:** Modify — `src/routes/+page.svelte`.
**Deliverable:** at 390px the arming checkbox occupies the same position on every card regardless
of how many controls the card carries — today it sits beside `destroy` on cards without a connect
link and a row above it on cards with one.
**Behaviors to cover with tests:** at phone width, the checkbox's offset within its card is
identical across a sandbox that has a connect link and one that does not; every control keeps its
44px minimum; the page still does not scroll sideways.
**Gate:** `npm run test:component`.

### Tests
- AC1 → T1 / the `prettier --check` gate in CI and `check:web`.
- AC2 → T5 / the unmodified nine-column and card tests.
- AC3 → T2 / out-of-order resolution for both scope and kit.
- AC4 → T4 / `stderr` precedence unit tests plus the existing refusal display test.
- AC5 → T3 / missing-key config and snapshot rendering.
- AC6 → T6 / existing viewport tests, plus reading the corrected comment.
- AC7 → T7 / checkbox offset equality across card shapes.
- AC8 → T4 / `test:unit` running `api.test.ts` without the flag.

### Risks and trade-offs
- **Sibling-repo conventions unverified.** The dashboard's Vitest setup was copied from homehub
  and babytabs, and the standing preference is to match their tooling patterns. Neither repo is
  on this machine, so the Prettier config here was derived from sluss's own source instead. If
  those repos already carry a `.prettierrc`, prefer theirs and re-run T1's reformat.
- **PR 1 rewrites every file.** Mitigated by matching the config to what is on disk, but it will
  still collide with any other in-flight dashboard branch. Land it when nothing else is open.
- **T5 is the highest-risk change** — it edits the markup around an irreversible destroy control.
  The mitigation is the no-test-edits rule: the force and destroy behaviors are already covered by
  seven tests, so a behavior change cannot pass quietly.
- **T2 before T4** is deliberate. The ordering guard lands in the same functions the helper later
  rewrites; fixing the defect first keeps the red-green evidence readable, and T4 must then
  preserve the guard.
- Dropping `--passWithNoTests` (T4) means a broken `unit` glob starts failing loudly, which is the
  point, but it is a new way for CI to go red.

## Open questions
- None. ESLint, PR batching and the de-duplication shape were all settled before planning.

## Amendments
- 2026-08-29 — AC5 narrowed during T3, self-approved as non-blocking. `fleet.snapshot` stays
  `Snapshot | null`, so `+page.svelte` keeps `fleet.snapshot?.repos ?? []` and
  `?.scopeErrors ?? []`. Those two guards cover "no poll has arrived yet", which the dashboard
  renders as its own state ("Waiting for the first poll…") — they are not guarding against
  missing keys, which is what normalisation removes. The six `access.config` guards are gone as
  planned. Removing the last two would mean dropping the nullable snapshot for a second flag,
  which buys nothing.
- 2026-08-29 — T4 file renamed, self-approved as non-blocking. The plan named
  `src/lib/api.test.ts`, but the `unit` project excluded `src/lib/**` wholesale, so that path
  could only ever run in the browser project. `src/lib` now holds both a framework-free module
  and browser-bound ones, so the split moved from directory to suffix: `*.unit.test.ts` is the
  unit project, everything else is the component project. The test file is
  `src/lib/api.unit.test.ts`. AC8 is met — `test:unit` runs 7 real tests without the flag.
- 2026-08-29 — T1 config corrected after the fact. The plan's first risk said the Prettier config
  was derived from sluss's own source because homehub and babytabs were unavailable. They were
  never unavailable: both are private GitHub repos readable with `gh api`, which the standing
  convention note already records — the check was made against the filesystem alone and wrongly
  concluded otherwise. Both repos do carry `frontend/.prettierrc.json`, they agree byte for byte,
  and they differ from the derived config in one setting: `trailingComma: "none"` against
  Prettier 3's default of `"all"`. sluss now uses their file verbatim, under their filename
  (`.prettierrc.json`), and `.prettierignore` follows babytabs' shape. The risk is closed, not by
  argument but by adopting the house standard. It also shrank the reformat from 19 files to 12,
  since sluss was already written without trailing commas.
- 2026-08-29 — M3 reverted at the Document waypoint. Adding `check:web` and `test:web` to
  `task check` contradicted a documented deliberate decision (`AGENTS.md:88`). See the withdrawal
  note in review.md. Two of this trail's three process errors have the same root: reviewing and
  planning against the code without first reading `AGENTS.md` and `docs/DECISIONS.md`, which
  `AGENTS.md` names under "Read first" — `docs/DECISIONS.md:320` already recorded that both
  sibling repos had Prettier and ESLint and sluss had neither, which would have settled the
  config question before it was ever raised as a risk.
