package fleet_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

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

// checkout builds a primary repository with one commit and a worktree per name.
func checkout(t *testing.T, base, name string, tasks ...string) (repo string, worktrees map[string]string) {
	t.Helper()
	repo = filepath.Join(base, name)
	git(t, base, "init", "-q", "-b", "main", name)
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing README: %v", err)
	}
	git(t, repo, "add", "README")
	git(t, repo, "commit", "-q", "-m", "init")

	worktrees = make(map[string]string, len(tasks))
	for _, task := range tasks {
		path := filepath.Join(base, "worktrees", name, task)
		git(t, repo, "worktree", "add", "-q", "-b", "agent/"+task, path)
		worktrees[task] = path
	}
	return repo, worktrees
}

// fixture renders one sbx listing with the two workspaces scripts/sluss mounts.
func fixture(repo string, entries ...[3]string) string {
	var rows []string
	for _, e := range entries {
		name, status, worktree := e[0], e[1], e[2]
		ports := ""
		if status == "running" {
			ports = `"ports": [{"host_ip":"127.0.0.1","host_port":49155,"sandbox_port":4096,"protocol":"tcp"}],`
		}
		rows = append(rows, fmt.Sprintf(
			`{"name": %q, "id": "id-%s", "agent": "opencode", "status": %q, %s "workspaces": [%q, %q]}`,
			name, name, status, ports, worktree, filepath.Join(repo, ".git")))
	}
	return `{"sandboxes": [` + strings.Join(rows, ",") + `]}`
}

func poller(t *testing.T, stub *sbxstub.Stub, cfg *config.Config) *fleet.Poller {
	t.Helper()
	return fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 10*time.Millisecond)
}

func TestPollMergesScopesAndGitFacts(t *testing.T) {
	base := t.TempDir()
	repoA, wtA := checkout(t, base, "alpha", "web")
	repoB, wtB := checkout(t, base, "beta", "auth")

	// alpha/web is dirty; beta/auth has one unmerged commit.
	if err := os.WriteFile(filepath.Join(wtA["web"], "scratch"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtB["auth"], "work"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing work: %v", err)
	}
	git(t, wtB["auth"], "add", "work")
	git(t, wtB["auth"], "commit", "-q", "-m", "work")

	stub := sbxstub.Install(t, sbxstub.Empty)
	stub.ScopeFixture(t, "personal", fixture(repoA, [3]string{"web", "running", wtA["web"]}))
	stub.ScopeFixture(t, "work", fixture(repoB, [3]string{"auth", "stopped", wtB["auth"]}))

	cfg := &config.Config{AppNames: []string{"personal", "work"}, WorktreeRoot: filepath.Join(base, "worktrees")}
	snap := poller(t, stub, cfg).Poll(context.Background())

	if len(snap.ScopeErrors) != 0 {
		t.Fatalf("scope errors: %+v", snap.ScopeErrors)
	}
	if len(snap.Repos) != 2 {
		t.Fatalf("got %d repo groups, want 2", len(snap.Repos))
	}
	if snap.Repos[0].Name != "alpha" || snap.Repos[1].Name != "beta" {
		t.Errorf("groups = %q, %q; want alpha, beta sorted", snap.Repos[0].Name, snap.Repos[1].Name)
	}

	web, ok := snap.Find("personal", "web")
	if !ok {
		t.Fatal("personal/web missing from the snapshot")
	}
	if web.Repo != repoA || web.Worktree != wtA["web"] || web.Branch != "agent/web" {
		t.Errorf("web = %+v", web)
	}
	if !web.Dirty || web.Unmerged != 0 || web.Missing {
		t.Errorf("web git facts = %+v, want dirty with no unmerged commits", web)
	}
	if web.WebPort != 49155 {
		t.Errorf("web WebPort = %d, want 49155", web.WebPort)
	}

	auth, ok := snap.Find("work", "auth")
	if !ok {
		t.Fatal("work/auth missing from the snapshot")
	}
	if auth.Unmerged != 1 || auth.Dirty {
		t.Errorf("auth git facts = %+v, want 1 unmerged and clean", auth)
	}
	if auth.WebPort != 0 {
		t.Errorf("stopped sandbox has WebPort %d, want 0", auth.WebPort)
	}
}

func TestNamesAreScopeQualified(t *testing.T) {
	base := t.TempDir()
	repo, wt := checkout(t, base, "alpha", "web")

	stub := sbxstub.Install(t, fixture(repo, [3]string{"web", "running", wt["web"]}))
	cfg := &config.Config{AppNames: []string{"personal", "work"}, WorktreeRoot: filepath.Join(base, "worktrees")}

	snap := poller(t, stub, cfg).Poll(context.Background())

	if len(snap.Repos) != 1 || len(snap.Repos[0].Sandboxes) != 2 {
		t.Fatalf("want the same name once per scope, got %+v", snap.Repos)
	}
	for _, scope := range []string{"personal", "work"} {
		if _, ok := snap.Find(scope, "web"); !ok {
			t.Errorf("%s/web missing", scope)
		}
	}
}

func TestRemovedSandboxDisappears(t *testing.T) {
	base := t.TempDir()
	repo, wt := checkout(t, base, "alpha", "web")

	stub := sbxstub.Install(t, sbxstub.Empty)
	stub.ScopeFixture(t, "personal", fixture(repo, [3]string{"web", "running", wt["web"]}))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: filepath.Join(base, "worktrees")}
	p := poller(t, stub, cfg)

	if _, ok := p.Poll(context.Background()).Find("personal", "web"); !ok {
		t.Fatal("personal/web missing from the first snapshot")
	}

	stub.ScopeFixture(t, "personal", sbxstub.Empty) // an "sbx rm -f web" by hand
	snap := p.Poll(context.Background())
	if _, ok := snap.Find("personal", "web"); ok {
		t.Error("personal/web survived its removal")
	}
	if len(snap.Repos) != 0 {
		t.Errorf("repos = %+v, want none", snap.Repos)
	}
}

