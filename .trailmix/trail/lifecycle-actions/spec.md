---
slug: lifecycle-actions
title: Sandbox lifecycle actions
created: 2026-08-29
updated: 2026-08-29
waypoint: discuss
status: approved
document: done
---

# Sandbox lifecycle actions — spec

**Problem / why:** The dashboard can destroy work more easily than it can resume it. A stopped
sandbox's row offers only `destroy`; the only way back is retyping the create form. Destroy fires
on one unconfirmed click and can never override the script's dirty/unmerged refusal, so a dirty
sandbox cannot be removed from a phone at all.

**In scope:**

- **Start / resume** — a stopped sandbox gets a `start` action in the fleet, reusing the repo,
  scope and name already in the snapshot.
- **Destroy, confirmed and forceable** — a confirm step before any destroy, and a `force`
  checkbox that passes `--force` through to the script.

**Out of scope:**

- **Primary-checkout (`--here`) sandboxes.** Considered and dropped on 2026-08-29: worktrees stay
  the only mode. This removed the whole `scripts/sluss` slice of the work.
- Orphan worktree detection, kit selection in the create form, `doctor` in the browser, sandbox
  age — the other gaps found in the same review. Separate trails.
- Any change to `scripts/sluss`. Both capabilities this trail needs already exist there.
- Any change to how sandboxes are created or displayed.

**Chosen approach:**

*Start* is a new route, `POST /api/sandboxes/{scope}/{name}/start`, that looks the sandbox up in
the current snapshot and calls the existing `Runner.Start` with the repo, scope and name it finds
there. `start_environment` already resumes an existing worktree (`[[ -d "$worktree" ]]` →
`sbx exec NAME true`), so the script is untouched. The browser never names a repo for this route
— it names a sandbox that already exists, which keeps the create endpoint's configured-repo guard
the only place a client-supplied path is accepted.

*Destroy* gains `?force=true`, threaded to `Runner.Destroy` and on to the script's existing
`--force`. The GUI shows a `force` checkbox beside destroy and confirms before sending, chosen
over a two-stage "try, then confirm the refusal" flow: one round trip, and the unforced refusal
is still shown verbatim when the box is left unchecked.

This reverses a standing project instruction rather than filling a gap in one. `AGENTS.md` states
that slussd "never passes `--force`" and that "a confirmation prompt is the wrong affordance";
`internal/script`'s package and `Destroy` doc comments say the same. The human's judgement is that
an undeletable dirty sandbox is the worse failure, and that a confirm plus an explicit opt-in is a
sufficient guard. The instruction and the comments are updated as part of this work, so nothing in
the repo is left telling a future agent to undo it.

**Constraints:**

- Lifecycle stays in `scripts/sluss` (D14). The daemon passes a flag; it implements nothing.
- All sbx invocation stays in `internal/sbx` (D3, D10).
- sluss holds no state (D12).
- Mutating routes stay behind the existing cross-site `guard`. No token gate (D15).
- `FlushInterval: -1`, the single listener, and secret write-only-ness are untouched.
- The start route accepts no filesystem path from the client. The repo comes from the snapshot.

**Acceptance criteria:**

- [ ] AC1: A stopped sandbox shows a `start` action; using it returns the sandbox to `running`
      within one poll interval, on the same branch and worktree it had before.
- [ ] AC2: `POST /api/sandboxes/{scope}/{name}/start` for a name absent from the snapshot returns
      404 and runs the script not at all.
- [ ] AC3: The start route takes no repository path from the request; the repo used is the one
      the snapshot reports for that sandbox.
- [ ] AC4: Destroy without the force box on a dirty or unmerged sandbox surfaces the script's
      refusal text unchanged, and the sandbox still exists afterwards.
- [ ] AC5: Destroy with the force box checked removes a dirty sandbox, its worktree and its
      `agent/NAME` branch.
- [ ] AC6: Every destroy from the GUI passes through a confirm step first; dismissing it sends no
      request.
- [ ] AC7: The start action is not offered for a running sandbox, and the route is harmless if
      called for one anyway.

**Edge cases:**

- Start pressed on a sandbox that vanished between the snapshot and the click: 404, no script run.
- Start on a sandbox whose `Repo` the fleet could not determine: refuse with the same shape
  `handleDestroy` already uses for that case, rather than running the script from the wrong
  directory.
- Force-destroy of a sandbox whose worktree is already gone from disk: the script's
  `worktree not found` refusal stands; force does not paper over a missing directory.
- The force checkbox must not stay armed across sandboxes — arming it for one row and then
  destroying another must not silently force the second.

**Affected areas (current code):**

- `internal/script/script.go` — a force parameter on `Destroy`, and the package and method doc
  comments that currently state force is never passed.
- `internal/server/server.go` — the start route and `?force=` on destroy.
- `web/dashboard/src/routes/+page.svelte` — start button, confirm, and the force checkbox, in
  both the table and the card layout.
- `AGENTS.md` — the "Destroy is destructive" paragraph, which currently instructs the opposite.
- `docs/DECISIONS.md` — a new entry for force from the GUI.

**Open questions:**
- None.
