package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartCreatesTheWorktreeBranchAndSandbox(t *testing.T) {
	r, stub, repo := newRunner(t)

	result, err := r.Start(context.Background(), StartOpts{
		Repo: repo, Name: "auth", AppName: "personal",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if result.Refused() {
		t.Fatalf("Start refused: %+v", result)
	}

	worktree := filepath.Join(r.WorktreeRoot, filepath.Base(repo), "auth")
	if !isDir(worktree) {
		t.Errorf("worktree %s was not created", worktree)
	}
	if !branchExists(context.Background(), repo, "agent/auth") {
		t.Error("branch agent/auth was not created")
	}
	if !strings.Contains(result.Stdout, "Created auth on agent/auth") {
		t.Errorf("stdout = %q", result.Stdout)
	}

	// Asked of git rather than assembled: on macOS the temp dir is reached through a
	// symlink, and git reports the physical path.
	git, err := gitDir(context.Background(), repo)
	if err != nil {
		t.Fatalf("gitDir: %v", err)
	}
	want := "--app-name personal create --name auth --publish 4096 opencode " + worktree + " " + git
	if got := callMatching(t, stub, "create"); got != want {
		t.Errorf("create argv =\n  %q\nwant\n  %q", got, want)
	}
}

// The regression this guards: keying the publish default off the runner's default
// agent instead of the resolved one silently drops OpenCode's port.
func TestStartResolvesTheAgentAndItsPortTogether(t *testing.T) {
	tests := []struct {
		name         string
		runnerAgent  string
		requested    string
		extra        []string
		wantAgent    string
		wantPublish  bool
		wantForwards []string
	}{
		{
			name:        "defaults to opencode and publishes 4096",
			wantAgent:   "opencode",
			wantPublish: true,
		},
		{
			name:        "--agent selects the agent and skips the opencode port",
			requested:   "claude",
			wantAgent:   "claude",
			wantPublish: false,
		},
		{
			name:        "inherits the runner's default agent",
			runnerAgent: "copilot",
			wantAgent:   "copilot",
			wantPublish: false,
		},
		{
			name:        "--agent overrides the default and restores the opencode port",
			runnerAgent: "claude",
			requested:   "opencode",
			wantAgent:   "opencode",
			wantPublish: true,
		},
		{
			name:         "does not double-publish 4096 when it is already published",
			extra:        []string{"--publish", "14096:4096"},
			wantAgent:    "opencode",
			wantPublish:  false,
			wantForwards: []string{"--publish", "14096:4096"},
		},
		{
			name:         "forwards everything else in order",
			requested:    "claude",
			extra:        []string{"--cpus", "4", "--memory", "8g"},
			wantAgent:    "claude",
			wantForwards: []string{"--cpus", "4", "--memory", "8g"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, stub, repo := newRunner(t)
			r.Agent = tt.runnerAgent

			if _, err := r.Start(context.Background(), StartOpts{
				Repo: repo, Name: "auth", AppName: "personal",
				Agent: tt.requested, Extra: tt.extra,
			}); err != nil {
				t.Fatalf("Start: %v", err)
			}

			call := callMatching(t, stub, "create")
			if !strings.Contains(call, " "+tt.wantAgent+" ") {
				t.Errorf("call %q does not create with agent %q", call, tt.wantAgent)
			}
			publishes := strings.Contains(call, "--publish 4096 ")
			if publishes != tt.wantPublish {
				t.Errorf("call %q: auto-published 4096 = %v, want %v", call, publishes, tt.wantPublish)
			}
			if len(tt.wantForwards) > 0 && !strings.Contains(call, strings.Join(tt.wantForwards, " ")) {
				t.Errorf("call %q does not forward %v in order", call, tt.wantForwards)
			}
		})
	}
}

func TestStartUnwindsAFailedCreate(t *testing.T) {
	r, stub, repo := newRunner(t)
	stub.FailCommand(t, "create", 1, "Error: sandbox limit reached")

	result, err := r.Start(context.Background(), StartOpts{
		Repo: repo, Name: "auth", AppName: "personal",
	})
	if err != nil {
		t.Fatalf("Start returned an error rather than a refusal: %v", err)
	}
	if !result.Refused() {
		t.Fatalf("Start succeeded although sbx create failed: %+v", result)
	}

	worktree := filepath.Join(r.WorktreeRoot, filepath.Base(repo), "auth")
	if exists(worktree) {
		t.Errorf("worktree %s survived a failed create", worktree)
	}
	if branchExists(context.Background(), repo, "agent/auth") {
		t.Error("branch agent/auth survived a failed create")
	}
	if !strings.Contains(result.Stderr, "sandbox limit reached") {
		t.Errorf("stderr = %q, want sbx's own reason", result.Stderr)
	}
	if !strings.Contains(result.Stderr, "removed worktree") {
		t.Errorf("stderr = %q, want it to say the worktree was removed", result.Stderr)
	}
}

func TestStartResumesAnExistingWorktree(t *testing.T) {
	r, stub, repo := newRunner(t)
	ctx := context.Background()

	if _, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	before := len(stub.Calls(t))

	result, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"})
	if err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if result.Refused() {
		t.Fatalf("resuming refused: %+v", result)
	}

	after := stub.Calls(t)[before:]
	if len(after) != 1 || after[0] != "--app-name personal exec auth true" {
		t.Errorf("resume calls = %v, want a single liveness probe", after)
	}
}

func TestStartRefusals(t *testing.T) {
	ctx := context.Background()

	t.Run("a file where the worktree should go", func(t *testing.T) {
		r, stub, repo := newRunner(t)
		worktree := filepath.Join(r.WorktreeRoot, filepath.Base(repo), "auth")
		if err := os.MkdirAll(filepath.Dir(worktree), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, worktree, "not a directory")

		result, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if !strings.Contains(result.Stderr, "worktree path already exists") {
			t.Errorf("stderr = %q", result.Stderr)
		}
		noCallMatching(t, stub, "create")
	})

	t.Run("a branch with no worktree", func(t *testing.T) {
		r, stub, repo := newRunner(t)
		git(t, repo, "branch", "agent/auth")

		result, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if !strings.Contains(result.Stderr, "branch already exists without its expected worktree") {
			t.Errorf("stderr = %q", result.Stderr)
		}
		noCallMatching(t, stub, "create")
	})

	t.Run("a name that cannot be a branch", func(t *testing.T) {
		r, stub, repo := newRunner(t)

		result, err := r.Start(ctx, StartOpts{Repo: repo, Name: "a", AppName: "personal"})
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if result.ExitCode != 2 {
			t.Errorf("exit code = %d, want 2 for misuse", result.ExitCode)
		}
		noCallMatching(t, stub, "create")
	})
}

// A missing scope or worktree root is a misconfiguration of sluss itself, not a
// refusal to show the user, so it comes back as an error.
func TestStartRequiresScopeAndWorktreeRoot(t *testing.T) {
	r, _, repo := newRunner(t)
	ctx := context.Background()

	if _, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth"}); err == nil {
		t.Error("Start accepted an empty app-name")
	}
	r.WorktreeRoot = ""
	if _, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"}); err == nil {
		t.Error("Start accepted an empty worktree root")
	}
}

