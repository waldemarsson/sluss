# sluss

A single-binary control plane over [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). Creates an isolated agent environment per repo, serves each one's OpenCode Web UI at a stable URL, and keeps the work reviewable with ordinary git.

*sluss* is Swedish for an airlock or canal lock — an enclosed chamber things pass through in isolation.

**Status:** two pieces, both usable. `sluss` is the shell script that owns the sandbox and
worktree lifecycle. `slussd` is a Go daemon that shows every sandbox across repos and sbx scopes in
a browser, proxies OpenCode Web through one port, and runs the script for you. Neither has yet been
exercised against a real sandbox on the NAS — see [docs/ROADMAP.md](docs/ROADMAP.md).

## Docs

| File | Purpose |
|---|---|
| [docs/SPEC.md](docs/SPEC.md) | What sluss is, how it routes, how it is configured |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What shipped, what is still unverified |
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

## The dashboard (`slussd`)

`slussd` polls every configured sbx scope, joins what sbx knows with what git knows about each
worktree, and serves the result. It stores nothing: an `sbx rm` by hand shows up within one poll.

Build it and write a configuration file:

```bash
task build     # → dist/slussd (builds the dashboard first)
```

`scripts/install.sh` installs the script only, so put the daemon on `PATH` yourself
(`install -m 0755 dist/slussd ~/.local/bin/slussd`) or run it as `./dist/slussd`.

```json
// ~/.config/sluss/config.json
{ "lan": false, "port": 8420, "access": "path",
  "appNames": ["personal", "omegapoint"],
  "repos": ["/Users/marwal/src/mercurius"],
  "kitsDir": "/Users/marwal/kits",
  "worktreeRoot": "/Users/marwal/src/worktrees" }
```

```bash
slussd doctor   # bound address, sbx version, each scope, the script, repos, kits
slussd serve    # http://127.0.0.1:8420
```

`lan: false` binds loopback; `true` binds every interface, for a VM behind a front proxy. A
failure to bind is fatal and names the address — sluss never quietly binds something else.

Sandboxes are reached over that same port. With `"access": "path"` (the default) an OpenCode
sandbox is at `http://127.0.0.1:8420/s/<scope>/<name>/`, which needs no DNS, no certificate and no
reverse proxy. With `"access": "host"` — which additionally needs wildcard DNS and a matching
certificate — it is at `https://sluss-<name>.<domain>/`. Claude Code and Copilot sandboxes are not
proxied; the dashboard deep-links to `claude.ai/code` and `github.com` instead.

There is **no authentication**, by decision (D15): reachability is the boundary, so keep `lan`
false unless the network in front of it is one you trust. Mutating requests do reject cross-site
browser calls, but that closes drive-by requests only — it is not auth.

The dashboard also offers a write-only secrets pane per scope (names are listed, values only ever
go in) and a plain-text editor for each kit's `spec.yaml`, with the kits directory's git status
beside it. Committing kits stays a manual step.

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
sluss start review --agent claude
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

`--agent` picks the agent for one task, overriding `SLUSS_AGENT`; it is consumed by sluss
and never forwarded to `sbx create`. Every other argument after the name still is — pass `--`
to end sluss's option scanning and forward the rest untouched. An existing sandbox keeps the
agent it was created with, so `--agent` is ignored when restarting one.

`sluss attach` reads the sandbox's agent back from `sbx ls --json` — nothing is persisted
host-side — and enables that agent's remote interface automatically:

- OpenCode starts `opencode web` on sandbox port 4096 and prints its published port.
- Claude Code starts Remote Control for access through `claude.ai/code` and its mobile app.
- GitHub Copilot CLI enables remote steering through GitHub.com and GitHub Mobile.

Because the agent comes from sbx, `sluss attach` needs no worktree context and runs from any
directory. The other repository commands still expect the primary checkout or one of its
worktrees.

OpenCode's automatic host port is loopback-only. Set `OPENCODE_SERVER_PASSWORD` and
publish an explicit LAN-facing host address before exposing it beyond the host.

Earlier versions recorded each sandbox's agent in
`$SLUSS_WORKTREE_ROOT/<repository>/.sluss/`. Nothing reads or writes that directory any more;
delete it if one is left over.

## Development

```bash
task web       # build the dashboard into web/dashboard/build (go:embed reads it there)
task build     # host binary → dist/slussd, dashboard included
task check     # gofmt + go vet + go test + shellcheck + the script's black-box tests
```

No Go test shells out to a real `sbx`: `internal/sbxstub` puts a fake one on `PATH` the way
`scripts/test-sluss` already does, and the script tests run the real `scripts/sluss` against it.

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer. The container builds and tests the Go code; anything touching a real sandbox runs on the host.
