---
slug: dashboard-navigation
waypoint: review
status: approved
updated: 2026-08-29
findings: H1:fixed M1:fixed M2:fixed M3:fixed L1:fixed L2:fixed L3:open
---

Strengths: the singleton rune modules (`web/dashboard/src/lib/fleet.svelte.ts`, `access.svelte.ts`) are clean and exactly match the plan's contract, with a real `FakeEventSource` test proving idempotent `connect()` and snapshot survival across a drop (`fleet.svelte.test.ts:37-64`); `NavLinks.svelte`'s longest-prefix-match active-link logic is a genuinely elegant solve for the `/` vs `/config/` ambiguity; `internal/server/server_test.go:277-278` proves AC8 with both URL forms through a real redirect-following `http.Get`, and `web/embed_test.go`'s new test asserts against the actual `task web` build output, not a fixture.

HIGH:
- H1 · web/dashboard/src/lib/Kits.svelte:113 (`font-size: 13px`) → Svelte's scoped-style compilation adds a `.svelte-*` class to the selector, giving it higher specificity than the shared `textarea{font-size:16px}` mobile rule in `web/dashboard/src/app.css:84`; confirmed in the actual build output (`build/_app/immutable/assets/5.DsvmVuO8.css`: `textarea.svelte-1cse2ke{...font-size:13px}` beats `textarea{...font-size:16px}` inside the same media query) → the spec.yaml editor keeps its 13px font at every width, so focusing it on an iPhone still triggers Safari's zoom-on-focus — directly contradicting AC5/T6's "form controls at least 16px to stop iOS zooming," and untested (existing tests check height, not font-size) → set the textarea's mobile font-size to 16px inside Kits.svelte's own `@media (max-width: 47.999rem)` block (or drop the explicit `font-size: 13px` so the shared rule can win).

MEDIUM:
- M1 · web/dashboard/src/routes/+page.svelte:192-198,241-247 & web/dashboard/src/app.css:66-68 → the old scoped `+page.svelte` had `.warn { margin-right: 0.5rem }`; the shared `.warn` moved into `app.css` only sets color, confirmed in the built CSS → adjacent "dirty" and "N unmerged" badges (e.g. the exact fixture used in `+page.test.ts`, `dirty: true, unmerged: 2`) render touching with no gap, in both the table and every card → restore the spacing on `.warn` in `app.css`, or add a gap in the row/card markup.
- M2 · web/dashboard/src/routes/+page.svelte:11,31-37 → `wide` defaults to `false` and is only corrected inside `onMount`'s `matchMedia` sync, even though `ssr = false` guarantees `window` is available synchronously when the script runs → the first render on any wide viewport computes the narrow-only DOM (cards, collapsed create form) before the effect corrects it, a flash whose absence is implementation-timing-dependent rather than guaranteed, and AC7 explicitly requires the form open by default on wide → initialize `wide` from `window.matchMedia(WIDE).matches` directly in the script body instead of deferring the first read to `onMount`.
- M3 · web/dashboard/src/routes/+page.svelte:29,114-118 → `configured` is derived purely from `access.config.repos.length`, which starts at `[]` before the `/api/config` fetch resolves, so on every load — even when repos are genuinely configured — the "No repositories configured" message and the absent disclosure can flash for the fetch's duration; no test exercises the loading-then-populated transition (existing test sets the empty config before render) → track a separate "loaded" flag, or accept and document the transient state.

LOW:
- L1 · README.md:96-98 → still describes secrets and kits as part of "the dashboard" with no mention of `/config/secrets`, `/config/kits`, or the new top navigation, though `spec.md`'s Affected areas explicitly calls out "the README's dashboard section describes one page" → update the paragraph to reflect the split.
- L2 · .trailmix/trail/dashboard-navigation/plan.md:208-209 → the plan's `## Amendments` section reads "None," which doesn't match this review's framing of "two recorded deviations"; nothing in the diff looks like an undocumented departure from the plan itself, but the metadata is worth reconciling with the orchestrator before relying on it for future re-reviews.

Spec compliance:
- [x] AC1 — `+page.svelte` renders only the create form and fleet; `Secrets`/`Kits` are gone (verified against diff and `routes/config/routes.test.ts`).
- [x] AC2 — `config/secrets/+page.svelte` and `config/kits/+page.svelte` each render one component; `config/routes.test.ts` proves isolation and sub-nav marking.
- [x] AC3 — `Nav.svelte`/`NavLinks.svelte` cover badge and active-link marking, tested in `Nav.test.ts`/`NavLinks.test.ts`.
- [x] AC4 — `fleet.svelte.ts`'s idempotent `connect()` plus the root layout's single `onMount` call, tested in `fleet.svelte.test.ts`.
- [x] AC5 — card rendering and 44px/16px rules present and tested, but see H1 (font-size regression on the kits textarea specifically).
- [x] AC6 — nine-column table preserved and asserted at desktop width in `+page.test.ts`.
- [x] AC7 — disclosure behavior tested at both widths.
- [x] AC8 — proven against both the fstest fixture and a real `task web` build; verified redirect behavior in `server.go` matches.
- [x] AC9 — `npm run check`, `test:unit`/`test:component`, `go test ./...` all reconfirmed green during this review.

