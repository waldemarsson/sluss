package main

// The per-command help ported from the shell script sluss used to be. Each topic is
// reachable two ways, as it always was: "sluss start --help" and "sluss help start".

const usageText = `sluss - isolated agent sandboxes over Docker Sandboxes (sbx)

Usage:
  sluss start NAME [--agent AGENT] [SBX_CREATE_ARGS...]
  sluss list
  sluss attach NAME [AGENT_ARGS...]
  sluss exec NAME [COMMAND...]
  sluss stop NAME...
  sluss destroy NAME [--force]
  sluss path NAME
  sluss serve [-config PATH] [-interval DURATION]
  sluss doctor [-config PATH]
  sluss update
  sluss version

Commands:
  start      Create or start a task sandbox without attaching
  list       List sandboxes and this repository's worktrees
  attach     Start or attach to the sandbox's agent
  exec       Open a shell or execute a command in a sandbox
  stop       Stop sandboxes while preserving their state
  destroy    Remove a sandbox, worktree, and task branch safely
  path       Print a task's host worktree path
  serve      Serve the dashboard and the sandbox reverse proxy
  doctor     Check that this deployment actually works
  update     Download and install the latest sluss release
  version    Print the version

Run repository-specific commands from the primary checkout or one of its worktrees.
Set SLUSS_APP_NAME, SLUSS_AGENT, or SLUSS_WORKTREE_ROOT to change defaults; sluss
otherwise reads the scope and worktree root from its configuration file.

Run "sluss COMMAND --help" for command-specific documentation.
`

const startHelp = `Usage: sluss start NAME [--agent AGENT] [SBX_CREATE_ARGS...]

Create or start a task sandbox without attaching to its agent.

On first use, this command:
  1. Creates branch agent/NAME from the primary checkout's current HEAD.
  2. Creates $SLUSS_WORKTREE_ROOT/<repository>/NAME as a Git worktree.
  3. Creates an sbx sandbox named NAME using --agent, else $SLUSS_AGENT,
     else opencode.
  4. Mounts the worktree and the repository's shared Git metadata writable.

For OpenCode, sandbox port 4096 is published to an automatically selected
loopback host port unless SBX_CREATE_ARGS already publish port 4096.

If the worktree already exists, the command starts the sandbox if necessary.
Use "sluss attach NAME" when you want to open the agent interactively.

All arguments after NAME except --agent are passed to "sbx create" on first
creation. They are ignored on subsequent starts because sbx creation settings
are immutable, and so is --agent: an existing sandbox keeps the agent it was
created with. --agent is consumed by sluss and never reaches sbx; pass -- to
end sluss's own option scanning and forward the rest untouched.

Options:
  --agent AGENT  Agent to create the sandbox with, overriding $SLUSS_AGENT
  -h, --help     Show this help

Examples:
  sluss start budget
  sluss start budget --agent claude
  sluss start budget --agent copilot --kit ~/kits/dotnet-svelte
  sluss start budget --cpus 4 --memory 8g --publish 5173

Safety:
  If sbx creation fails, the new worktree and branch are removed. Sandboxes for
  the same repository share writable Git objects and refs; use one branch per task.
`

const listHelp = `Usage: sluss list

List sbx sandboxes in the configured app-name scope. When run inside a Git
repository or one of its worktrees, also list that repository's host worktrees.

Aliases: sluss ls

Options:
  -h, --help    Show this help
`

const attachHelp = `Usage: sluss attach NAME [AGENT_ARGS...]

Start the named sandbox and enable its remote interface:
  opencode  Start OpenCode Web on sandbox port 4096, then show port mappings.
  claude    Enable sbx support and start Claude Code Remote Control.
  copilot   Start Copilot CLI with GitHub remote steering enabled.

Other agents attach normally with "sbx run". The agent is read back from
"sbx ls --json", so the agent chosen at creation is used even if SLUSS_AGENT
later changes. Extra arguments are passed to the agent.

Unlike the other repository commands, this one needs no worktree context and
runs from any directory.

Alias: sluss run

Options:
  -h, --help    Show this help

Examples:
  sluss attach budget
  sluss attach budget --continue

Remote clients:
  OpenCode: open the host port shown by this command in a browser, or reach it
            through "sluss serve" at /s/<scope>/<name>/.
  Claude:   open claude.ai/code or the Claude mobile app.
  Copilot:  open the session from GitHub.com or GitHub Mobile.
`

