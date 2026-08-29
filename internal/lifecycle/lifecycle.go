package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/waldemarsson/sluss/internal/gitfacts"
	"github.com/waldemarsson/sluss/internal/sbx"
)

// DefaultAgent is what a sandbox is created with when nothing else says otherwise.
const DefaultAgent = "opencode"

// Runner performs lifecycle operations. It holds no state of its own: every fact
// comes from sbx or git, per call (docs/DECISIONS.md D12).
type Runner struct {
	Sbx          *sbx.Client // the one chokepoint for sbx invocation
	WorktreeRoot string      // where worktrees are created
	Agent        string      // default agent; empty means DefaultAgent
}

// Result is one lifecycle operation's outcome, shaped for both callers: the command
// line prints the streams and exits with the code, and the dashboard renders the
// same three fields as JSON.
//
// A non-zero ExitCode is a refusal to be shown to the user, not a failure of sluss,
// so it comes back without an error — the same contract the shell script had.
type Result struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// Refused reports whether the operation declined to act.
func (r Result) Refused() bool { return r.ExitCode != 0 }

// StartOpts are the arguments of "sluss start".
type StartOpts struct {
	Repo    string   // repository the sandbox belongs to
	Name    string   // sandbox and branch name
	Agent   string   // overrides the runner's default; empty means use it
	AppName string   // sbx scope
	Extra   []string // forwarded to "sbx create" verbatim
}

// ErrNoWorktreeRoot means the runner was built without somewhere to put worktrees.
var ErrNoWorktreeRoot = errors.New("lifecycle: no worktree root configured")

// resultOf turns an operation's output and error into the Result both callers read.
//
// Refusals, misuse and outright failures all become a non-zero exit code with the
// message on stderr, because that is what the shell script did and the dashboard
// already renders. Only a misconfigured runner is an error.
func resultOf(out string, err error) (Result, error) {
	if err == nil {
		return Result{Stdout: out}, nil
	}
	if errors.Is(err, ErrNoWorktreeRoot) || errors.Is(err, sbx.ErrNoAppName) {
		return Result{}, err
	}
	return Result{ExitCode: ExitCode(err), Stdout: out, Stderr: "error: " + err.Error() + "\n"}, nil
}

// agentFor resolves which agent a new sandbox gets: the call, else the runner's
// default, else opencode.
func (r *Runner) agentFor(requested string) string {
	if requested != "" {
		return requested
	}
	if r.Agent != "" {
		return r.Agent
	}
	return DefaultAgent
}

func (r *Runner) check(appName string) error {
	if r.WorktreeRoot == "" {
		return ErrNoWorktreeRoot
	}
	if appName == "" {
		return sbx.ErrNoAppName
	}
	return nil
}

// Start creates a task sandbox, or starts an existing one.
func (r *Runner) Start(ctx context.Context, opts StartOpts) (Result, error) {
	return resultOf(r.start(ctx, opts))
}

func (r *Runner) start(ctx context.Context, opts StartOpts) (string, error) {
	if err := r.check(opts.AppName); err != nil {
		return "", err
	}
	if err := checkName(opts.Name); err != nil {
		return "", err
	}
	if opts.Repo == "" {
		return "", misuse("start: no repository")
	}

	agent := r.agentFor(opts.Agent)
	worktree := WorktreePath(r.WorktreeRoot, opts.Repo, opts.Name)
	branch := BranchName(opts.Name)

	// An existing worktree means this task already exists: start its sandbox rather
	// than trying to create a second one on the same branch.
	if isDir(worktree) {
		if err := r.Sbx.Exec(ctx, opts.AppName, opts.Name, "true"); err != nil {
			return "", err
		}
		return "", nil
	}
	if exists(worktree) {
		return "", refuse("worktree path already exists: %s", worktree)
	}
	if branchExists(ctx, opts.Repo, branch) {
		return "", refuse("branch already exists without its expected worktree: %s", branch)
	}

	git, err := gitDir(ctx, opts.Repo)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(worktree), err)
	}
	if _, err := gitfacts.Run(ctx, opts.Repo, "worktree", "add", "-b", branch, worktree); err != nil {
		return "", err
	}

	// OpenCode's web UI is the whole point of the proxy, so its port is published
	// unless the caller published it themselves. The decision keys off the *resolved*
	// agent, not the runner's default: keying it off the default silently drops the
	// port whenever --agent asks for opencode.
	extra := opts.Extra
	if agent == DefaultAgent && !publishesOpenCodePort(extra) {
		extra = append([]string{"--publish", openCodePort}, extra...)
	}

	createErr := r.Sbx.Create(ctx, opts.AppName, sbx.CreateOpts{
		Name:       opts.Name,
		Agent:      agent,
		Extra:      extra,
		Workspaces: []string{worktree, git},
	})
	if createErr != nil {
		// Unwind, written with the happy path rather than after it: a worktree and
		// branch left behind by a failed create would block the next attempt.
		unwind := r.unwind(ctx, opts.Repo, worktree, branch)
		return "", fmt.Errorf("%w\nsbx creation failed; removed worktree %s%s", createErr, worktree, unwind)
	}
	return fmt.Sprintf("Created %s on %s at %s using %s\n", opts.Name, branch, worktree, agent), nil
}

