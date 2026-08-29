package script_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbxstub"
	"github.com/waldemarsson/sluss/internal/script"
)

// realScript is the repository's own scripts/sluss — the point of these tests is
// that the GUI runs exactly that, not a Go reimplementation of it.
func realScript(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	path := filepath.Join(wd, "..", "..", "scripts", "sluss")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("locating scripts/sluss: %v", err)
	}
	return path
}

func git(t *testing.T, dir string, args ...string) {
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

// environment gives a repository, a worktree root and a stubbed sbx.
func environment(t *testing.T) (runner *script.Runner, repo string, stub *sbxstub.Stub) {
	t.Helper()
	base := t.TempDir()
	repo = filepath.Join(base, "mercurius")
	git(t, base, "init", "-q", "-b", "main", "mercurius")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	git(t, repo, "add", "README")
	git(t, repo, "commit", "-q", "-m", "init")

	stub = sbxstub.Install(t, sbxstub.Empty)
	runner = &script.Runner{Script: realScript(t), WorktreeRoot: filepath.Join(base, "worktrees")}
	return runner, repo, stub
}

func TestStartCreatesTheWorktreeAndSandbox(t *testing.T) {
	runner, repo, stub := environment(t)

	result, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if result.Refused() {
		t.Fatalf("Start refused: %+v", result)
	}

	worktree := filepath.Join(runner.WorktreeRoot, "mercurius", "web")
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree not created at %s: %v", worktree, err)
	}
	branches := exec.Command("git", "-C", repo, "branch", "--list", "agent/web")
	if out, _ := branches.Output(); !strings.Contains(string(out), "agent/web") {
		t.Errorf("branch agent/web not created: %q", out)
	}

	// The scope reaches sbx as --app-name, the agent as the create argument, and
	// the worktree root through the paths the script mounts.
	want := "--app-name personal create --name web --publish 4096 opencode " +
		worktree + " " + filepath.Join(repo, ".git")
	calls := stub.Calls(t)
	found := false
	for _, call := range calls {
		if call == want {
			found = true
		}
	}
	if !found {
		t.Errorf("sbx calls = %v,\nwant one exactly %q", calls, want)
	}
}

func TestStartHonoursTheChosenAgent(t *testing.T) {
	runner, repo, stub := environment(t)

	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "claude", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	calls := strings.Join(stub.Calls(t), "\n")
	if !strings.Contains(calls, " claude ") {
		t.Errorf("sbx calls = %q, want the sandbox created with claude", calls)
	}
	// Only OpenCode gets the web port published.
	if strings.Contains(calls, "--publish 4096") {
		t.Errorf("sbx calls = %q, want no OpenCode port for a claude sandbox", calls)
	}
}

func TestStartForwardsExtraArguments(t *testing.T) {
	runner, repo, stub := environment(t)

	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
		Extra: []string{"--cpus", "4"},
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if calls := strings.Join(stub.Calls(t), "\n"); !strings.Contains(calls, "--cpus 4") {
		t.Errorf("sbx calls = %q, want the extra arguments forwarded", calls)
	}
}

func TestStartRunsInTheRequestedRepository(t *testing.T) {
	runner, repo, _ := environment(t)
	other := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("creating %s: %v", other, err)
	}

	// Nothing about the process's own working directory decides the repository:
	// the worktree lands under the repo that was asked for.
	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runner.WorktreeRoot, "mercurius", "web")); err != nil {
		t.Errorf("worktree not under the requested repository: %v", err)
	}
}

func TestStartRejectsAnInvalidName(t *testing.T) {
	runner, repo, _ := environment(t)

	result, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "a", Agent: "opencode", AppName: "personal",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !result.Refused() {
		t.Fatal("the script accepted a one-character name")
	}
	if !strings.Contains(result.Stderr, "NAME must be") {
		t.Errorf("stderr = %q, want the script's own message", result.Stderr)
	}
}

func TestDestroyRefusesDirtyWork(t *testing.T) {
	runner, repo, stub := environment(t)
	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	worktree := filepath.Join(runner.WorktreeRoot, "mercurius", "web")
	if err := os.WriteFile(filepath.Join(worktree, "unsaved"), []byte("work"), 0o644); err != nil {
		t.Fatalf("writing unsaved work: %v", err)
	}

	result, err := runner.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !result.Refused() {
		t.Fatal("Destroy proceeded with uncommitted changes")
	}
	if !strings.Contains(result.Stderr, "uncommitted changes") {
		t.Errorf("stderr = %q, want the script's refusal", result.Stderr)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Errorf("worktree removed despite the refusal: %v", err)
	}
	if strings.Contains(strings.Join(stub.Calls(t), "\n"), "rm --force") {
		t.Error("sbx rm ran despite the refusal")
	}
}

func TestDestroyRefusesUnmergedCommits(t *testing.T) {
	runner, repo, _ := environment(t)
	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	worktree := filepath.Join(runner.WorktreeRoot, "mercurius", "web")
	if err := os.WriteFile(filepath.Join(worktree, "feature"), []byte("work"), 0o644); err != nil {
		t.Fatalf("writing feature: %v", err)
	}
	git(t, worktree, "add", "feature")
	git(t, worktree, "commit", "-q", "-m", "feature")

	result, err := runner.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if !result.Refused() {
		t.Fatal("Destroy discarded unmerged commits")
	}
	if !strings.Contains(result.Stderr, "not merged") {
		t.Errorf("stderr = %q, want the unmerged refusal", result.Stderr)
	}
}

