package gitfacts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/gitfacts"
)

// run executes a git command in dir and fails the test if it errors.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// repoWithWorktree builds a primary checkout with one commit plus a linked worktree
// on agent/task, the same shape scripts/sluss creates.
func repoWithWorktree(t *testing.T) (repo, worktree string) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "repo")
	worktree = filepath.Join(base, "worktrees", "repo", "task")

	run(t, base, "init", "-q", "-b", "main", "repo")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	run(t, repo, "add", "README")
	run(t, repo, "commit", "-q", "-m", "init")
	run(t, repo, "worktree", "add", "-q", "-b", "agent/task", worktree)
	return repo, worktree
}

func commitIn(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	run(t, dir, "add", name)
	run(t, dir, "commit", "-q", "-m", name)
}

func TestCleanWorktree(t *testing.T) {
	repo, worktree := repoWithWorktree(t)

	got, err := gitfacts.Read(context.Background(), repo, worktree)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Branch != "agent/task" || got.Dirty || got.Unmerged != 0 || got.Missing {
		t.Errorf("Read = %+v, want branch agent/task, clean, 0 unmerged", got)
	}
}

func TestDirtyWorktree(t *testing.T) {
	repo, worktree := repoWithWorktree(t)
	if err := os.WriteFile(filepath.Join(worktree, "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing scratch: %v", err)
	}

	got, err := gitfacts.Read(context.Background(), repo, worktree)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !got.Dirty {
		t.Errorf("Read = %+v, want dirty (untracked files count)", got)
	}
}

func TestUnmergedCommits(t *testing.T) {
	repo, worktree := repoWithWorktree(t)
	commitIn(t, worktree, "one")
	commitIn(t, worktree, "two")

	got, err := gitfacts.Read(context.Background(), repo, worktree)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Unmerged != 2 {
		t.Errorf("Unmerged = %d, want 2", got.Unmerged)
	}
	if got.Dirty {
		t.Errorf("Read = %+v, want clean after committing", got)
	}
}

func TestMergedBranchCountsZero(t *testing.T) {
	repo, worktree := repoWithWorktree(t)
	commitIn(t, worktree, "one")
	run(t, repo, "merge", "-q", "--ff-only", "agent/task")

	got, err := gitfacts.Read(context.Background(), repo, worktree)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Unmerged != 0 {
		t.Errorf("Unmerged = %d, want 0 once merged", got.Unmerged)
	}
}

func TestMissingWorktree(t *testing.T) {
	repo, worktree := repoWithWorktree(t)
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatalf("removing worktree: %v", err)
	}

	got, err := gitfacts.Read(context.Background(), repo, worktree)
	if err != nil {
		t.Fatalf("Read on a vanished worktree returned an error: %v", err)
	}
	if !got.Missing || got.Branch != "" {
		t.Errorf("Read = %+v, want Missing with no branch", got)
	}
}

func TestNotARepository(t *testing.T) {
	dir := t.TempDir()

	_, err := gitfacts.Read(context.Background(), "", dir)
	if err == nil {
		t.Fatal("Read succeeded outside a git repository")
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error = %q, does not name the path", err)
	}
}

func TestUnknownRepositorySkipsTheCount(t *testing.T) {
	_, worktree := repoWithWorktree(t)

	got, err := gitfacts.Read(context.Background(), "", worktree)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Branch != "agent/task" || got.Unmerged != 0 {
		t.Errorf("Read = %+v, want branch with no count", got)
	}
}
