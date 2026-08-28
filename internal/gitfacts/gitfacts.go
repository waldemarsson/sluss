// Package gitfacts reads the git state of one sandbox worktree.
//
// sluss stores none of this: it is recomputed every poll from the worktree path sbx
// reports, so a commit, a stash or an "rm -rf" shows up within one tick.
package gitfacts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Facts is what the dashboard shows for a worktree.
type Facts struct {
	Branch   string // the worktree's checked-out branch, or "HEAD" when detached
	Dirty    bool   // uncommitted changes, tracked or not
	Unmerged int    // commits on this branch that the primary checkout's HEAD lacks
	Missing  bool   // the worktree path is gone from disk
}

// Read gathers the facts for worktreePath, counting unmerged commits against
// repoPath's current HEAD — the same base "sluss destroy" refuses on, so the
// dashboard's count and the script's refusal never disagree.
//
// A worktree that no longer exists is not an error: it is a state the fleet has to
// render. An empty repoPath means the repository is unknown, so no count is made.
func Read(ctx context.Context, repoPath, worktreePath string) (Facts, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return Facts{Missing: true}, nil
		}
		return Facts{}, fmt.Errorf("checking worktree %s: %w", worktreePath, err)
	}

	branch, err := git(ctx, worktreePath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return Facts{}, err
	}
	status, err := git(ctx, worktreePath, "status", "--porcelain")
	if err != nil {
		return Facts{}, err
	}
	facts := Facts{Branch: branch, Dirty: status != ""}

	if repoPath == "" {
		return facts, nil
	}
	base, err := git(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return Facts{}, err
	}
	count, err := git(ctx, worktreePath, "rev-list", "--count", base+"..HEAD")
	if err != nil {
		return Facts{}, err
	}
	facts.Unmerged, err = strconv.Atoi(count)
	if err != nil {
		return Facts{}, fmt.Errorf("parsing commit count %q for %s: %w", count, worktreePath, err)
	}
	return facts, nil
}

// git runs one command in dir and returns its trimmed stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("git %s in %s: %s", strings.Join(args, " "), dir, firstLine(detail))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
