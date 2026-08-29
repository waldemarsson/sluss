package lifecycle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// git runs a git command in dir and fails the test if it does not succeed.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes a real git repository with one commit on "main". The lifecycle
// tests drive real git rather than a fake: worktree add, branch -d and
// merge-base --is-ancestor are the behaviour under test, not an implementation
// detail worth stubbing.
func newRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "sluss tests")
	t.Setenv("GIT_AUTHOR_EMAIL", "tests@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "sluss tests")
	t.Setenv("GIT_COMMITTER_EMAIL", "tests@example.invalid")

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeTestFile(t, filepath.Join(dir, "README.md"), "seed\n")
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-m", "seed")
	return dir
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// newRunner wires a Runner to a stubbed sbx and a fresh worktree root.
func newRunner(t *testing.T) (*Runner, *sbxstub.Stub, string) {
	t.Helper()
	repo := newRepo(t)
	stub := sbxstub.Install(t, sbxstub.Fixture)
	r := &Runner{
		Sbx:          &sbx.Client{Bin: stub.Bin},
		WorktreeRoot: t.TempDir(),
	}
	return r, stub, repo
}

// callMatching returns the one recorded sbx call containing want, or fails.
func callMatching(t *testing.T, stub *sbxstub.Stub, want string) string {
	t.Helper()
	for _, call := range stub.Calls(t) {
		if strings.Contains(call, want) {
			return call
		}
	}
	t.Fatalf("no sbx call containing %q; calls = %v", want, stub.Calls(t))
	return ""
}

func noCallMatching(t *testing.T, stub *sbxstub.Stub, unwanted string) {
	t.Helper()
	for _, call := range stub.Calls(t) {
		if strings.Contains(call, unwanted) {
			t.Fatalf("unexpected sbx call %q", call)
		}
	}
}