const execHelp = `Usage: sluss exec NAME [COMMAND [ARG...]]

Start the named sandbox if stopped, then execute a command inside it. With no
COMMAND, open an interactive POSIX shell.

The command's exit status becomes sluss's own.

Options:
  -h, --help    Show this help

Examples:
  sluss exec budget
  sluss exec budget git status
  sluss exec budget npm --prefix frontend test
`

const stopHelp = `Usage: sluss stop NAME...

Stop one or more sandboxes. Their worktrees, branches and sandbox state are
preserved; "sluss start NAME" resumes one.

Options:
  -h, --help    Show this help

Examples:
  sluss stop budget
  sluss stop budget auth
`

const destroyHelp = `Usage: sluss destroy NAME [--force]

Remove a sandbox, its Git worktree, and its agent/NAME branch.

Without --force this refuses to act when the worktree has uncommitted changes,
or when the branch holds commits the primary checkout's HEAD does not. Both
refusals leave the sandbox untouched.

--force discards uncommitted changes and unmerged commits irreversibly.

Alias: sluss rm

Options:
  --force       Discard uncommitted changes and unmerged commits
  -h, --help    Show this help

Examples:
  sluss destroy budget
  sluss destroy budget --force
`

const pathHelp = `Usage: sluss path NAME

Print the host worktree path for a task, whether or not it exists yet.

Options:
  -h, --help    Show this help

Examples:
  sluss path budget
  cd "$(sluss path budget)"
`

const serveHelp = `Usage: sluss serve [-config PATH] [-interval DURATION]

Serve the dashboard and the sandbox reverse proxy on the configured port.

Every configured sbx scope is polled, joined with what Git knows about each
worktree, and served as one fleet. Nothing is stored: an "sbx rm" by hand shows
up within one poll.

Sandboxes are reached over the same port. In "path" access mode an OpenCode
sandbox is at http://HOST:PORT/s/<scope>/<name>/ ; in "host" mode it is at
https://<hostPrefix><name>.<domain>/ , which needs wildcard DNS and a matching
certificate. Claude and Copilot sandboxes are deep-linked rather than proxied.

There is no authentication. Keep "lan" false unless the network in front of
sluss is one you trust.

Options:
  -config PATH        Configuration file (default ~/.config/sluss/config.json)
  -interval DURATION  How often to poll every scope (default 3s)
  -h, --help          Show this help
`

const doctorHelp = `Usage: sluss doctor [-config PATH]

Check that this deployment actually works, and say so before you find out the
hard way: the bound address and whether it is loopback, the access mode and the
URLs it implies, sbx's version, every configured scope, git, the worktree root,
the configured repositories, and the kits directory.

Exits non-zero if any check fails.

Options:
  -config PATH  Configuration file (default ~/.config/sluss/config.json)
  -h, --help    Show this help
`

const updateHelp = `Usage: sluss update

Download the latest sluss release and replace the running binary in place.

The archive's SHA-256 is verified against the release's checksums.txt before
anything is replaced, and the replacement is atomic, so an interrupted update
cannot leave a partial binary behind.

Updating is refused when the running sluss is a symlink: that normally points
into a checkout, which should be updated with git instead.

Environment:
  SLUSS_VERSION   Install this tag instead of the latest release

Options:
  -h, --help    Show this help

Examples:
  sluss update
  SLUSS_VERSION=v0.4.0 sluss update
`

// helpFor maps a command, or one of its aliases, to its help topic.
func helpFor(command string) (string, bool) {
	switch command {
	case "start":
		return startHelp, true
	case "list", "ls":
		return listHelp, true
	case "attach", "run":
		return attachHelp, true
	case "exec":
		return execHelp, true
	case "stop":
		return stopHelp, true
	case "destroy", "rm":
		return destroyHelp, true
	case "path":
		return pathHelp, true
	case "serve":
		return serveHelp, true
	case "doctor":
		return doctorHelp, true
	case "update":
		return updateHelp, true
	}
	return "", false
}
