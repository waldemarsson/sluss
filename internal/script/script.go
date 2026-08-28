// Package script drives sandbox lifecycle by running scripts/sluss.
//
// The script is the one implementation of worktree lifecycle, and it is unchanged
// by the GUI: creating a sandbox from the browser runs the same code path as typing
// the command, so the worktree, branch and mounts cannot drift apart. Destroy is
// never forced from here — the script's refusal on dirty or unmerged work is the
// safety property, not an obstacle.
package script

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes the sluss script.
type Runner struct {
	Script       string // absolute path to scripts/sluss
	WorktreeRoot string // passed through as SLUSS_WORKTREE_ROOT
}

// StartOpts are the arguments of "sluss start".
type StartOpts struct {
	Repo    string   // repository to run in; becomes the working directory
	Name    string   // sandbox and branch name
	Agent   string   // opencode, claude, copilot… empty means the script's default
	AppName string   // sbx scope
	Extra   []string // forwarded to "sbx create" verbatim
}

// Result is one script run. A non-zero ExitCode is a refusal to be shown to the
// user, not a failure of sluss, so it comes back without an error.
type Result struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// Refused reports whether the script declined to act.
func (r Result) Refused() bool { return r.ExitCode != 0 }

// ResolveScript finds scripts/sluss without ever doing a bare PATH lookup that
// could find something else named sluss.
func ResolveScript() (string, error) {
	looked := []string{}

	if path := os.Getenv("SLUSS_SCRIPT"); path != "" {
		if executable(path) {
			return path, nil
		}
		looked = append(looked, path+" (from SLUSS_SCRIPT)")
	}
	if home, err := os.UserHomeDir(); err == nil {
		installed := filepath.Join(home, ".local", "bin", "sluss")
		if executable(installed) {
			return installed, nil
		}
		looked = append(looked, installed)
	}
	return "", fmt.Errorf("sluss script not found; looked in %s. Install it with scripts/install.sh, or set SLUSS_SCRIPT", strings.Join(looked, ", "))
}

func executable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}

// Start creates or starts a sandbox.
func (r *Runner) Start(ctx context.Context, opts StartOpts) (Result, error) {
	args := append([]string{"start", opts.Name}, opts.Extra...)
	return r.run(ctx, opts.Repo, opts.AppName, opts.Agent, args...)
}

// Stop stops sandboxes without touching their worktrees.
func (r *Runner) Stop(ctx context.Context, repo, appName string, names ...string) (Result, error) {
	if len(names) == 0 {
		return Result{}, errors.New("stop: no sandbox named")
	}
	return r.run(ctx, repo, appName, "", append([]string{"stop"}, names...)...)
}

// Destroy removes a sandbox, its worktree and its branch. --force is deliberately
// not passed and not offered: discarding unmerged work stays a terminal decision.
func (r *Runner) Destroy(ctx context.Context, repo, appName, name string) (Result, error) {
	return r.run(ctx, repo, appName, "", "destroy", name)
}

func (r *Runner) run(ctx context.Context, repo, appName, agent string, args ...string) (Result, error) {
	if r.Script == "" {
		return Result{}, errors.New("no sluss script configured")
	}
	if appName == "" {
		return Result{}, errors.New("app-name is required")
	}

	cmd := exec.CommandContext(ctx, r.Script, args...)
	cmd.Dir = repo
	// The script reads its scope, agent default and worktree root from the
	// environment, which is exactly how a terminal invocation configures it.
	cmd.Env = append(os.Environ(),
		"SLUSS_APP_NAME="+appName,
		"SLUSS_WORKTREE_ROOT="+r.WorktreeRoot,
	)
	if agent != "" {
		cmd.Env = append(cmd.Env, "SLUSS_AGENT="+agent)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return result, nil
	case errors.As(err, &exitErr):
		// The script ran and said no. That is an answer, not an error.
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	default:
		return result, fmt.Errorf("running %s %s: %w", r.Script, strings.Join(args, " "), err)
	}
}
