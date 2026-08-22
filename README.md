# sluss

A single-binary control plane over [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). Creates an isolated agent environment per repo, serves each one's OpenCode Web UI at a stable URL, and keeps the work reviewable with ordinary git.

*sluss* is Swedish for an airlock or canal lock — an enclosed chamber things pass through in isolation.

**Status: M0 spike in progress.** The design is settled, but its remaining assumptions
must be verified before implementation starts. See `docs/SPIKE.md`.

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

Install Docker Sandboxes on macOS or Ubuntu 24.04+:

```bash
./scripts/install-sbx.sh
```

The same script can be piped directly from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install-sbx.sh | bash
```

Review downloaded scripts before running them if you do not trust the source. The script
uses Docker's official Homebrew tap on macOS and apt repository on Ubuntu, then prints
the interactive login and policy setup commands. It does not install `sluss` yet because
the project is still in its M0 spike.

```bash
task build     # host binary → dist/sluss
task check     # gofmt + go vet + go test
```

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer. The container builds and tests the Go code; anything touching a real sandbox runs on the host.
