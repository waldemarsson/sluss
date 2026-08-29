package sbx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Streams are the three standard files an interactive sbx command borrows from
// its caller. They are io interfaces rather than *os.File so tests can drive them,
// but os/exec passes an *os.File straight through to the child, so a real terminal
// stays a real terminal — which is what "attach" and "exec" need.
type Streams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// CreateOpts are the arguments of "sbx create".
//
// Extra is forwarded verbatim and in order: sluss never reorders what the caller
// passed, because sbx's tolerance for reordering is unverified (docs/SPIKE.md).
type CreateOpts struct {
	Name       string
	Agent      string
	Workspaces []string // host paths mounted writable, in the order sbx expects
	Extra      []string // publish flags and anything the caller passed through
}

// Create makes a sandbox. The argument order — --name, extras, agent, workspaces —
// is the order the retired shell script used, reproduced deliberately.
func (c *Client) Create(ctx context.Context, appName string, opts CreateOpts) error {
	if opts.Name == "" {
		return errors.New("sbx create: no sandbox name")
	}
	if opts.Agent == "" {
		return errors.New("sbx create: no agent")
	}
	args := []string{"create", "--name", opts.Name}
	args = append(args, opts.Extra...)
	args = append(args, opts.Agent)
	args = append(args, opts.Workspaces...)
	_, err := c.run(ctx, appName, nil, args...)
	return err
}

// Remove deletes a sandbox. sbx's own --force is always passed: it means "do not
// prompt", not "discard work". Whether the *worktree* may be discarded is a
// separate decision made in internal/lifecycle (D20).
func (c *Client) Remove(ctx context.Context, appName, name string) error {
	if name == "" {
		return errors.New("sbx rm: no sandbox name")
	}
	_, err := c.run(ctx, appName, nil, "rm", "--force", name)
	return err
}

// Stop stops one or more sandboxes without touching their state.
func (c *Client) Stop(ctx context.Context, appName string, names ...string) error {
	if len(names) == 0 {
		return errors.New("sbx stop: no sandbox named")
	}
	_, err := c.run(ctx, appName, nil, append([]string{"stop"}, names...)...)
	return err
}

// Exec runs a command inside a sandbox and waits for it, discarding its output.
// Interactive callers want Interactive instead.
func (c *Client) Exec(ctx context.Context, appName, name string, argv ...string) error {
	if name == "" {
		return errors.New("sbx exec: no sandbox name")
	}
	_, err := c.run(ctx, appName, nil, append([]string{"exec", name}, argv...)...)
	return err
}

// Ports reports a sandbox's published port mappings as sbx prints them.
func (c *Client) Ports(ctx context.Context, appName, name string) (string, error) {
	if name == "" {
		return "", errors.New("sbx ports: no sandbox name")
	}
	out, err := c.run(ctx, appName, nil, "ports", name)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// SettingSet writes one sbx setting for a scope.
func (c *Client) SettingSet(ctx context.Context, appName, key, value string) error {
	if key == "" {
		return errors.New("sbx settings set: no key")
	}
	_, err := c.run(ctx, appName, nil, "settings", "set", key, value)
	return err
}

// ListRaw returns "sbx ls" as sbx prints it, for the human-facing listing.
func (c *Client) ListRaw(ctx context.Context, appName string) (string, error) {
	out, err := c.run(ctx, appName, nil, "ls")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Interactive runs a scoped sbx command with the caller's own stdin, stdout and
// stderr, and reports the child's exit code.
//
// A non-zero exit is a value, not an error: "sluss exec NAME false" should exit 1
// without sluss claiming anything went wrong. Only a failure to run sbx at all —
// no binary, no permission — comes back as an error.
func (c *Client) Interactive(ctx context.Context, appName string, streams Streams, args ...string) (int, error) {
	if appName == "" {
		return 0, ErrNoAppName
	}
	argv := append([]string{"--app-name", appName}, args...)

	cmd := exec.CommandContext(ctx, c.binary(), argv...)
	cmd.Stdin = streams.Stdin
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	// errors.As unwraps to the concrete type exec returns when the child ran and
	// exited non-zero; anything else means it never ran.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, classify(appName, strings.Join(argv, " "), "", err)
}

// InteractiveExec opens a command — or a shell, with no argv — inside a sandbox,
// attached to the caller's terminal.
func (c *Client) InteractiveExec(ctx context.Context, appName, name string, streams Streams, argv ...string) (int, error) {
	if name == "" {
		return 0, errors.New("sbx exec: no sandbox name")
	}
	if len(argv) == 0 {
		argv = []string{"sh"}
	}
	return c.Interactive(ctx, appName, streams, append([]string{"exec", "-it", name}, argv...)...)
}

// InteractiveRun starts a sandbox's agent, attached to the caller's terminal.
// Everything after "--" belongs to the agent, not to sbx.
func (c *Client) InteractiveRun(ctx context.Context, appName, name string, streams Streams, agentArgs ...string) (int, error) {
	if name == "" {
		return 0, fmt.Errorf("sbx run: no sandbox name")
	}
	args := []string{"run", "--name", name, "--"}
	return c.Interactive(ctx, appName, streams, append(args, agentArgs...)...)
}