func TestRepoRootFindsThePrimaryCheckoutFromAWorktree(t *testing.T) {
	r, _, repo := newRunner(t)
	ctx := context.Background()

	if _, err := r.Start(ctx, StartOpts{Repo: repo, Name: "auth", AppName: "personal"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	worktree := filepath.Join(r.WorktreeRoot, filepath.Base(repo), "auth")

	fromWorktree, err := RepoRoot(ctx, worktree)
	if err != nil {
		t.Fatalf("RepoRoot from a worktree: %v", err)
	}
	fromRepo, err := RepoRoot(ctx, repo)
	if err != nil {
		t.Fatalf("RepoRoot from the checkout: %v", err)
	}
	if fromWorktree != fromRepo {
		t.Errorf("RepoRoot from worktree = %q, from checkout = %q; want the same primary checkout", fromWorktree, fromRepo)
	}
}

func TestRepoRootRefusesOutsideARepository(t *testing.T) {
	dir := t.TempDir()
	_, err := RepoRoot(context.Background(), dir)
	if err == nil {
		t.Fatal("RepoRoot succeeded outside a repository")
	}
	if KindOf(err) != Refused {
		t.Errorf("kind = %v, want Refused", KindOf(err))
	}
}

func TestRepoNameDropsADotGitSuffix(t *testing.T) {
	if got := RepoName("/src/mercurius.git"); got != "mercurius" {
		t.Errorf("RepoName = %q, want mercurius", got)
	}
	if got := RepoName("/src/mercurius"); got != "mercurius" {
		t.Errorf("RepoName = %q, want mercurius", got)
	}
}
