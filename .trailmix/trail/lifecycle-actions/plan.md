---
slug: lifecycle-actions
waypoint: plan
status: approved
updated: 2026-08-29
tasks: T1:done T2:done T3:done T4:done
---

# Plan: Sandbox lifecycle actions

**Goal:** Give the dashboard a start action and a confirmed, optionally forced destroy.

**Architecture:** Bottom-up through the three layers that already exist. `internal/script` gains a
force parameter; `internal/server` gains a start route and a force query parameter;
`web/dashboard` renders both. `scripts/sluss` is not touched — it already resumes on start and
already accepts `--force` on destroy. A fourth task fixes the repo documentation that currently
forbids what the first three build.

## Global constraints
Copied from `spec.md`. Every task inherits these.
- Lifecycle stays in `scripts/sluss` (D14). The daemon passes a flag; it implements nothing.
- All sbx invocation stays in `internal/sbx` (D3, D10).
- sluss holds no state (D12).
- Mutating routes stay behind the existing cross-site `guard`. No token gate (D15).
- `FlushInterval: -1`, the single listener, and secret write-only-ness are untouched.
- The start route accepts no filesystem path from the client.

## File structure
- `internal/script/script.go` — Modify — `force` on `Destroy`; the package and `Destroy` doc
  comments that assert force is never passed.
- `internal/script/script_test.go` — Modify — argv assertions for `--force`.
- `internal/server/server.go` — Modify — `handleStart`, `?force=` on `handleDestroy`.
- `internal/server/server_test.go` — Modify — route behaviour, 404 and guard paths.
- `web/dashboard/src/routes/+page.svelte` — Modify — start button, confirm, force checkbox; table
  and card layouts both.
- `web/dashboard/src/routes/+page.test.ts` — Modify — component tests for the above.
- `AGENTS.md` — Modify — the "Destroy is destructive" paragraph.
- `docs/DECISIONS.md` — Modify — D20, force from the GUI.

## Public contracts (keep stable across tasks)
- `internal/script` — `Destroy(ctx context.Context, repo, appName, name string, force bool) (Result, error)`.
- `internal/server` — `POST /api/sandboxes/{scope}/{name}/start` — resumes a known sandbox; 404
  when the snapshot does not have it.
- `internal/server` — `DELETE /api/sandboxes/{scope}/{name}?force=true` — absent or any other
  value means unforced.

## Tasks

### T1: force reaches the script
**Files:** Modify — `internal/script/script.go`, `internal/script/script_test.go`
**Deliverable:** `Destroy` takes `force bool` and produces the right argv.
**Contract:** `Destroy(ctx, repo, appName, name string, force bool) (Result, error)`.
**Notes:** the package doc comment and `Destroy`'s doc comment both assert that force is never
passed and frame that as the safety property. Rewrite them to state what is now true — force is
an explicit, confirmed choice the GUI can make, and the script's refusal is still the default
path — rather than leaving comments that contradict the code.
**Behaviors to cover with tests:** `force` true appends `--force` as the final argument; false
appends nothing; the existing Start and Stop argv are unchanged; the working directory and
environment threading are unchanged.
**Gate:** `task fmt:check && task vet && go test ./internal/script/...` green.

### T2: routes for start and force
**Files:** Modify — `internal/server/server.go`, `internal/server/server_test.go`
**Deliverable:** both API changes, behind the existing guard.
**Contract:** as in Public contracts.
**Notes:** `handleStart` reuses `sandboxFor` and `lifecycleContext`, mirroring `handleStop`'s
shape, so the browser names a sandbox that already exists and never a path. It needs
`handleDestroy`'s empty-`Repo` refusal too: without a repo there is no working directory to run
the script in. Parse `force` strictly — only `true` enables it, so a typo cannot arm a
destructive flag.
**Behaviors to cover with tests:** start on a known stopped sandbox runs the script with the
snapshot's repo, scope and name; start on an unknown scope/name is 404 and runs the script not at
all; start on a sandbox with an empty `Repo` is 409 and runs the script not at all; start is
refused cross-site by the guard; a script refusal comes back 409 with its stderr; destroy with no
parameter passes force false; `?force=true` passes true; `?force=1` and `?force=yes` pass false.
**Gate:** `task fmt:check && task vet && task test` green.

### T3: the dashboard
**Files:** Modify — `web/dashboard/src/routes/+page.svelte`,
`web/dashboard/src/routes/+page.test.ts`
**Deliverable:** a start button on stopped sandboxes, and a confirm step plus force checkbox on
destroy — in both the table and the card layout.
**Notes:** `+page.svelte` renders table and card branches separately and the existing tests assert
one of everything in the DOM; every affordance added here belongs in both. The force checkbox is
per row and must reset when a destroy completes or another row is acted on, so arming one row
cannot force a different one. Reuse the existing `act()` wrapper so refusals keep rendering
through `problem`.
**Behaviors to cover with tests:** a stopped sandbox shows start and a running one does not; start
posts to the start route; destroy without confirming sends no request; confirmed destroy with the
box clear sends no `force` parameter; with the box checked sends `force=true`; a refusal renders
the script's stderr unchanged; the checkbox does not stay armed across rows; both layouts carry
every affordance.
**Gate:** `task check:web && task test:web` green (component project needs Chromium in
`~/.cache/ms-playwright`).

### T4: the docs that currently say otherwise
**Files:** Modify — `AGENTS.md`, `docs/DECISIONS.md`
**Deliverable:** the repo's own instructions match the code.
**Notes:** AGENTS.md's "Destroy is destructive" paragraph instructs that slussd never passes
`--force` and that a confirmation prompt is the wrong affordance. Both are now false. Rewrite it
to state the new rule — the unforced refusal is still the default and still shown verbatim, force
is opt-in per destroy and always behind a confirm — and add D20 recording what changed the earlier
judgement: a dirty sandbox was undeletable from the dashboard, which is the failure the phone
deployment actually hits.
**Behaviors to cover with tests:** none; documentation.
**Gate:** `task check` green, as the trail's final gate.

## Tests
- AC1 → T3 start button + T2 start route
- AC2 → T2 unknown-name 404
- AC3 → T2 start uses the snapshot's repo; no path in the request
- AC4 → T3 unforced destroy renders the refusal + T2 force-false argv
- AC5 → T1 `--force` argv + T2 force-true threading
- AC6 → T3 dismissing the confirm sends no request
- AC7 → T3 start not offered when running

## Risks / trade-offs
- **Force is now reachable from a phone.** The confirm step and the per-row checkbox reset are the
  whole mitigation, so both are correctness requirements in T3, not polish.
- **`Destroy`'s signature changes.** One caller today (`handleDestroy`), so the churn is trivial,
  but the boolean is positional and easy to misread at the call site — the test asserting
  false-appends-nothing is what keeps that honest.
- **T4 reverses a written instruction.** Recorded as a decision rather than a silent edit, so the
  reasoning survives for whoever reads D20 next.

## Open questions
- None.

## Amendments
- 2026-08-29 — `--here` / primary-checkout sandboxes dropped from scope by the human before
  implementation; worktrees remain the only mode. Removed the `scripts/sluss`, `internal/fleet`
  and `scripts/test-sluss` slices and the `Sandbox.Primary` contract entirely.
