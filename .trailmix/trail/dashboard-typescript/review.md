---
slug: dashboard-typescript
title: Dashboard frontend migrated to TypeScript
created: 2026-08-28
updated: 2026-08-28
waypoint: review
status: approved
verdict: pass
---

# Review — dashboard-typescript

Self-review (inline, no reviewer subagent dispatched).

## Gates

| Gate | Result |
|---|---|
| `npm run check` | 245 files, 0 errors, 0 warnings |
| `npm run test:component` | 1 file, 1 test passed |
| `npm run test:unit` | no test files (`--passWithNoTests`, unchanged) |
| `npm run build` | ✔ done |
| `task build` | `dist/slussd` links, dashboard embedded |
| `task check` | Go vet + all Go tests pass; **shellcheck not installed in this container** — pre-existing environment gap, no shell script was touched |

Type enforcement was confirmed live rather than assumed: a deliberate
`let bogus: string = snapshot?.repos[0]?.sandboxes[0]?.unmerged ?? 0;` produced
`ERROR "src/routes/+page.svelte" 16:6 "Type 'number' is not assignable to type 'string'."`,
proving the chain resolves through `api.ts` end to end. Reverted.

## Findings

**MED — import extension diverged from the sibling convention (fixed during review).**
The first pass wrote `import type { … } from '#lib/api.ts'`. It resolved and built, but
babytabs writes `#lib/i18n/state.svelte.js` — the `.js` extension, TS-resolved — off an
identical `"#lib/*": "./src/lib/*"` imports map. Since matching the sibling convention *is*
this change's rationale, inventing a third style in the one new import would have undercut
it. Changed to `#lib/api.js` in all three components.

**LOW — props typed inline rather than as `Props` (fixed during review).**
`Secrets.svelte` first had `let { scopes = [] }: { scopes?: string[] } = $props()`.
babytabs declares an `interface Props { … }` above the destructure. Matched.

**Accepted, not fixed — `KitList & ApiError` overstates the error path.** An intersection
makes `kits`/`names` required, though an error body carries neither. Every call site
already guards with `?? []` and returns early on `!response.ok`, so the runtime behavior is
correct and unchanged; making the payload halves `Partial` would add noise for no caught
bug.

## Verdict

Pass. Mechanical migration, no behavior change — every edit is an annotation, a rename, or
the one `list[0]` restructure that `noUncheckedIndexedAccess` requires.

## Follow-ups (not in this change)

- `docs/DECISIONS.md` has no ADR for the frontend language. This trail is the record; a
  short ADR would put it where the other decisions live.
- eslint + prettier: both siblings have them, sluss has neither.