func TestDestroyRemovesCleanMergedWork(t *testing.T) {
	runner, repo, stub := environment(t)
	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	worktree := filepath.Join(runner.WorktreeRoot, "mercurius", "web")

	result, err := runner.Destroy(context.Background(), repo, "personal", "web", false)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Refused() {
		t.Fatalf("Destroy refused clean merged work: %+v", result)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree still present: %v", err)
	}
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "agent/web").Output()
	if strings.Contains(string(out), "agent/web") {
		t.Errorf("branch survived destroy: %q", out)
	}
	if !strings.Contains(strings.Join(stub.Calls(t), "\n"), "--app-name personal rm --force web") {
		t.Errorf("sbx calls = %v, want the sandbox removed", stub.Calls(t))
	}
}

// recorder replaces the script with something that just logs its argv, so the
// arguments sluss chooses can be asserted directly.
func recorder(t *testing.T) (path, log string) {
	t.Helper()
	dir := t.TempDir()
	path = filepath.Join(dir, "sluss")
	log = filepath.Join(dir, "argv")
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + log + "\"\nprintf 'env %s %s %s\\n' \"$SLUSS_APP_NAME\" \"$SLUSS_WORKTREE_ROOT\" \"${SLUSS_AGENT:-none}\" >> \"" + log + "\"\nprintf '%s\\n' \"$PWD\" >> \"" + log + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing recorder: %v", err)
	}
	return path, log
}

// The default is still the unforced call: force is opt-in per destroy, so a caller
// that forgets the argument cannot discard work by omission.
func TestDestroyArgv(t *testing.T) {
	for _, tc := range []struct {
		name  string
		force bool
		want  string
	}{
		{"unforced", false, "destroy web"},
		{"forced", true, "destroy web --force"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, log := recorder(t)
			repo := t.TempDir()
			runner := &script.Runner{Script: path, WorktreeRoot: "/w"}

			if _, err := runner.Destroy(context.Background(), repo, "personal", "web", tc.force); err != nil {
				t.Fatalf("Destroy: %v", err)
			}

			recorded, err := os.ReadFile(log)
			if err != nil {
				t.Fatalf("reading the recording: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
			if lines[0] != tc.want {
				t.Errorf("argv = %q, want %q", lines[0], tc.want)
			}
			if lines[1] != "env personal /w none" {
				t.Errorf("environment = %q, want the scope and worktree root", lines[1])
			}
			if lines[2] != repo {
				t.Errorf("working directory = %q, want %q", lines[2], repo)
			}
		})
	}
}

// Start and Stop must not have grown a --force of their own.
func TestStartAndStopArgvUnchanged(t *testing.T) {
	path, log := recorder(t)
	repo := t.TempDir()
	runner := &script.Runner{Script: path, WorktreeRoot: "/w"}

	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := runner.Stop(context.Background(), repo, "personal", "web"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	recorded, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("reading the recording: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if lines[0] != "start web" {
		t.Errorf("start argv = %q, want \"start web\"", lines[0])
	}
	if lines[3] != "stop web" {
		t.Errorf("stop argv = %q, want \"stop web\"", lines[3])
	}
}

// The end-to-end counterpart to TestDestroyArgv's forced case: against the real
// script, --force must actually discard the dirty worktree and its branch.
func TestDestroyForceRemovesDirtyWork(t *testing.T) {
	runner, repo, stub := environment(t)
	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: repo, Name: "web", Agent: "opencode", AppName: "personal",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	worktree := filepath.Join(runner.WorktreeRoot, "mercurius", "web")
	if err := os.WriteFile(filepath.Join(worktree, "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatalf("dirtying the worktree: %v", err)
	}

	result, err := runner.Destroy(context.Background(), repo, "personal", "web", true)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if result.Refused() {
		t.Fatalf("--force still refused dirty work: %+v", result)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree survived a forced destroy: %v", err)
	}
	out, _ := exec.Command("git", "-C", repo, "branch", "--list", "agent/web").Output()
	if strings.Contains(string(out), "agent/web") {
		t.Errorf("branch survived a forced destroy: %q", out)
	}
	if !strings.Contains(strings.Join(stub.Calls(t), "\n"), "--app-name personal rm --force web") {
		t.Errorf("sbx calls = %v, want the sandbox removed", stub.Calls(t))
	}
}

func TestStartCarriesTheAgentInTheEnvironment(t *testing.T) {
	path, log := recorder(t)
	runner := &script.Runner{Script: path, WorktreeRoot: "/w"}

	if _, err := runner.Start(context.Background(), script.StartOpts{
		Repo: t.TempDir(), Name: "web", Agent: "claude", AppName: "work",
	}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	recorded, _ := os.ReadFile(log)
	if !strings.Contains(string(recorded), "env work /w claude") {
		t.Errorf("recording = %q, want SLUSS_AGENT=claude", recorded)
	}
}

func TestMissingScript(t *testing.T) {
	runner := &script.Runner{Script: "/nonexistent/sluss", WorktreeRoot: "/w"}

	_, err := runner.Stop(context.Background(), t.TempDir(), "personal", "web")
	if err == nil {
		t.Fatal("Stop succeeded with no script")
	}
	if !strings.Contains(err.Error(), "/nonexistent/sluss") {
		t.Errorf("error = %q, does not name the script", err)
	}
}

func TestScopeIsRequired(t *testing.T) {
	path, _ := recorder(t)
	runner := &script.Runner{Script: path, WorktreeRoot: "/w"}

	if _, err := runner.Destroy(context.Background(), t.TempDir(), "", "web", false); err == nil {
		t.Fatal("Destroy ran without an app-name")
	}
}

func TestResolveScriptPrefersTheEnvironment(t *testing.T) {
	path, _ := recorder(t)
	t.Setenv("SLUSS_SCRIPT", path)

	got, err := script.ResolveScript()
	if err != nil {
		t.Fatalf("ResolveScript: %v", err)
	}
	if got != path {
		t.Errorf("ResolveScript() = %q, want %q", got, path)
	}
}