// unwind removes a worktree and branch created for a sandbox that never appeared.
// It returns a note about anything it could not clean up rather than an error: the
// create failure is the thing worth reporting, and hiding it behind a cleanup
// failure would name the wrong cause.
func (r *Runner) unwind(ctx context.Context, repo, worktree, branch string) string {
	var left []string
	if _, err := gitfacts.Run(ctx, repo, "worktree", "remove", "--force", worktree); err != nil {
		left = append(left, worktree)
	}
	if _, err := gitfacts.Run(ctx, repo, "branch", "-D", branch); err != nil {
		left = append(left, branch)
	}
	if len(left) == 0 {
		return ""
	}
	return "\ncould not clean up: " + strings.Join(left, ", ")
}

// Stop stops sandboxes without touching their worktrees.
//
// repo is accepted and unused: stopping needs no repository, but every lifecycle
// method takes one so callers do not have to know which ones care.
func (r *Runner) Stop(ctx context.Context, repo, appName string, names ...string) (Result, error) {
	_ = repo
	return resultOf(r.stop(ctx, appName, names))
}

func (r *Runner) stop(ctx context.Context, appName string, names []string) (string, error) {
	if appName == "" {
		return "", sbx.ErrNoAppName
	}
	if len(names) == 0 {
		return "", misuse("stop: no sandbox named")
	}
	if err := r.Sbx.Stop(ctx, appName, names...); err != nil {
		return "", err
	}
	return "Stopped " + strings.Join(names, ", ") + "\n", nil
}

// Destroy removes a sandbox, its worktree and its branch.
//
// Without force it refuses uncommitted or unmerged work, and that refusal is the
// answer rather than an error. force discards both irreversibly, so every caller
// asks a human first: the dashboard confirms each destroy and arms force per
// sandbox (docs/DECISIONS.md D20).
func (r *Runner) Destroy(ctx context.Context, repo, appName, name string, force bool) (Result, error) {
	return resultOf(r.destroy(ctx, repo, appName, name, force))
}

func (r *Runner) destroy(ctx context.Context, repo, appName, name string, force bool) (string, error) {
	if err := r.check(appName); err != nil {
		return "", err
	}
	if repo == "" {
		return "", misuse("destroy: no repository")
	}

	worktree := WorktreePath(r.WorktreeRoot, repo, name)
	branch := BranchName(name)
	if !isDir(worktree) {
		return "", refuse("worktree not found: %s", worktree)
	}

	if !force {
		if err := r.refuseUnsafeDestroy(ctx, repo, worktree, branch); err != nil {
			return "", err
		}
	}

	// sbx's own --force means "do not prompt". Whether the worktree may be discarded
	// is the separate question already answered above.
	if err := r.Sbx.Remove(ctx, appName, name); err != nil {
		return "", err
	}

	removeArgs := []string{"worktree", "remove", worktree}
	deleteArgs := []string{"branch", "-d", branch}
	if force {
		removeArgs = []string{"worktree", "remove", "--force", worktree}
		deleteArgs = []string{"branch", "-D", branch}
	}
	if _, err := gitfacts.Run(ctx, repo, removeArgs...); err != nil {
		return "", err
	}
	if _, err := gitfacts.Run(ctx, repo, deleteArgs...); err != nil {
		return "", err
	}
	return fmt.Sprintf("Destroyed %s, its worktree and %s\n", name, branch), nil
}

// refuseUnsafeDestroy is the whole safety story of destroy: uncommitted work first,
// then commits the primary checkout has never seen. Both name --force as the way
// past, and neither has run sbx yet, so a refusal leaves the sandbox untouched.
func (r *Runner) refuseUnsafeDestroy(ctx context.Context, repo, worktree, branch string) error {
	status, err := gitfacts.Run(ctx, worktree, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return refuse("%s has uncommitted changes; commit them or use --force", branch)
	}
	if !gitfacts.OK(ctx, repo, "merge-base", "--is-ancestor", branch, "HEAD") {
		primary, err := gitfacts.Run(ctx, repo, "branch", "--show-current")
		if err != nil {
			return err
		}
		return refuse("%s has commits not merged into %s; merge them or use --force", branch, primary)
	}
	return nil
}

// List shows the scope's sandboxes and, inside a repository, its worktrees.
// An empty repo is not an error: attach and list run from anywhere.
func (r *Runner) List(ctx context.Context, repo, appName string) (Result, error) {
	return resultOf(r.list(ctx, repo, appName))
}

func (r *Runner) list(ctx context.Context, repo, appName string) (string, error) {
	if appName == "" {
		return "", sbx.ErrNoAppName
	}
	sandboxes, err := r.Sbx.ListRaw(ctx, appName)
	if err != nil {
		return "", err
	}
	if repo == "" {
		return sandboxes, nil
	}
	worktrees, err := gitfacts.Run(ctx, repo, "worktree", "list")
	if err != nil {
		return sandboxes, nil
	}
	return fmt.Sprintf("%s\nGit worktrees for %s:\n%s\n", sandboxes, repo, worktrees), nil
}

// Path prints where a task's worktree lives. It asks neither git nor sbx: the path
// is derived from the worktree root, the repository name and the task name.
func (r *Runner) Path(repo, name string) (Result, error) {
	return resultOf(r.path(repo, name))
}

func (r *Runner) path(repo, name string) (string, error) {
	if r.WorktreeRoot == "" {
		return "", ErrNoWorktreeRoot
	}
	if repo == "" {
		return "", misuse("path: no repository")
	}
	return WorktreePath(r.WorktreeRoot, repo, name) + "\n", nil
}
