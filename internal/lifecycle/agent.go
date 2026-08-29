package lifecycle

import (
	"context"
	"fmt"
	"io"

	"github.com/waldemarsson/sluss/internal/sbx"
)

// openCodeWeb is run inside the sandbox by "sluss attach" for an opencode sandbox.
//
// The PID file makes repeated attaches idempotent without having to ask what is
// already listening: a second attach finds a live process and does nothing, so it
// never starts a second server on the same port.
const openCodeWeb = `pid_file=/tmp/sluss-opencode-web.pid
if test -f "$pid_file" && kill -0 "$(cat "$pid_file")" 2>/dev/null; then
	exit 0
fi
nohup opencode web --hostname 0.0.0.0 --port 4096 "$@" \
	>/tmp/sluss-opencode-web.log 2>&1 &
echo $! > "$pid_file"
`

// AgentFor reports which agent a sandbox was created with.
//
// sbx is the source of truth — nothing is persisted host-side — so an agent chosen
// at creation is still the agent used months later, whatever the environment says
// now. A *failed* listing is an error rather than a guess: guessing there would
// report the wrong cause exactly when the source of truth is unreachable. Only a
// successful listing that names no agent falls back, and it says so.
func (r *Runner) AgentFor(ctx context.Context, appName, name string, warn io.Writer) (string, error) {
	sandboxes, err := r.Sbx.List(ctx, appName)
	if err != nil {
		return "", fmt.Errorf("sbx ls failed; cannot determine the agent for %s: %w", name, err)
	}
	for _, s := range sandboxes {
		// An exact match, so "gh" never resolves the agent of "gh-remote".
		if s.Name == name && s.Agent != "" {
			return s.Agent, nil
		}
	}
	fallback := r.agentFor("")
	if warn != nil {
		fmt.Fprintf(warn, "warning: sbx reported no agent for %s; assuming %s\n", name, fallback)
	}
	return fallback, nil
}

// Attach starts a sandbox's agent and enables its remote interface.
//
// Unlike the other repository commands this one needs no worktree context: the
// agent comes from sbx, so attach runs from any directory.
func (r *Runner) Attach(ctx context.Context, appName, name string, streams sbx.Streams, args []string) (int, error) {
	if appName == "" {
		return 0, sbx.ErrNoAppName
	}
	if name == "" {
		return 0, misuse("attach: no sandbox named")
	}

	agent, err := r.AgentFor(ctx, appName, name, streams.Stderr)
	if err != nil {
		return 0, err
	}

	switch agent {
	case "opencode":
		// OpenCode Web is a background server, not a session to sit in: start it,
		// then say which host port reaches it.
		argv := append([]string{"sh", "-c", openCodeWeb, "sh"}, args...)
		if err := r.Sbx.Exec(ctx, appName, name, argv...); err != nil {
			return 0, err
		}
		ports, err := r.Sbx.Ports(ctx, appName, name)
		if err != nil {
			return 0, err
		}
		fmt.Fprintf(streams.Stdout, "OpenCode Web port:\n%s", ports)
		return 0, nil

	case "claude":
		if err := r.Sbx.SettingSet(ctx, appName, "claude.remoteControl", "true"); err != nil {
			return 0, err
		}
		return r.Sbx.InteractiveRun(ctx, appName, name, streams, append([]string{"--remote-control"}, args...)...)

	case "copilot":
		return r.Sbx.InteractiveRun(ctx, appName, name, streams, append([]string{"--remote"}, args...)...)

	default:
		return r.Sbx.InteractiveRun(ctx, appName, name, streams, args...)
	}
}

// ExecIn runs a command inside a sandbox, or opens a shell when none is given.
// The child's exit code becomes sluss's, so "sluss exec NAME false" exits 1.
func (r *Runner) ExecIn(ctx context.Context, appName, name string, streams sbx.Streams, argv []string) (int, error) {
	if appName == "" {
		return 0, sbx.ErrNoAppName
	}
	if name == "" {
		return 0, misuse("exec: no sandbox named")
	}
	return r.Sbx.InteractiveExec(ctx, appName, name, streams, argv...)
}
