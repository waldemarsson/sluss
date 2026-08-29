package sbx_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// The argv these build is a contract with the retired shell script: the order of
// --name, the pass-through arguments, the agent and the workspaces is reproduced
// deliberately, because sbx's tolerance for reordering is unverified.
func TestLifecycleCommandsBuildTheScriptsArgv(t *testing.T) {
	tests := []struct {
		name string
		call func(*sbx.Client, context.Context) error
		want string
	}{
		{
			name: "create with the opencode publish default",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.Create(ctx, "personal", sbx.CreateOpts{
					Name:       "auth",
					Agent:      "opencode",
					Extra:      []string{"--publish", "4096"},
					Workspaces: []string{"/wt/auth", "/repo/.git"},
				})
			},
			want: "--app-name personal create --name auth --publish 4096 opencode /wt/auth /repo/.git",
		},
		{
			name: "create forwards extras in order, before the agent",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.Create(ctx, "work", sbx.CreateOpts{
					Name:       "web",
					Agent:      "claude",
					Extra:      []string{"--cpus", "4", "--memory", "8g"},
					Workspaces: []string{"/wt/web", "/repo/.git"},
				})
			},
			want: "--app-name work create --name web --cpus 4 --memory 8g claude /wt/web /repo/.git",
		},
		{
			name: "remove always passes sbx's own --force",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.Remove(ctx, "personal", "web")
			},
			want: "--app-name personal rm --force web",
		},
		{
			name: "stop takes every name in one call",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.Stop(ctx, "personal", "web", "auth")
			},
			want: "--app-name personal stop web auth",
		},
		{
			name: "exec probes liveness",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.Exec(ctx, "personal", "web", "true")
			},
			want: "--app-name personal exec web true",
		},
		{
			name: "settings set",
			call: func(c *sbx.Client, ctx context.Context) error {
				return c.SettingSet(ctx, "personal", "claude.remoteControl", "true")
			},
			want: "--app-name personal settings set claude.remoteControl true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := sbxstub.Install(t, sbxstub.Fixture)
			c := &sbx.Client{Bin: stub.Bin}

			if err := tt.call(c, context.Background()); err != nil {
				t.Fatalf("call: %v", err)
			}
			calls := stub.Calls(t)
			if len(calls) != 1 {
				t.Fatalf("calls = %v, want exactly one", calls)
			}
			if calls[0] != tt.want {
				t.Errorf("argv = %q\nwant     %q", calls[0], tt.want)
			}
		})
	}
}

func TestPortsReturnsWhatSbxPrinted(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Ports(t, "4096/tcp -> 127.0.0.1:49155\n")
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.Ports(context.Background(), "personal", "auth")
	if err != nil {
		t.Fatalf("Ports: %v", err)
	}
	if !strings.Contains(got, "49155") {
		t.Errorf("ports = %q, want the mapping", got)
	}
	if calls := stub.Calls(t); len(calls) != 1 || calls[0] != "--app-name personal ports auth" {
		t.Errorf("calls = %v", calls)
	}
}

func TestLifecycleCommandsAreScoped(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}
	ctx := context.Background()

	// Every one of these must refuse rather than run unscoped: an unscoped sbx call
	// would silently act on the default scope (AGENTS.md's chokepoint rule).
	checks := map[string]error{
		"create": c.Create(ctx, "", sbx.CreateOpts{Name: "a", Agent: "opencode"}),
		"rm":     c.Remove(ctx, "", "a"),
		"stop":   c.Stop(ctx, "", "a"),
		"exec":   c.Exec(ctx, "", "a", "true"),
		"set":    c.SettingSet(ctx, "", "k", "v"),
	}
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s accepted an empty app-name", name)
		}
	}
	if _, err := c.Ports(ctx, "", "a"); err == nil {
		t.Error("ports accepted an empty app-name")
	}
	if _, err := c.Interactive(ctx, "", sbx.Streams{}, "ls"); err == nil {
		t.Error("Interactive accepted an empty app-name")
	}
	if calls := stub.Calls(t); len(calls) != 0 {
		t.Errorf("sbx was invoked anyway: %v", calls)
	}
}

func TestFailedLifecycleCallsAreClassified(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Fail(t, 1, "Error: sandbox \"web\" already exists")
	c := &sbx.Client{Bin: stub.Bin}

	err := c.Create(context.Background(), "personal", sbx.CreateOpts{
		Name: "web", Agent: "opencode", Workspaces: []string{"/wt/web"},
	})
	if err == nil {
		t.Fatal("Create succeeded against a failing sbx")
	}
	if !strings.Contains(err.Error(), "already exists in scope personal") {
		t.Errorf("error = %q, want the mapped hint", err)
	}
}

func TestFailCommandTargetsOneCallOnly(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.FailCommand(t, "create", 1, "boom")
	c := &sbx.Client{Bin: stub.Bin}
	ctx := context.Background()

	if err := c.Create(ctx, "personal", sbx.CreateOpts{Name: "web", Agent: "opencode", Workspaces: []string{"/wt"}}); err == nil {
		t.Fatal("create succeeded although it was told to fail")
	}
	if _, err := c.List(ctx, "personal"); err != nil {
		t.Fatalf("an unrelated call also failed: %v", err)
	}
}

// A non-zero child is an answer, not a failure of sluss: "sluss exec NAME false"
// must exit 1 without claiming anything went wrong.
func TestInteractiveReturnsTheChildsExitCode(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Fail(t, 3, "")
	c := &sbx.Client{Bin: stub.Bin}

	var out, errOut bytes.Buffer
	code, err := c.InteractiveExec(context.Background(), "personal", "web",
		sbx.Streams{Stdout: &out, Stderr: &errOut}, "false")
	if err != nil {
		t.Fatalf("Interactive reported an error for a non-zero child: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestInteractiveErrorsWhenSbxCannotRun(t *testing.T) {
	c := &sbx.Client{Bin: "/nonexistent/sbx"}

	_, err := c.Interactive(context.Background(), "personal", sbx.Streams{}, "ls")
	if err == nil {
		t.Fatal("Interactive succeeded with no sbx binary")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want the install hint", err)
	}
}

func TestInteractiveBuildsTheScriptsArgv(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}
	ctx := context.Background()
	var sink bytes.Buffer
	streams := sbx.Streams{Stdout: &sink, Stderr: &sink}

	if _, err := c.InteractiveExec(ctx, "personal", "web", streams); err != nil {
		t.Fatalf("exec shell: %v", err)
	}
	if _, err := c.InteractiveExec(ctx, "personal", "web", streams, "git", "status"); err != nil {
		t.Fatalf("exec command: %v", err)
	}
	if _, err := c.InteractiveRun(ctx, "personal", "auth", streams, "--remote-control"); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := []string{
		"--app-name personal exec -it web sh",
		"--app-name personal exec -it web git status",
		"--app-name personal run --name auth -- --remote-control",
	}
	calls := stub.Calls(t)
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %d", calls, len(want))
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("call %d = %q, want %q", i, calls[i], w)
		}
	}
}
