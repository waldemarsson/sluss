# sluss

A single binary that gives every task its own isolated agent sandbox, serves each one's web UI at
a stable URL, and keeps the work reviewable with ordinary git.

sluss is a thin layer over [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`). It
creates a git worktree and a branch per task, hands both to sbx, and shows the result — every
sandbox across every scope — in a dashboard you can drive from a phone.

*sluss* is Swedish for an airlock or canal lock: an enclosed chamber things pass through in
isolation.

## Contents

- [Install](#install) · [Update](#update) · [Uninstall](#uninstall)
- [Getting started](#getting-started)
- [Commands](#commands)
- [Configuration](#configuration)
- [The dashboard](#the-dashboard)
- [How sandbox web UIs are exposed](#how-sandbox-web-uis-are-exposed)
- [Security](#security)
- [Running as a service](#running-as-a-service)
- [Development](#development)

## Requirements

- **Docker Sandboxes (`sbx`)** — the runtime sluss delegates isolation to. Install it with
  [Docker's guide](https://docs.docker.com/ai/sandboxes/install/). sbx needs hardware
  virtualisation and its own daemon.
- **git** — sluss creates worktrees and branches with it.
- macOS or Linux, on arm64 or amd64.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh | sh
```

The script detects your operating system and architecture, downloads the matching release
archive, **verifies its SHA-256 against the release's `checksums.txt` before replacing
anything**, and installs `sluss` to `~/.local/bin`. Running it again upgrades in place.

| Variable | Default | Effect |
|---|---|---|
| `SLUSS_INSTALL_DIR` | `~/.local/bin` | Where the binary is installed |
| `SLUSS_VERSION` | `latest` | Install a specific tag, for example `v0.4.0` |

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh |
  SLUSS_INSTALL_DIR=/usr/local/bin SLUSS_VERSION=v0.4.0 sh
```

If `~/.local/bin` is not on your `PATH`, the installer says so and shows how to add it.

### Update

```bash
sluss update
```

`sluss update` resolves the latest release, compares it with the running version, and stops there
if there is nothing to do. Otherwise it downloads the archive, verifies its checksum, and replaces
the running binary atomically — an interrupted update leaves either the old sluss or the new one,
never half of either. `SLUSS_VERSION` pins a specific tag here too.

It refuses when the binary is a symlink, which normally points into a checkout that `git pull`
should update instead, and when its directory is not writable.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/uninstall.sh | sh
```

This removes the binary and nothing else. Your configuration file, worktrees, branches and
sandboxes all survive; the script lists where they are rather than deleting them.

## Getting started

```bash
# 1. Tell sluss which sbx scope to work in.
export SLUSS_APP_NAME=personal

# 2. From a git checkout, create a task sandbox.
cd ~/src/myproject
sluss start auth

# 3. Start its agent and open the UI it prints.
sluss attach auth

# 4. When the work is merged, take it away.
sluss destroy auth
```

`sluss start auth` creates branch `agent/auth`, a worktree at
`$SLUSS_WORKTREE_ROOT/myproject/auth`, and an sbx sandbox mounting both that worktree and the
repository's shared git metadata. One task, one branch, one sandbox.

## Commands

Run `sluss COMMAND --help` — or `sluss help COMMAND` — for the full documentation of any of these.

| Command | What it does |
|---|---|
| `sluss start NAME [--agent AGENT] [SBX_ARGS…]` | Create a task sandbox, or start an existing one |
| `sluss list` (`ls`) | List the scope's sandboxes, and this repository's worktrees |
| `sluss attach NAME [AGENT_ARGS…]` (`run`) | Start the sandbox's agent and enable its remote interface |
| `sluss exec NAME [COMMAND…]` | Run a command, or open a shell, inside a sandbox |
| `sluss stop NAME…` | Stop sandboxes, preserving their state |
| `sluss destroy NAME [--force]` (`rm`) | Remove a sandbox, its worktree and its branch |
| `sluss path NAME` | Print a task's host worktree path |
| `sluss serve` | Serve the dashboard and the sandbox reverse proxy |
| `sluss doctor` | Check that this deployment actually works |
| `sluss update` | Install the latest release over this one |
| `sluss version` | Print the version |

Run the repository commands — `start`, `destroy`, `path`, and `list`'s worktree section — from the
primary checkout or one of its worktrees. `attach` and `exec` read the sandbox from sbx, so they
run from anywhere.

### `start`

Everything after `NAME` except `--agent` is forwarded to `sbx create` verbatim, on first creation
only: sbx's creation settings are immutable, so a later `sluss start` on the same task starts the
existing sandbox and ignores them. `--agent` is consumed by sluss and never reaches sbx; pass `--`
to end sluss's own option scanning and forward the rest untouched.

```bash
sluss start budget                                   # opencode, the default
sluss start budget --agent claude
sluss start budget --agent copilot --kit ~/kits/dotnet-svelte
sluss start budget --cpus 4 --memory 8g --publish 5173
```

For an opencode sandbox, port 4096 is published automatically unless you published it yourself.

If `sbx create` fails, the worktree and branch sluss just made are removed again.

### `attach`

The agent is read back from `sbx ls --json`, so a sandbox keeps the agent it was created with even
if `SLUSS_AGENT` changes later. Each agent's remote interface is enabled the way that agent
expects:

| Agent | What `attach` does | Where you reach it |
|---|---|---|
| `opencode` | Starts OpenCode Web on sandbox port 4096 | The host port it prints, or through `sluss serve` |
| `claude` | Enables Claude Code Remote Control | `claude.ai/code` and the Claude mobile app |
| `copilot` | Starts Copilot CLI with `--remote` | GitHub.com and GitHub Mobile |
| anything else | `sbx run` | Your terminal |

Attaching twice to an opencode sandbox does not start a second server.

### `destroy`

Without `--force`, `destroy` refuses to act when the worktree has uncommitted changes, or when the
branch holds commits the primary checkout's `HEAD` does not. Both refusals leave the sandbox
untouched, and both name `--force` as the way past.

`--force` discards uncommitted changes and unmerged commits **irreversibly**.

### Optional fish helper

```fish
source /path/to/sluss/scripts/sluss.fish   # adds sluss-cd
sluss-cd auth                              # cd into the task's worktree
```

## Configuration

`sluss serve` and `sluss doctor` read one hand-edited JSON file. It is never written back: the
dashboard edits kits and secrets, not its own configuration.

Default path: `~/.config/sluss/config.json`, or `$XDG_CONFIG_HOME/sluss/config.json` when that is
set. Override with `-config PATH`.

```json
{
  "lan": false,
  "port": 8420,
  "access": "path",
  "appNames": ["personal", "work"],
  "repos": ["/home/you/src/myproject"],
  "kitsDir": "/home/you/kits",
  "worktreeRoot": "/home/you/src/worktrees"
}
```

| Key | Type | Default | Effect |
|---|---|---|---|
| `lan` | bool | `false` | `false` binds `127.0.0.1`; `true` binds every interface. See [Security](#security). |
| `port` | int | `8420` | The one listener. Dashboard, API and every sandbox share it. |
| `access` | string | `"path"` | `"path"` or `"host"` — how sandbox URLs are formed. See below. |
| `hostPrefix` | string | `"sluss-"` | Host mode only: the hostname prefix before the sandbox name. |
| `domain` | string | — | Host mode only, and **required** there: the wildcard domain. |
| `appNames` | []string | — | **Required.** The sbx scopes to poll. The first is the command line's default scope. |
| `repos` | []string | `[]` | Repositories the dashboard may create sandboxes from. A create request naming anything else is rejected. |
| `kitsDir` | string | — | Directory of hand-authored kits, for the dashboard's kit editor. |
| `worktreeRoot` | string | `~/src/worktrees` | Where task worktrees are created, as `<root>/<repository>/<name>`. |

A failure to bind is fatal and names the address it tried — sluss never quietly binds something
else.

### Environment variables

The command line reads these first and falls back to the configuration file, so a shell that
exports them keeps working with no config file at all.

| Variable | Falls back to | Effect |
|---|---|---|
| `SLUSS_APP_NAME` | `appNames[0]` | The sbx scope every command acts in. Required; there is no implicit default scope. |
| `SLUSS_WORKTREE_ROOT` | `worktreeRoot`, else `~/src/worktrees` | Where worktrees are created |
| `SLUSS_AGENT` | `opencode` | The agent new sandboxes are created with |
| `SLUSS_VERSION` | `latest` | Which release `sluss update` installs |

### Checking a deployment

```bash
sluss doctor
```

Reports, in order: the bound address and whether it is loopback, the access mode and the URLs it
implies, sbx's version, each configured scope, git, whether the worktree root is writable, each
configured repository, and the kits directory. Exits non-zero if any check fails.

## The dashboard

```bash
sluss serve          # http://127.0.0.1:8420
```

`serve` polls every configured scope, joins what sbx knows with what git knows about each
worktree, and serves the result. It stores nothing: an `sbx rm` by hand shows up within one poll
(`-interval`, 3s by default).

The fleet is the home page. Each sandbox carries its own lifecycle controls — **start** for a
stopped one, **stop** for a running one, and **destroy**, which always asks for confirmation
first. Destroy defers to the same refusal on uncommitted or unmerged work that the command line
gives; the per-sandbox **force** box discards it instead, and is cleared again after every destroy
so it can never carry over to another sandbox.

A top bar leads to `/config/secrets` and `/config/kits`. Secrets are write-only per scope — names
are listed, values only ever go in. Each kit's `spec.yaml` gets a plain-text editor with the kits
directory's git status beside it; committing kits stays a manual step.

Both work on a phone: the fleet table becomes a card per sandbox on a narrow screen, and the
create form collapses behind a **New sandbox** button so the sandboxes are what you see first.

### HTTP API

The dashboard is a client of this; nothing here is private to it.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/config` | Access mode and creatable repositories, for forming URLs |
| `GET` | `/api/fleet` | The current snapshot |
| `GET` | `/api/events` | Server-sent events: a snapshot per poll |
| `POST` | `/api/sandboxes` | Create — body names `repo`, `name`, `agent`, `scope`, `extra` |
| `POST` | `/api/sandboxes/{scope}/{name}/start` | Start a stopped sandbox |
| `POST` | `/api/sandboxes/{scope}/{name}/stop` | Stop a running one |
| `DELETE` | `/api/sandboxes/{scope}/{name}[?force=true]` | Destroy |
| `GET` | `/api/secrets/{scope}` | Secret **names** only |
| `PUT`/`DELETE` | `/api/secrets/{scope}/{name}` | Set or remove a secret value |
| `GET` | `/api/kits` | List kits |
| `GET`/`PUT` | `/api/kits/{name}/spec` | Read or write a kit's `spec.yaml` as opaque text |

Lifecycle routes answer `{"exitCode": N, "stdout": "…", "stderr": "…"}`. A refusal is `409` with a
non-zero `exitCode` and the reason in `stderr` — it is an answer, not a server error. `force` must
be spelled exactly `?force=true`; any other value takes the safe path.

Mutating requests reject cross-site browser calls and require `Content-Type: application/json` on
`POST`. That closes drive-by requests from other pages. **It is not authentication** — see below.

## How sandbox web UIs are exposed

Every sandbox is reached over the **same port** `serve` binds. There is no second listener and no
port per sandbox.

Only **opencode** sandboxes are proxied, because only they have a web UI to proxy: sluss forwards
to the host port sbx published for sandbox port 4096. Claude and Copilot sandboxes are deep-linked
from the dashboard to `claude.ai/code` and `github.com` instead, which is where their remote
interfaces actually live.

### `"access": "path"` (the default)

```
http://<host>:<port>/s/<scope>/<name>/
```

For example `http://127.0.0.1:8420/s/personal/auth/`. This needs **no DNS, no certificate and no
reverse proxy** — it is the mode to use unless you have a reason not to.

### `"access": "host"`

```
https://<hostPrefix><name>.<domain>/
```

For example `https://sluss-auth.example.com/`. The hostname, not the path, says which sandbox is
meant, so this needs **wildcard DNS** (`*.example.com` → the host running sluss) and a matching
wildcard certificate, normally terminated by a proxy in front of sluss. Use it when a sandboxed
app misbehaves under a path prefix.

Both modes stream responses without buffering, which is what makes the proxied UI feel live rather
than broken.

## Security

**sluss has no authentication.** Reachability is the whole boundary:

- `"lan": false` binds `127.0.0.1`, so nothing outside the machine can reach it.
- `"lan": true` binds every interface, so **anything that can route to the host can drive sluss** —
  create sandboxes, read the fleet, write secrets, and destroy work.

Only set `lan` to `true` behind a network you trust, and put a proxy that authenticates in front
of it if that network is not one you control. Do not port-forward it.

Weigh that against what the dashboard can do: `destroy` with `force` discards uncommitted and
unmerged work irreversibly, so anyone who can reach the page can throw away an afternoon of it.

The cross-site rejection on mutating routes closes drive-by requests from other web pages a
browser happens to have open. It is not a login and must not be described as one.

## Running as a service

`serve` handles `SIGINT` and `SIGTERM` as "stop", gives in-flight streams five seconds, and exits.
A minimal systemd unit:

```ini
[Unit]
Description=sluss
After=network-online.target docker.service

[Service]
ExecStart=%h/.local/bin/sluss serve
Restart=on-failure

[Install]
WantedBy=default.target
```

Install it as a **user** unit (`~/.config/systemd/user/sluss.service`, then
`systemctl --user enable --now sluss`): sbx runs per user, and sluss must run as the user that
owns the sandboxes, the checkouts and the worktree root.

## Development

```bash
task web         # build the dashboard into web/dashboard/build (go:embed reads it there)
task build       # host binary → dist/sluss, dashboard included
task check       # gofmt + go vet + go test + shell linting + the install-script tests
task check:web   # Prettier + svelte-check
task test:web    # dashboard unit and component tests
```

No Go test shells out to a real `sbx`: `internal/sbxstub` puts a fake one on `PATH`, and the
lifecycle tests drive real git repositories in temporary directories. The install and uninstall
scripts are tested against a fake release served over `file://`, so that suite runs offline too.

`sbx` needs hardware virtualisation and its own daemon, so it does not run in the devcontainer.
The container builds and tests the code; anything touching a real sandbox runs on the host.

Releases are built by CI from a `v*` tag: four archives — darwin/arm64, darwin/amd64, linux/amd64,
linux/arm64 — plus `checksums.txt`, which is what the install script and `sluss update` verify
against.

## Docs

| File | Purpose |
|---|---|
| [docs/DECISIONS.md](docs/DECISIONS.md) | Why choices were made, what was rejected |
| [docs/SPEC.md](docs/SPEC.md) | The design as written before the GUI work |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What shipped, what is still unverified |
| [docs/SPIKE.md](docs/SPIKE.md) | sbx assumptions, verified before coding |
| [docs/PROFILES.md](docs/PROFILES.md) | A deferred per-client profile design |
| [docs/BACKGROUND.md](docs/BACKGROUND.md) | How the design got here, and an honest read |
| [AGENTS.md](AGENTS.md) | Instructions for coding agents |