func TestMissingWorktreeIsMarkedNotFatal(t *testing.T) {
	base := t.TempDir()
	repo, wt := checkout(t, base, "alpha", "web")
	if err := os.RemoveAll(wt["web"]); err != nil {
		t.Fatalf("removing worktree: %v", err)
	}

	stub := sbxstub.Install(t, fixture(repo, [3]string{"web", "running", wt["web"]}))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: filepath.Join(base, "worktrees")}

	got, ok := poller(t, stub, cfg).Poll(context.Background()).Find("personal", "web")
	if !ok {
		t.Fatal("a sandbox with no worktree was dropped from the fleet")
	}
	if !got.Missing {
		t.Errorf("got %+v, want Missing", got)
	}
}

func TestFailingScopeLeavesOthersIntact(t *testing.T) {
	base := t.TempDir()
	repo, wt := checkout(t, base, "alpha", "web")

	stub := sbxstub.Install(t, sbxstub.Empty)
	stub.ScopeFixture(t, "personal", fixture(repo, [3]string{"web", "running", wt["web"]}))
	stub.FailScope(t, "work", "Error: not logged in")
	cfg := &config.Config{AppNames: []string{"personal", "work"}, WorktreeRoot: filepath.Join(base, "worktrees")}

	snap := poller(t, stub, cfg).Poll(context.Background())

	if _, ok := snap.Find("personal", "web"); !ok {
		t.Error("a healthy scope lost its sandboxes because another scope failed")
	}
	if len(snap.ScopeErrors) != 1 || snap.ScopeErrors[0].Scope != "work" {
		t.Fatalf("scope errors = %+v, want one for work", snap.ScopeErrors)
	}
	if !strings.Contains(snap.ScopeErrors[0].Error, "login") {
		t.Errorf("scope error = %q, want the mapped hint", snap.ScopeErrors[0].Error)
	}
}

func TestSlowPollSkipsTicksInsteadOfStacking(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	stub.Slow(t, "0.1")
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}
	p := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 5*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	_ = p.Run(ctx)

	// Each poll takes ~100ms, so 350ms allows at most 4 — a stacking poller would
	// queue a poll per 5ms tick and run far more.
	if calls := len(stub.Calls(t)); calls > 4 {
		t.Errorf("%d polls in 350ms with a 100ms poll; ticks are stacking", calls)
	}
}

func TestSubscribeDeliversAndUnsubscribes(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}
	p := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 5*time.Millisecond)

	ch, unsubscribe := p.Subscribe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = p.Run(ctx) }()

	select {
	case snap := <-ch:
		if snap == nil {
			t.Fatal("received a nil snapshot")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot delivered to the subscriber")
	}

	unsubscribe()
	unsubscribe() // idempotent: a double unsubscribe must not panic
	cancel()

	// The channel is closed, so a receive returns the zero value immediately rather
	// than blocking or delivering more snapshots.
	select {
	case _, open := <-ch:
		if open {
			t.Error("snapshot delivered after unsubscribing")
		}
	case <-time.After(time.Second):
		t.Error("channel neither closed nor delivering")
	}
}

func TestSlowSubscriberDoesNotStallThePoller(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}
	p := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Millisecond)

	_, unsubscribe := p.Subscribe() // never drained
	defer unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = p.Run(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller blocked on a subscriber that never reads")
	}
	if len(stub.Calls(t)) < 2 {
		t.Errorf("only %d polls; the loop was not running freely", len(stub.Calls(t)))
	}
}

func TestCurrentIsNilBeforeTheFirstPoll(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}

	if snap := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Second).Current(); snap != nil {
		t.Errorf("Current() = %+v before any poll, want nil", snap)
	}
	var empty *fleet.Snapshot
	if _, ok := empty.Find("personal", "web"); ok {
		t.Error("Find on a nil snapshot reported a hit")
	}
}
