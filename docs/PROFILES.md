# Deferred: per-client profiles

**Status:** designed, researched, deliberately not in the MVP. Nothing here needs building until the trigger conditions below are met.

**Why it's written down:** this was the strategically interesting part of the original concept and it took real digging to work out. It is additive — adding it later requires no refactor — so there's no cost to deferring, but there would be a cost to forgetting.

---

## The problem it solves

Consulting across multiple clients means multiple GitHub accounts, multiple SSH identities, different coding standards, and an obligation not to leak one client's context into another's environment. The failure mode is concrete: create a sandbox in the wrong scope, and a commit lands on a client repo authored as your personal account. Recoverable, but embarrassing and easy to do at the end of a long day.

Notably, no tool in the parallel-agent space solves this. They all assume one developer, one identity, N tasks. The closest is Authsome, a standalone credential broker with a profile concept, but it isn't fused to environment lifecycle.

---

## The design

Three layers. The key insight is that **layers 1 and 2 are sbx primitives** — sluss would only add the mapping between them.

### Layer 1 — boundary: `--app-name`

Every sbx command accepts `--app-name`, which scopes daemon state. Separate login, separate secret store, separate sandbox namespace, separate policy.

```bash
sbx --app-name omegapoint login
sbx --app-name omegapoint secret set GITHUB_TOKEN
sbx --app-name personal secret ls      # should not show it
```

`sbx --app-name X reset --force` wipes one scope without touching the others.

This is a real boundary, not a naming convention — which is what makes the design worth building at all. **Verify this before relying on it** (see SPIKE.md assumption 5).

### Layer 2 — content: kit

A kit carries the identity's substance:

```yaml
# ~/.config/sluss/profiles/omegapoint/kit/spec.yaml
schemaVersion: "1"
kind: mixin
name: omegapoint
extends: opencode

network:
  allowedDomains:
    - api.github.com
    - pkgs.dev.azure.com
    - archive.ubuntu.com
    - security.ubuntu.com
    - ports.ubuntu.com      # arm64 — needed for Mac/NAS parity

environment:
  variables:
    DOTNET_CLI_TELEMETRY_OPTOUT: "1"

commands:
  startup:
    - command: ["opencode", "serve", "--hostname", "0.0.0.0", "--port", "4096"]
      background: true
```

Plus `files/home/.gitconfig` for the client's commit identity, `files/home/.ssh/config` for the right key, and a rendered memory file at `kits-memory/omegapoint.md` carrying that client's coding standards into the agent's context.

Kits are distributable: `--kit` accepts a local path, an OCI reference, a ZIP, or `git+ssh://git@github.com/omegapoint/sbx-kits.git#ref=v1.2&dir=omegapoint`. So a private repo of client kits, version-pinned, shareable with colleagues — the same pattern as sharing Copilot skills, but native.

### Layer 3 — environment: `.sbxenv.yaml`

Per-environment overlay (name, workspace path, port), deep-merged over the repo's committed file with docker-compose `-f` semantics.

### What sluss would add

```json
// ~/.config/sluss/profiles/omegapoint.json
{
  "name": "omegapoint",
  "appName": "omegapoint",
  "kit": "~/.config/sluss/profiles/omegapoint/kit",
  "git": { "userName": "Martin", "userEmail": "martin@..." }
}
```

```bash
sluss env create auth --repo ~/src/mercurius --profile omegapoint
sluss profile list
sluss profile login omegapoint
```

**Invariant if built:** no code path may run an sbx command without `--app-name`. Make it a required parameter on the sbx wrapper, not an option with a default.

Estimated size: 150–250 lines for the resolver, plus threading `--app-name` through the sbx wrapper and profile-aware grouping in `env list` and the dashboard.

---

## The MVP workaround

Everything above works today by typing it:

```bash
sbx --app-name omegapoint create opencode \
  --kit ~/kits/omegapoint \
  --name auth ~/src/mercurius-sluss/auth
```

Verbose, and you can forget a flag. But credential isolation is fully intact, because the boundary belongs to sbx, not to sluss.

A shell alias per client gets most of the ergonomics for free:

```fish
alias sbx-op 'sbx --app-name omegapoint'
```

---

## Trigger conditions

Build this when **any** of these is true:

1. You've created a sandbox under the wrong app-name twice
2. A commit has landed authored as the wrong identity
3. You're running three or more client identities regularly
4. A colleague wants to use your kits and needs the setup to be one command
5. `env list` becomes confusing because it can't group by client

Until then, the manual flags are the correct amount of tooling.

---

## Adding it later

No refactor required, because the mechanisms are sbx's:

1. Add `internal/profile` with the resolver
2. Change `internal/sbx` so every command takes an app-name parameter — the compiler finds all the call sites
3. Add `--profile` to `env create`, store it in state
4. Group `env list` and the dashboard by profile

The one thing to get right **now**, in the MVP, so this stays cheap: **centralise all sbx invocation in `internal/sbx`.** If sbx commands get constructed in three different places, adding a required app-name later means hunting them down. One chokepoint, always.
