---
slug: dashboard-navigation
title: Dashboard navigation and mobile layout
created: 2026-08-28
updated: 2026-08-29
waypoint: discuss
status: approved
document: done
---

# Dashboard navigation and mobile layout — spec

**Problem / why:** the dashboard is one page that stacks the fleet, secrets and kits on top of
each other, in a nine-column table that is unusable on a phone. The NAS is reached from a phone
as often as from a desktop, so mobile is a primary target, not a courtesy. Splitting operations
(the fleet) from configuration (secrets, kits) also stops the page growing without bound.

**In scope:**
- `/` is the dashboard: the create form, scope errors, and the fleet.
- `/config/secrets` and `/config/kits` — the two existing components, each on its own route
  under a shared config shell with its own sub-navigation.
- A top navigation bar shared by every route: the `sluss` wordmark, the SSE live/reconnecting
  badge, and links to Dashboard and Configuration.
- A responsive fleet view: table on wide screens, one card per sandbox below the breakpoint.
- The create form collapses behind a "New sandbox" disclosure on narrow screens; open by
  default on wide ones.
- Mobile hardening across all routes: touch targets, no iOS input zoom, safe-area insets,
  no horizontal page scroll.
- Static-hosting support for the new nested URLs, so a bookmarked `/config/kits` loads from
  the Go binary's file server.

**Out of scope:**
- New API endpoints or Go handler logic. `internal/server` keeps the routes it has.
- Any change to what the fleet shows — same fields, same actions, same wording.
- A light theme, a design system, or a CSS framework. The palette stays the current dark one.
- Offline/PWA behaviour, installability, push.
- A third configuration area. `/config` grows later; two sub-routes is what exists today.

**Chosen approach:** a SvelteKit layout hierarchy — a root `+layout.svelte` carrying the header
and global styles, a `/config` layout carrying the sub-navigation — with `Secrets.svelte` and
`Kits.svelte` moved from `+page.svelte` onto their own routes largely unchanged. The fleet table
gains a card rendering at a breakpoint rather than horizontal scroll, because the actions
(open, attach, stop, destroy) are the point of the view and must be thumb-reachable. Navigation
is a plain wrapping top bar rather than a drawer or bottom tabs: two destinations do not earn
the extra chrome or JS state, and it degrades to a phone as-is.

Shared state is the one thing the split forces. Both the header badge and the dashboard need the
SSE connection, and both the create form and `Secrets.svelte` need `/api/config`'s scope list, so
each moves out of `+page.svelte` into a module both layouts and pages can read. Which shape that
module takes is `plan`'s call; the requirement is that navigating between routes does not tear
down and re-establish the SSE stream.

**Constraints:**
- Everything is served by `http.FileServerFS` over `go:embed`ed static files
  (`internal/server/server.go:74`, `web/embed.go`) — there is no SPA fallback handler. Nested
  routes must therefore prerender to `index.html` inside a directory, which means
  `trailingSlash: 'always'`. Adding a Go fallback instead is the rejected alternative: a config
  line beats a handler.
- `prerender = true` and `ssr = false` stay (`src/routes/+layout.ts`). The fleet only exists at
  runtime; nothing renders at build time.
- The proxy prefix (`/s/<scope>/<name>/`) and `/api/` are owned by Go. Dashboard routes must not
  collide with either.
- Svelte 5 runes, TypeScript throughout, `#lib/*` import alias, existing dark palette.
- Component tests run in real Chromium via Vitest browser mode; the split must leave
  `Kits.test.ts` passing and testable at its new location.

**Acceptance criteria:**
- [ ] AC1: `/` renders the create form, scope errors and the fleet, and no secrets or kits UI.
- [ ] AC2: `/config/secrets` and `/config/kits` each render only their own area, under a shared
      config sub-navigation that marks the current one.
- [ ] AC3: The top bar appears on every route, links to Dashboard and Configuration, marks the
      active section, and shows the live/reconnecting badge.
- [ ] AC4: Navigating `/` → `/config/kits` → `/` does not drop the SSE connection: the badge
      stays live throughout and the fleet is populated on return without waiting for a poll.
- [ ] AC5: At a 375px-wide viewport, every route fits with no horizontal page scroll, and each
      sandbox renders as a card whose open/attach/stop/destroy controls are at least 44px tall.
- [ ] AC6: At a desktop width the fleet is still the current table with the same nine columns.
- [ ] AC7: On a narrow viewport the create form is collapsed behind a "New sandbox" control and
      the first sandbox is visible without scrolling past it; on a wide viewport it is open.
- [ ] AC8: A cold `GET /config/kits` against the built binary's file server returns the page —
      not a 404 — as does `GET /config/kits/`.
- [ ] AC9: `npm run check`, `npm run test:unit`, `npm run test:component` and `go test ./...`
      all pass.

**Edge cases:**
- `/config` with no sub-path is a reachable URL (typed, bookmarked); it must land somewhere real
  rather than 404.
- The kits editor is a full-width `<textarea>` — on a phone it must not force horizontal page
  scroll or shrink to unusable height.
- `access.repos` / `access.scopes` can be empty, so the create form and the secrets scope select
  can both have nothing to choose; the collapsed form must not hide that state silently.
- SSE reconnects while the user is on a config route: the badge must reflect it there too.
- Long branch names and repo paths must wrap or truncate inside a card rather than widen it.
- iOS Safari's dynamic chrome and the home indicator — bottom-most controls need safe-area
  padding.

**Affected areas (current code):**
- `web/dashboard/src/routes/+page.svelte` — loses secrets and kits; keeps fleet plus create
  form; gains the card rendering and the disclosure.
- `web/dashboard/src/routes/+layout.ts` — gains `trailingSlash`.
- `web/dashboard/src/routes/` — new root `+layout.svelte`, new `config/` subtree.
- `web/dashboard/src/lib/Secrets.svelte`, `Kits.svelte` — re-homed, styles adjusted for mobile;
  logic unchanged.
- `web/dashboard/src/lib/Kits.test.ts` — follows whatever `Kits.svelte` becomes.
- `web/dashboard/src/lib/api.ts` — unchanged; the API contract is untouched.
- `web/dashboard/src/app.html` — already has the viewport meta; may gain `color-scheme`.
- `internal/server/server_test.go` — a test for AC8 against the static file server.
- `docs/` — the README's dashboard section describes one page.

**Open questions:**
- None
