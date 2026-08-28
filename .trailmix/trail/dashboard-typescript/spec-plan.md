---
slug: dashboard-typescript
title: Dashboard frontend migrated to TypeScript
created: 2026-08-28
updated: 2026-08-28
waypoint: spec-plan
status: approved
document: done
tasks: T1:done T2:done T3:done
---

# Dashboard frontend migrated to TypeScript

Frontend only: `web/dashboard/`. No Go code changes — the API is the contract being
mirrored, not modified.

## Problem

The dashboard is plain JS while both sibling frontends are TypeScript: babytabs
`frontend/src` is 63 `.ts` + 60 `.svelte` with zero `.js`, homehub is 36 `.ts` + 25
`.svelte`. No decision record chose JS here — `docs/DECISIONS.md` has no ADR on it, and
neither the `gui-control-plane` spec nor its plan mentions the language. It is scaffold
default, never revisited.

`web/dashboard/jsconfig.json` is visibly a JS-ified copy of babytabs'
`frontend/tsconfig.json` — same `extends: "$app/tsconfig"`, same comments about
`$app/tsconfig` shipping no include/exclude, same `#lib` imports-map note. It diverged in
four places: the filename, `vite.config.js` vs `.ts` in `include`, a missing
`src/**/*.ts`, and a `checkJs: false` block babytabs has no need for.

The code that pays for this is the API payload handling. `snapshot` comes straight out of
`JSON.parse` (`src/routes/+page.svelte:31`), and `connectTo(sandbox)` reaches into
`.agent`, `.status`, `.webPort`, `.scope`, `.name` (`:39-52`) with nothing checking those
names against what Go actually sends. Growing to several pages means these shapes start
crossing route boundaries.

## Decision

Migrate to TypeScript now, restoring the sibling convention rather than inventing one.

Verified against this repo's actual prerelease stack before committing to it:

- `lang="ts"` in a `.svelte` file compiles with **zero** config change — a probe component
  with typed locals built clean through `npm run build`.
- svelte-check already type-checks such a file under the existing `check` script: a
  deliberate error returned `ERROR "src/lib/__Probe.svelte" 4:6 "Type 'number | null' is
  not assignable to type 'string'."`

So this is not a big-bang: each file gains full checking the moment it converts, and
`npm run check` stays green throughout. `typescript@^6.0.3` is already a devDependency —
SvelteKit 3 requires it as a hard peer (D17) — so no new dependency.

Rejected: the `checkJs: true` + JSDoc path the current `jsconfig.json` comment prescribes.
Same checking, more verbose, and it would establish a *third* convention across the three
frontends.

## Plan

**T1 — config.** `jsconfig.json` → `tsconfig.json`: drop the `checkJs` block, add
`src/**/*.ts` to `include`, and add `noUncheckedIndexedAccess` to match babytabs. Rename
`vite.config.js` → `vite.config.ts` and point `package.json`'s `check` script at
`./tsconfig.json`.

**T2 — API types.** New `src/lib/api.ts`, hand-mirroring the Go structs that reach the
browser. Hand-written, not generated: seven small types against a Go API in the same repo
is not worth a codegen step (YAGNI).

| TS type | Go source |
|---|---|
| `Sandbox` | `internal/fleet/fleet.go:29` |
| `RepoGroup` | `internal/fleet/fleet.go:47` |
| `ScopeError` | `internal/fleet/fleet.go:55` |
| `Snapshot` | `internal/fleet/fleet.go:62` |
| `AccessConfig` | `internal/server/server.go:149-153` (anonymous) |
| `Kit`, `GitStatus` | `internal/kits/kits.go:27,33` |
| `ApiError` | `writeError` → `{"error": string}` |
| `ScriptResult` | `internal/script/script.go:37` (`stderr` in the error path) |

**T3 — components.** `lang="ts"` on `Kits.svelte`, `Secrets.svelte`, `+page.svelte`;
`+layout.js` → `+layout.ts`; `Kits.test.js` → `.test.ts`. Annotate against `api.ts`. Widen
the vitest `include` globs in `vite.config.ts` from `*.test.js` to `*.test.{js,ts}`.

## Gates

`npm run check` reports 0 errors 0 warnings; `npm run test:component` green;
`npm run build` succeeds; `task` build of the Go binary still embeds the assets.

## Out of scope

eslint + prettier. Both siblings have them and sluss has neither — the same
"diverged from the sibling convention" family, but a separate change.
