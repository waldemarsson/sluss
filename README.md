# sluss

A single-binary control plane over [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). Creates an isolated agent environment per repo, serves each one's OpenCode Web UI at a stable URL, and keeps the work reviewable with ordinary git.

*sluss* is Swedish for an airlock or canal lock — an enclosed chamber things pass through in isolation.

**Status: pre-code.** The design is settled; the assumptions it rests on are not. `docs/SPIKE.md` has seven unanswered questions, two of them load-bearing, and nothing gets built until they're answered.

## Docs

| File | Purpose |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | What we're building, architecture, size estimate |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Milestone order, gates, what's still open |
| [docs/SPIKE.md](docs/SPIKE.md) | Assumptions to verify **before writing code** |
| [docs/DECISIONS.md](docs/DECISIONS.md) | Why choices were made, what was rejected |
| [docs/PROFILES.md](docs/PROFILES.md) | Deferred per-client profile design |
| [docs/BACKGROUND.md](docs/BACKGROUND.md) | How the design got here, and an honest read |
| [AGENTS.md](AGENTS.md) | Instructions for coding agents |

## Development

```bash
task build     # host binary → dist/sluss
task check     # gofmt + go vet + go test
```

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer. The container builds and tests the Go code; anything touching a real sandbox runs on the host.
