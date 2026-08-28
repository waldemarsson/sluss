---
slug: agent-selection
title: Per-task agent selection and sbx-sourced agent read-back
created: 2026-08-25
updated: 2026-08-25
waypoint: spec-plan
status: approved
document: done
tasks: T1:done T2:done T3:done
---

# Per-task agent selection and sbx-sourced agent read-back

Single file: `scripts/sluss` (bash). No Go code involved — the Go application is on hold
(README.md:7-8).

## Problem

1. **No per-task agent.** `start_environment` reads only the global `$agent`
   (`SLUSS_AGENT`, default `opencode`) at `scripts/sluss:303,306`. Mixing agents across
   tasks means mutating an env var per invocation. Everything after `NAME` is forwarded
   verbatim to `sbx create`, so there is nowhere to put a flag today.
2. **App-name bypass.** `scripts/sluss:373` calls `command sbx settings set
   claude.remoteControl true` directly, the only sbx call in the script that skips
   `run_sbx` and therefore drops `--app-name`. If sbx settings are per-app-scope, this
   writes to the wrong scope whenever `SLUSS_APP_NAME` is set.
3. **Unwanted state file.** `agent_file()` (`scripts/sluss:225-227`) persists the chosen
   agent to `$SLUSS_WORKTREE_ROOT/<repo>/.sluss/<name>.agent`. Not git-tracked and outside
   every worktree, but still sluss state living in the worktree tree. Decision: remove it
   and read the agent back from sbx instead — SPEC.md:158 already says sbx is the source
   of truth and sluss stores only what sbx doesn't know.

## Probe results (host, sbx from `sbx --help`)

- `sbx ls --json` is documented as listing "all sandboxes with their agent, status,
  published ports, and workspace". The flag is `--json`; `--format json` is rejected.
- `sbx inspect SANDBOX_NAME --json` also reports the agent, but `inspect` **does not
  appear in `sbx --help`'s command list** — it is undocumented at top level. `ls` is
  listed. Prefer `ls --json`.
- `settings` **also does not appear in `sbx --help`'s command list**, yet
  `scripts/sluss:373` calls `sbx settings set claude.remoteControl true`. Either it is
  another hidden command or that line has been failing silently on every Claude attach
  (its `|| return` would then abort the attach before `sbx run`).

Still outstanding: the JSON key names in `sbx ls --json`, and whether `sbx settings`
exists at all.

## Design

### T1 — `--agent NAME` on `sluss start`

Consume `--agent NAME` / `--agent=NAME` from the argument list before the remainder is
forwarded to `sbx create`. Precedence: `--agent` > `$SLUSS_AGENT` > `opencode`.

The resolved agent — not the global `$agent` — must drive the `--publish 4096` default at
`scripts/sluss:303`, otherwise `sluss start x --agent opencode` under `SLUSS_AGENT=claude`
silently loses its port.

`sbx create --help` has not been read; if sbx grows its own `--agent`, sluss shadows it.
Documented in `start_usage` as consumed-by-sluss.

### T2 — route the settings call through `run_sbx`

`command sbx settings set …` → `run_sbx settings set …`. Contingent on the probe: if sbx
rejects `--app-name` on `settings` (i.e. it is a genuinely global user preference), revert
to `command sbx` and leave a comment stating why, so the inconsistency reads as deliberate.

### T3 — replace the metadata file with an sbx read-back

Delete `agent_file()`, its write in `start_environment` (`scripts/sluss:314-317`), its read
in `environment_agent` (`scripts/sluss:229-238`), and its removal in `destroy_environment`
(`scripts/sluss:467`).

`environment_agent NAME` becomes a query against sbx. When sbx does not report an agent for
the sandbox, fall back to `$agent` **and warn on stderr** — this is the hardening item: a
silent wrong-agent attach becomes a visible guess.

Pre-existing `.sluss/` directories on disk are orphaned by this change. They are inert;
note manual removal in the summary rather than adding cleanup code.

## Tasks

- **T1** — `--agent` flag on `start`, resolved agent drives the publish default; `start_usage` + README updated.
- **T2** — settings call through `run_sbx` (or a comment explaining why not).
- **T3** — metadata file removed, `environment_agent` reads from sbx, warns on fallback.

## Gates

- `bash -n scripts/sluss` — the only automated check available in this container.
  `shellcheck`, `shfmt`, and `bats` are all absent; `task check` covers Go only.
- Behavioural verification is host-side and manual: `sluss start` with and without
  `--agent`, `sluss attach` for each of opencode/claude/copilot, `sluss destroy`.

## Out of scope

- `docs/SPEC.md:22` ("Supporting agents other than OpenCode in v1") describes the deferred
  Go application, not this script. Not touched.
- Agent-name validation. `sbx create <agent>` already fails loudly on an unknown agent.
