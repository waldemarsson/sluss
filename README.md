# sluss

A single-binary control plane over [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). Creates an isolated agent environment per repo, serves each one's OpenCode Web UI at a stable URL, and keeps the work reviewable with ordinary git.

*sluss* is Swedish for an airlock or canal lock — an enclosed chamber things pass through in isolation.

**Status:** the planned Go application is on hold. The repository currently provides a
small shell wrapper for the sbx and Git-worktree workflow.

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

Install the lightweight `sluss` script:

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh | bash
```

This downloads the script from `main`, validates its shell syntax, and installs it to
`~/.local/bin`. Override the Git ref or destination with `SLUSS_REF` and
`SLUSS_INSTALL_DIR`:

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh |
  SLUSS_REF=my-branch SLUSS_INSTALL_DIR=/usr/local/bin bash
```

Docker Sandboxes is a separate runtime prerequisite; follow
[Docker's installation guide](https://docs.docker.com/ai/sandboxes/install/).

## Lightweight sbx workflow

For local development, install the checked-out script directly:

```bash
install -m 0755 scripts/sluss ~/.local/bin/sluss
```

Set optional defaults in `~/.config/fish/config.fish`:

```fish
set -gx SLUSS_WORKTREE_ROOT "$HOME/src/worktrees"
set -gx SLUSS_AGENT opencode
set -gx SLUSS_APP_NAME personal # optional identity and secret scope
source /path/to/sluss/scripts/sluss.fish # adds sluss-cd
```

Then, from the primary checkout:

```fish
sluss start auth --kit ~/kits/omegapoint --publish 14096:4096
sluss list
sluss-cd auth
sluss attach auth
sluss stop auth
sluss destroy auth       # refuses dirty or unmerged work
```

Every command has focused documentation, for example `sluss start --help` and
`sluss destroy --help`. The equivalent `sluss help start` form also works.

`sluss start` creates `agent/<name>` under
`$SLUSS_WORKTREE_ROOT/<repository>/<name>`, mounts both the worktree and shared Git
metadata, and removes the worktree if sandbox creation fails. Extra arguments are passed
to `sbx create`. Calling it again starts an existing stopped sandbox.

`sluss attach` enables the selected agent's remote interface automatically:

- OpenCode starts `opencode web` on sandbox port 4096 and prints its published port.
- Claude Code starts Remote Control for access through `claude.ai/code` and its mobile app.
- GitHub Copilot CLI enables remote steering through GitHub.com and GitHub Mobile.

OpenCode's automatic host port is loopback-only. Set `OPENCODE_SERVER_PASSWORD` and
publish an explicit LAN-facing host address before exposing it beyond the host.

## Development

```bash
task build     # host binary → dist/sluss
task check     # gofmt + go vet + go test
```

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer. The container builds and tests the Go code; anything touching a real sandbox runs on the host.
