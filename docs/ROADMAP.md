# sluss — Roadmap

Sequencing for `SPEC.md`. Phases there describe *what* gets built; this describes *in what order*, *what unlocks the next step*, and *where the honest stopping points are*.

**Status:** M0 in progress. Assumption 1 is verified; the remaining assumptions in
`SPIKE.md` must be answered before anything below M0 becomes committed work.

---

## Milestones

| # | Milestone | Deliverable | Exit criterion |
|---|---|---|---|
| **M0** | Spike | `SPIKE.md` answered — 1 and 3 first, then 2, 4, 5, 7. Assumption 6 only if M5 is still wanted. | Verdict paragraph written: build, revise, or abandon |
| **M0.5** | Repo skeleton | Initial commit, `.gitignore`, `AGENTS.md` at root, `idea/` → `docs/`, `go mod init`, Taskfile | `go build ./...` passes |
| **M1a** | Walking skeleton | `env create` (worktree → `sbx create` → state write, with unwind) and `env destroy` (unmerged check → `sbx rm` → worktree remove) | One environment created and destroyed; `git worktree list` correct after both |
| **M1b** | Proxy | `sluss serve` on `127.0.0.1:8420`, host-header routing, `FlushInterval: -1`, routing table rebuilt from state at startup | `auth.sluss.localhost:8420` streams OpenCode tokens at TUI speed |
| **M1c** | Daily loop | `env list`, `env open`, `env stop`, defaults for `--app-name` / `--kit` from `~/.config/sluss/config.json` | Two environments running in parallel on one repo, both reviewed with ordinary `git diff` |
| **GATE** | Two weeks of use | No code | See *Gate* below |
| **M2** | Make it a tool | Reconciliation, sbx error mapping, `doctor`, table-driven tests, restart survival | `sbx rm -f <name>` by hand → `env list` marks it stale and does not crash |
| **M3** | Token auth | `~/.config/sluss/token`, cookie gate on every mutating route, bind moves to `0.0.0.0` | Unauthenticated `DELETE /api/environments/:id` returns 401. Only then: LAN and phone access |
| **M4** | Dashboard | Svelte + `go:embed`, `GET /api/environments`, `GET /api/events` SSE hub | Usable from a phone; the SSE hub exercises the same flush path as the OpenCode proxy |
| **M5** | Linux / NAS | `GOOS=linux` build, VM on TrueNAS SCALE, existing Traefik in front for TLS | Same daily loop works from outside the LAN |
| **—** | Profiles | Only when a trigger in `PROFILES.md` fires | — |

---

## Sequencing decisions

These differ from the phase table in `SPEC.md` §13. The deliverables are unchanged; the order is not.

### Auth comes before the dashboard, not with it

`SPEC.md` §8 bundles the shared token into the dashboard API, but §7 calls it non-negotiable before anything is LAN-reachable. Between M1b and M3 there is a proxy with destroy paths behind it and no gate, one bind-address change away from being exposed.

**Rule for M1b through M2: bind `127.0.0.1` only.** M3 is the milestone that earns `0.0.0.0`. Do not move the bind address early "just to test from the phone".

### Phase 1 splits into three

M1a and M1b can fail independently, and M1b failing makes M1a wasted work. The cheapest falsification order is: throwaway proxy in the spike (M0 assumption 3 already scripts it) → M1a → M1b. By the time real proxy code is written, the premise is already proven.

### The gate gets a date

`SPEC.md` §14 says "two weeks after Phase 1". Vague deadlines don't get enforced against code you've grown fond of.

**When M1c lands, write the date here:**

- M1c completed: `____-__-__`
- Gate review due: `____-__-__`

Gate question, answered honestly: *are environments being created without thinking about it?* If they aren't, this was a tool worth building, not one worth needing — stop at M1 and keep it as a personal script.

The other two kill criteria stay live throughout: SSE unpleasant after tuning → delete the proxy and use `sbx tui`; sbx breaking changes costing more than a day per release → too early to build on this.

---

## Resolved before M1a

### `--isolated` is out of the MVP

`SPEC.md` §6 kept `--clone` as an escape hatch, but D4 deleted the `git ext::` transport that was the only way to get work *out* of a cloned sandbox. As specified, `--isolated` produced work that couldn't be reviewed.

**Decision: not in the MVP.** No flag, no code path. If stronger isolation is wanted later it needs an explicit sync step designed alongside it — `sbx cp`, or pushing to a branch — not a flag that leaves work stranded.

---

## Module path

The confirmed module path is `github.com/waldemarsson/sluss`.

---

## Open — resolve during M0

These are answered by spiking, not by deciding.

| Question | Blocks | Depends on |
|---|---|---|
| **Port allocation.** State (§9) hardcodes `"port": 14096` as if sluss chooses it. If sbx assigns instead, sluss reads-after-create and the routing table must refresh on every sandbox start. | `internal/state`, `internal/proxy` | SPIKE 4 |
| **What drives the SSE hub.** §8 promises a state-change stream; nothing specifies what detects changes — polling `sbx ls`, watching the state directory, or in-process events only. In-process-only is the lean answer and means an external `sbx rm` stays invisible until reconciliation. Whichever is chosen, say so in the spec. | M4 | — (design call, make it in M2) |
| **`env open` semantics.** Print the URL, or launch a browser? There is no browser on the NAS. Print by default. | M1c | — |

---

## Structural invariants

Carried from `AGENTS.md` and `DECISIONS.md`, repeated because they are cheap now and expensive later:

- **All sbx invocation lives in `internal/sbx`,** app-name a parameter on every command. This is what keeps profiles a small addition rather than a hunt (D3, D10).
- **`FlushInterval: -1` is load-bearing** (D5). No buffering middleware, no `Content-Length` on streamed responses.
- **Every multi-step operation gets its unwind path written at the same time as the happy path** — worktree created but `sbx create` failed means the worktree is removed.
- **sluss parses no YAML** (D6). JSON in, `.sbxenv.yaml` emitted only.
