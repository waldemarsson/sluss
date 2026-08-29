---
slug: dashboard-navigation
waypoint: plan
status: approved
updated: 2026-08-28
tasks: T1:done T2:done T3:done T4:done T5:done T6:done T7:done
---

# Plan: dashboard navigation and mobile layout

**Goal:** split the single dashboard page into a fleet homepage and a `/config` subtree behind a
shared top bar, and make every route work on a phone.

**Architecture:** a SvelteKit layout hierarchy replaces the one-page dashboard. A root
`+layout.svelte` owns the header, the navigation and the global stylesheet; a `config/`
subtree owns its own sub-navigation and hosts the existing `Secrets.svelte` and `Kits.svelte`
unchanged. The SSE stream and the `/api/config` fetch move out of `+page.svelte` into two
singleton rune modules under `src/lib`, so the header badge and both routes read one copy and
navigation never restarts the stream. The fleet renders as a table or as cards depending on a
`matchMedia` breakpoint — one rendering in the DOM at a time, not two hidden by CSS.

## Global constraints
Copied from `spec.md`; every task inherits these.
- Everything is served by `http.FileServerFS` over `go:embed`ed static files
  (`internal/server/server.go:74`, `web/embed.go`) — there is no SPA fallback handler. Nested
  routes must prerender to `index.html` inside a directory, which means `trailingSlash: 'always'`.
- `prerender = true` and `ssr = false` stay. The fleet only exists at runtime.
- `/api/` and the proxy prefix `/s/<scope>/<name>/` are owned by Go; dashboard routes must not
  collide with either.
- Svelte 5 runes, TypeScript throughout, `#lib/*` import alias, existing dark palette.
- Component tests run in real Chromium via Vitest browser mode.
- No new API endpoints, no change to what the fleet shows, no light theme, no CSS framework.

**Breakpoint:** `48rem`. One value, used by both the CSS media queries and the `matchMedia`
query, so the card/table switch and the responsive styles never disagree.

## File structure
- `web/dashboard/src/lib/fleet.svelte.ts` — Create — the SSE snapshot and connection flag as a
  session-lifetime singleton.
- `web/dashboard/src/lib/access.svelte.ts` — Create — `GET /api/config` fetched once, held for
  every route.
- `web/dashboard/src/app.css` — Create — body, shared element styles (button, input, select,
  textarea, a) and the shared `.muted` / `.warn` / `.error` classes, plus the mobile rules.
- `web/dashboard/src/routes/+layout.svelte` — Create — header (wordmark, badge, nav), imports
  `app.css`, renders children.
- `web/dashboard/src/routes/+layout.ts` — Modify — add `trailingSlash`.
- `web/dashboard/src/routes/+page.svelte` — Modify — fleet and create form only; gains the card
  rendering and the disclosure; loses secrets, kits and the global styles.
- `web/dashboard/src/routes/config/+page.ts` — Create — redirect `/config/` to
  `/config/secrets/`.
- `web/dashboard/src/routes/config/+layout.svelte` — Create — the Secrets | Kits sub-navigation.
- `web/dashboard/src/routes/config/secrets/+page.svelte` — Create — renders `Secrets.svelte`
  with the scopes from `access`.
- `web/dashboard/src/routes/config/kits/+page.svelte` — Create — renders `Kits.svelte`.
- `web/dashboard/src/lib/Secrets.svelte` — Modify — styles only (mobile widths, wrapping).
- `web/dashboard/src/lib/Kits.svelte` — Modify — styles only (textarea sizing on narrow).
- `web/dashboard/src/lib/Kits.test.ts` — unchanged; it mounts the component, which has not moved.
- `web/dashboard/src/app.html` — Modify — `color-scheme: dark`.
- `internal/server/server_test.go` — Modify — nested-directory static serving cases.
- `web/embed_test.go` — Modify — assert the real build emits the nested `index.html`.

## Public contracts (keep stable across tasks)
Both are singletons: one instance per browser session, imported by whichever route needs them.
Runes cannot be exported as bare reassignable bindings, so each exposes readable properties on a
single exported object.

- `#lib/fleet.svelte.js` — `fleet.snapshot: Snapshot | null` — the latest `fleet` SSE event body,
  `null` until the first one arrives.
- `#lib/fleet.svelte.js` — `fleet.connected: boolean` — true between a delivered event and the
  next `EventSource` error; what the header badge reads.
- `#lib/fleet.svelte.js` — `fleet.connect(): void` — idempotent; opens the `EventSource` on first
  call and does nothing on later ones. Called from the root layout, never torn down.
