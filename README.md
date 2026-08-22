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

## Install

Install the current `main` version of sluss from source:

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh | bash
```

This requires Go 1.23+ and installs to `~/.local/bin`. Override the version or destination
with `SLUSS_VERSION` and `SLUSS_INSTALL_DIR`:

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh |
  SLUSS_VERSION=v0.1.0 SLUSS_INSTALL_DIR=/usr/local/bin bash
```

There are no releases yet, so this currently builds the M0 stub rather than a usable CLI.
Docker Sandboxes is a separate runtime prerequisite; follow
[Docker's installation guide](https://docs.docker.com/ai/sandboxes/install/).

## Development

```bash
task build     # host binary → dist/sluss
task check     # gofmt + go vet + go test
```

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer. The container builds and tests the Go code; anything touching a real sandbox runs on the host.
