package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// started creates a sandbox's worktree and branch and returns the worktree path.
func started(t *testing.T, r *Runner, repo, name string) string {
	t.Helper()
	if _, err := r.Start(context.Background(), StartOpts{
		Repo: repo, Name: name, AppName: "personal",
	}); err != nil {
		t.Fatalf("Start %s: %v", name, err)
	}
	return WorktreePath(r.WorktreeRoot, repo, name)
}

func TestDestroyRemovesACleanMergedTask(t *testing.T) {
	r, stub, repo := newRunner(t)
	worktree := started(t, r, repo, "web")

	result, err := r.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Refused() {
		t.Fatalf("Destroy refused a clean, merged worktree: %+v", result)
	}
	if exists(worktree) {
		t.Errorf("worktree %s survived", worktree)
	}
	if branchExists(context.Background(), repo, "agent/web") {
		t.Error("branch agent/web survived")
	}
	if got := callMatching(t, stub, " rm "); got != "--app-name personal rm --force web" {
		t.Errorf("rm argv = %q", got)
	}
}

func TestDestroyRefusesUncommittedWork(t *testing.T) {
	r, stub, repo := newRunner(t)
	worktree := started(t, r, repo, "web")
	writeTestFile(t, filepath.Join(worktree, "scratch.txt"), "unsaved\n")

	result, err := r.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !result.Refused() {
		t.Fatal("Destroy removed a dirty worktree")
	}
	if !strings.Contains(result.Stderr, "agent/web has uncommitted changes; commit them or use --force") {
		t.Errorf("stderr = %q", result.Stderr)
	}
	if !isDir(worktree) {
		t.Error("a refused destroy removed the worktree anyway")
	}
	// The refusal must come before sbx is touched, or a refused destroy would still
	// have thrown the sandbox away.
	noCallMatching(t, stub, " rm ")
}

func TestDestroyRefusesUnmergedCommits(t *testing.T) {
	r, stub, repo := newRunner(t)
	worktree := started(t, r, repo, "web")
	writeTestFile(t, filepath.Join(worktree, "feature.txt"), "work\n")
	git(t, worktree, "add", "feature.txt")
	git(t, worktree, "commit", "-m", "feature")

	result, err := r.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !result.Refused() {
		t.Fatal("Destroy removed an unmerged branch")
	}
	if !strings.Contains(result.Stderr, "has commits not merged into main") {
		t.Errorf("stderr = %q, want the primary branch named", result.Stderr)
	}
	if !branchExists(context.Background(), repo, "agent/web") {
		t.Error("a refused destroy deleted the branch anyway")
	}
	noCallMatching(t, stub, " rm ")
}

func TestDestroyForceDiscardsBoth(t *testing.T) {
	for _, tt := range []struct {
		name  string
		dirty bool
	}{
		{"uncommitted work", true},
		{"unmerged commits", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, stub, repo := newRunner(t)
			worktree := started(t, r, repo, "web")
			writeTestFile(t, filepath.Join(worktree, "work.txt"), "work\n")
			if !tt.dirty {
				git(t, worktree, "add", "work.txt")
				git(t, worktree, "commit", "-m", "unmerged")
			}

			result, err := r.Destroy(context.Background(), repo, "personal", "web", true)
			if err != nil {
				t.Fatalf("Destroy: %v", err)
			}
			if result.Refused() {
				t.Fatalf("forced destroy refused: %+v", result)
			}
			if exists(worktree) {
				t.Errorf("worktree %s survived a forced destroy", worktree)
			}
			if branchExists(context.Background(), repo, "agent/web") {
				t.Error("branch survived a forced destroy")
			}
			callMatching(t, stub, "rm --force web")
		})
	}
}

func TestDestroyRefusesAMissingWorktree(t *testing.T) {
	r, stub, repo := newRunner(t)

	result, err := r.Destroy(context.Background(), repo, "personal", "ghost", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !result.Refused() {
		t.Fatal("Destroy accepted a task that does not exist")
	}
	if !strings.Contains(result.Stderr, "worktree not found") {
		t.Errorf("stderr = %q", result.Stderr)
	}
	noCallMatching(t, stub, " rm ")
}

func TestStop(t *testing.T) {
	r, stub, _ := newRunner(t)
	ctx := context.Background()

	result, err := r.Stop(ctx, "", "personal", "web", "auth")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if result.Refused() {
		t.Fatalf("Stop refused: %+v", result)
	}
	if got := callMatching(t, stub, "stop"); got != "--app-name personal stop web auth" {
		t.Errorf("stop argv = %q", got)
	}

	naming, err := r.Stop(ctx, "", "personal")
	if err != nil {
		t.Fatalf("Stop with no names: %v", err)
	}
	if naming.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2 for misuse", naming.ExitCode)
	}
}

func TestListShowsSandboxesAndWorktrees(t *testing.T) {
	r, stub, repo := newRunner(t)
	stub.Secrets(t, "NAME   STATUS\nweb    running\n")
	started(t, r, repo, "web")

	inRepo, err := r.List(context.Background(), repo, "personal")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !strings.Contains(inRepo.Stdout, "web    running") {
		t.Errorf("stdout = %q, want the sbx listing", inRepo.Stdout)
	}
	if !strings.Contains(inRepo.Stdout, "Git worktrees for "+repo) {
		t.Errorf("stdout = %q, want the worktree section", inRepo.Stdout)
	}

	// Outside a repository the worktree section is simply absent.
	outside, err := r.List(context.Background(), "", "personal")
	if err != nil {
		t.Fatalf("List outside a repository: %v", err)
	}
	if strings.Contains(outside.Stdout, "Git worktrees") {
		t.Errorf("stdout = %q, want no worktree section", outside.Stdout)
	}
}

func TestPathAsksNeitherGitNorSbx(t *testing.T) {
	r, stub, repo := newRunner(t)

	result, err := r.Path(repo, "auth")
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(r.WorktreeRoot, filepath.Base(repo), "auth") + "\n"
	if result.Stdout != want {
		t.Errorf("stdout = %q, want %q", result.Stdout, want)
	}
	if calls := stub.Calls(t); len(calls) != 0 {
		t.Errorf("path invoked sbx: %v", calls)
	}
}