- `#lib/access.svelte.js` — `access.config: AccessConfig` — the `GET /api/config` body, starting
  as the same empty default `+page.svelte` uses today so consumers never see `undefined`.
- `#lib/access.svelte.js` — `access.load(): void` — idempotent; fetches once per session and
  swallows failure, matching today's `.catch(() => {})`.

Types come from `#lib/api.js` unchanged.

## Tasks

### T1: shared fleet and access state
**Files:** Create — `src/lib/fleet.svelte.ts`, `src/lib/access.svelte.ts`
**Deliverable:** both singletons exist and behave as the contract above, with the SSE and
`/api/config` logic lifted verbatim from `+page.svelte`.
**Contract:** as in *Public contracts*.
**Behaviors to cover with tests:** a delivered `fleet` event populates `snapshot` and sets
`connected`; an `EventSource` error clears `connected` without clearing `snapshot`; a second
`connect()` opens no second stream; `access.load()` twice issues one fetch; a failed
`access.load()` leaves the default config and throws nothing.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T2: root layout, navigation and the global stylesheet
**Files:** Create — `src/routes/+layout.svelte`, `src/app.css`; Modify — `src/routes/+layout.ts`,
`src/app.html`
**Deliverable:** every route renders inside a header carrying the `sluss` wordmark, the
live/reconnecting badge fed by `fleet.connected`, and links to Dashboard (`/`) and Configuration
(`/config/secrets/`). The layout calls `fleet.connect()` and `access.load()` on mount. The active
section is marked from `page.url.pathname` (`$app/state`) — a path under `/config` marks
Configuration. `trailingSlash = 'always'` is exported from `+layout.ts` beside the existing
`prerender` and `ssr`. The global styles that were scoped inside `+page.svelte` move to
`app.css`, which also fixes `.muted` / `.error` silently doing nothing inside `Secrets.svelte`
and `Kits.svelte` today.
**Behaviors to cover with tests:** the badge reads live when `fleet.connected` is set and
reconnecting when it is not; the nav marks Configuration active for a `/config/...` path and
Dashboard active for `/`.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T3: the config subtree
**Files:** Create — `src/routes/config/+page.ts`, `src/routes/config/+layout.svelte`,
`src/routes/config/secrets/+page.svelte`, `src/routes/config/kits/+page.svelte`; Modify —
`src/routes/+page.svelte`
**Deliverable:** `/config/secrets/` and `/config/kits/` each render only their own component
under a sub-navigation that marks the current one; `/config/` redirects to `/config/secrets/`;
`<Secrets>` and `<Kits>` are gone from `+page.svelte`. The secrets page passes
`access.config.scopes` as the existing `scopes` prop — `Secrets.svelte`'s own logic is untouched.
**Behaviors to cover with tests:** the secrets route renders the secrets form and no kits
editor, and the kits route the reverse; the sub-nav marks the route it is on; the dashboard
route renders neither.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T4: responsive fleet
**Files:** Modify — `src/routes/+page.svelte`
**Deliverable:** below `48rem` each sandbox renders as a card — name and status on a header
line, repo/branch/agent and the work flags as secondary lines, and open / attach / stop /
destroy as full-width controls — and at or above it the current nine-column table is unchanged.
The switch is a `matchMedia('(min-width: 48rem)')` result held in `$state`, so exactly one
rendering is in the DOM. Long branch names and repo paths wrap inside a card rather than widen
it. Every row's data and actions are reachable in both renderings.
**Behaviors to cover with tests:** at a 375px-wide viewport a sandbox's name, branch, agent,
status and work flags are all visible and no `<table>` is rendered; the stop control is absent
for a stopped sandbox and present for a running one, in both renderings; at a wide viewport the
table renders with its nine headers; resizing across the breakpoint swaps the rendering.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T5: create-form disclosure
**Files:** Modify — `src/routes/+page.svelte`
**Deliverable:** below the breakpoint the create form is collapsed behind a "New sandbox"
control and opens on activation; at or above it the form is open and the control is not shown.
An empty `access.config.repos` or `scopes` stays visible rather than being hidden inside a
collapsed form — the disclosure must not be the only place that state appears.
**Behaviors to cover with tests:** at a narrow viewport the form controls are hidden until
"New sandbox" is activated and visible after; at a wide viewport they are visible with no
disclosure control; a successful create still clears busy state and the form still submits from
the collapsed-then-opened state.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T6: mobile hardening
**Files:** Modify — `src/app.css`, `src/lib/Secrets.svelte`, `src/lib/Kits.svelte`
**Deliverable:** below the breakpoint every button, link-button, input, select and option target
is at least 44px tall; form controls are at least 16px to stop iOS zooming on focus; body padding
respects `env(safe-area-inset-*)`; no route scrolls horizontally at 375px, including the kits
`<textarea>`, which stays usable in height; the secrets scope select and name/value inputs stack
full-width instead of overflowing.
**Behaviors to cover with tests:** at 375px, `document.documentElement.scrollWidth` does not
exceed the viewport width on the dashboard, the secrets route and the kits route; the kits
textarea and the primary controls meet the 44px minimum.
**Gate:** `npm run check` clean, `npm run test:component` green.

