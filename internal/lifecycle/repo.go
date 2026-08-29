package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/waldemarsson/sluss/internal/gitfacts"
)

// RepoRoot finds the primary checkout that dir belongs to, whether dir is the
// checkout itself or one of its worktrees.
//
// --git-common-dir is what makes a worktree resolve to its primary checkout: a
// worktree's own --git-dir points inside .git/worktrees/, while the common dir is
// the shared .git the whole repository uses.
//
// The error is a refusal with no wording of its own: each command says what the
// user should have done instead ("run sluss start inside a Git repository"), so the
// message belongs to the caller.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	common, err := gitfacts.Run(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", refuse("not a Git repository: %s", dir)
	}
	return filepath.Dir(common), nil
}

// RepoName is the directory name a repository's worktrees are grouped under.
func RepoName(repo string) string {
	return strings.TrimSuffix(filepath.Base(repo), ".git")
}

// WorktreePath is where a task's worktree lives: <root>/<repository>/<name>.
func WorktreePath(worktreeRoot, repo, name string) string {
	return filepath.Join(worktreeRoot, RepoName(repo), name)
}

// BranchName is the branch a task's worktree checks out.
func BranchName(name string) string { return "agent/" + name }

// gitDir is the repository's own .git directory, mounted into the sandbox
// alongside the worktree so git commands work inside it.
func gitDir(ctx context.Context, repo string) (string, error) {
	return gitfacts.Run(ctx, repo, "rev-parse", "--absolute-git-dir")
}

// exists reports whether anything occupies path — a file, a directory, or a symlink
// whose target is gone. Lstat rather than Stat, so a dangling symlink counts as
// occupied and start refuses with a clear message instead of letting
// "git worktree add" fail with a confusing one.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// isDir reports whether path is a directory, following symlinks the way the shell's
// "-d" test did.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// branchExists asks git rather than the filesystem: a branch can exist with no
// worktree, which is one of the states start refuses.
func branchExists(ctx context.Context, repo, branch string) bool {
	return gitfacts.OK(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
}