Verdict: With fixes — H1 is a real, unaddressed mobile-zoom bug directly contradicting an acceptance criterion and slipped past every green gate; fix it (and ideally M1/M2) before merging, the architecture and the rest of the split are otherwise sound.

## Re-review (2026-08-29)

Strengths held: all six previously-reported fixes are real, source-level and build-verified where applicable, and the new/updated tests exercise the actual failure mode rather than restating the fix.

- H1 · held — `Kits.svelte:127-132` now sets `font-size: 16px` on its own scoped `textarea` rule inside its `@media (max-width: 47.999rem)` block. Confirmed in built output `web/dashboard/build/_app/immutable/assets/5.Dy4EZi5V.css`: `textarea.svelte-1cse2ke{...font-size:13px}` base rule is overridden by the later, same-specificity `@media(width<=47.999rem){textarea.svelte-1cse2ke{...font-size:16px}}` declaration — holds regardless of asset load order since it's the same selector in the same file. Real test: `web/dashboard/src/routes/config/routes.test.ts:65` asserts `getComputedStyle(editor).fontSize >= 16` at a 375px viewport; would fail on revert.
- M1 · held — `app.css:66-70` restores `.warn { margin-right: 0.5rem }`. Confirmed in built `0.BRe3prUO.css`: `.warn{color:#e0a458;margin-right:.5rem}`. Real test: `+page.test.ts:78-80` asserts `getComputedStyle(...).marginRight !== '0px'` on the "dirty" span; would fail on revert.
- M2 · held — `+page.svelte:11` now reads `wide = $state(window.matchMedia(WIDE).matches)` synchronously in the script body; `onMount` keeps only the `change` listener, the redundant initial `sync()` call is gone, and no functionality is lost (the listener still tracks all future resizes; the removed call was purely redundant with the new initializer). No test added — accepted with a caveat: the implementer's reasoning that `render()` (which awaits a trace-mark promise after `coreRender`) cannot observe the pre-`onMount` DOM is correct for *that* helper, because Svelte 5's `Batch.ensure()` schedules user effects (including `onMount`) via `queueMicrotask`, which fires before `render()`'s own `await mark(...)` resolves — so both the buggy and fixed code converge by assertion time. However, "no test is possible" is too strong: calling Svelte's raw `mount()` directly (bypassing `vitest-browser-svelte`'s `render()`) and asserting on the DOM synchronously immediately afterward, with no `await` in between, would catch a regression, since only the fixed code computes `wide` within the synchronous initial render. Worth a follow-up test, not a blocker — the source fix is correct and verified by reading Svelte's own `mount`/`component_root`/`Batch.ensure` implementation.
- M3 · held — `access.svelte.ts` adds `loaded = $state(false)`, set `true` in `.finally()` regardless of success/failure (covers non-JSON responses too, since `.then(r => r.json())` throwing is caught by `.catch(() => {})` before `.finally` runs); `+page.svelte` gates both the "No repositories configured" message and the create form on `access.loaded`. Tests are real, not tautological: `access.svelte.test.ts` proves `loaded` flips on both success and rejected fetch; `+page.test.ts:159-169` ("claims nothing about the configuration until it has arrived") sets `access.loaded = false` with empty repos and asserts neither the message nor the form/disclosure appear — reverting the gate would make this fail.
- L1 · held — `README.md:93-101` now accurately describes the `/config/secrets` and `/config/kits` split and the card/collapsed-form mobile behavior.
- L2 · held — `plan.md`'s `## Amendments` section now carries two dated 2026-08-28 entries instead of "None," matching the original review's framing.

New finding:
- L3 · web/dashboard/src/lib/access.svelte.ts:24-32 & web/dashboard/src/routes/+page.svelte:118-135 (regression risk introduced by the M3 fix) → gating the create form and the "not configured" message on `access.loaded`, which is only set in `fetch(...).finally()`, means a `/api/config` request that never settles (hangs rather than rejects — no timeout is set) leaves both permanently invisible, forever; pre-fix the form was unconditionally rendered on wide viewports (with empty selects) in that same scenario → cosmetic/edge-case only (the fleet table/cards render independently and are unaffected), but worth a documented trade-off or a fetch timeout if `slussd` hangs are a realistic failure mode.

Gates independently re-run this turn: `npm run test:component` → 7 files/18 tests passed; `npm run check` → 0 errors; `go test -v -run TestBuiltDashboard ./web/...` → both `TestBuiltDashboardNestsConfigRoutes` and `TestBuiltDashboardHasAnIndex` PASS (not skipped); `task check`'s Go vet/test stage passed (its shellcheck stage fails locally only because the `shellcheck` binary isn't on PATH in this environment — unrelated to this diff, no scripts were touched).

Verdict: Ready to proceed — Yes. All five re-checked findings (H1, M1, M2, M3, L1) hold under source and build inspection, L2's metadata is reconciled, and the one new item (L3) is a narrow edge case worth a follow-up note rather than a blocker.