### T7: static serving of nested routes
**Files:** Modify — `internal/server/server_test.go`, `web/embed_test.go`
**Deliverable:** the Go side is proven to serve the new URL shape. `server_test.go`'s `assets`
fixture gains a `config/kits/index.html` entry and `TestStaticAssets` gains cases for it;
`embed_test.go` gains a test — skipped when no dashboard has been built, like
`TestBuiltDashboardHasAnIndex` — asserting the real build output contains
`config/kits/index.html`, which is what actually proves `trailingSlash: 'always'` took effect.
No handler changes.
**Behaviors to cover with tests:** `GET /config/kits/` returns the nested index; `GET
/config/kits` redirects to the trailing-slash form rather than 404ing; a genuinely missing
nested path still 404s.
**Gate:** `go test ./...` green, and `task web && go test ./web/` with the skip not taken.

## Tests
- AC1 → T3 (dashboard renders neither secrets nor kits) + T4/T5 (what it does render)
- AC2 → T3 (each route renders only its own area; sub-nav marks the current one)
- AC3 → T2 (badge state, active-section marking)
- AC4 → T1 (idempotent `connect()`, `snapshot` survives) + T2 (layout owns the call)
- AC5 → T4 (card rendering, no `<table>`) + T6 (no horizontal scroll, 44px targets)
- AC6 → T4 (nine headers at a wide viewport)
- AC7 → T5 (disclosure hidden/shown by viewport)
- AC8 → T7 (both URL forms served; build output contains the nested index)
- AC9 → every task's gate, run once more at the end

## Risks / trade-offs
- **`trailingSlash` is the load-bearing assumption.** If SvelteKit 3's adapter-static does not
  emit `config/secrets/index.html` under it, nested routes 404 from the binary and the fallback
  is a Go handler that serves `index.html` for unmatched non-`/api/`, non-`/s/` paths. T7's
  `embed_test.go` case is what surfaces this, so run it against a real `task web` before
  building on top of T3.
- **The `#lib/*.svelte.js` specifier is unverified.** `#lib/api.js` resolving to `api.ts` works
  today; `#lib/fleet.svelte.js` resolving to `fleet.svelte.ts` through the same `imports` map
  should, but if the Svelte plugin does not see the runes file, fall back to a relative import
  from the layout rather than renaming the module.
- **Singleton state leaks across component tests.** `fleet` and `access` hold session state, so
  a test that mounts a route after another has connected sees the earlier state. Keep tests
  stubbing `fetch` and `EventSource` per test; add a reset only if a test actually needs one.
- **`matchMedia` over CSS-only.** Duplicating markup and hiding one copy would make every
  `getByText` ambiguous in tests and ship dead DOM; the standard `display: block` responsive-table
  trick cannot produce the agreed card shape. The cost is a small amount of JS in the render path
  and a resize listener.
- **Two renderings of the same row is duplicated markup within one file.** Accepted: the card
  and the table are genuinely different layouts, and factoring a shared row component would take
  more props than it saves lines.

## Open questions
- None

## Amendments
- 2026-08-28 — the header and both navigations are `src/lib/Nav.svelte` and
  `src/lib/NavLinks.svelte` rather than markup inside the two layouts. A `+layout.svelte` cannot
  be rendered on its own by a component test (it needs a `children` snippet and the `$app/state`
  runtime), so T2's and T3's active-marking behaviors were untestable as planned. Props in,
  markup out; the layouts stay shells that wire them to `page` and `fleet`. Same contracts, one
  extra file. Implementer's call, no scope change.
- 2026-08-28 — `fleet.svelte.ts` and `access.svelte.ts` export their classes (`Fleet`, `Access`)
  beside the singletons. Session-lifetime singletons cannot be tested for "connect/fetch once"
  and "survive a failure" in the same file, which is the leakage risk the plan's Risks section
  flagged; a test-only `reset()` was the alternative. Additive to the contract.
