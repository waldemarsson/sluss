package lifecycle

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// agentFixture names four sandboxes whose agents differ, including the gh /
// gh-remote pair a prefix match would confuse and a name carrying a backslash,
// which the retired script had to hand-escape for awk.
const agentFixture = `{
  "sandboxes": [
    { "name": "web", "agent": "opencode", "status": "running" },
    { "name": "auth", "agent": "claude", "status": "running" },
    { "name": "gh", "agent": "copilot", "status": "running" },
    { "name": "gh-remote", "agent": "gemini", "status": "running" },
    { "name": "a\\tb", "agent": "copilot", "status": "running" },
    { "name": "nameless", "status": "running" }
  ]
}`

func attachRunner(t *testing.T) (*Runner, *sbxstub.Stub) {
	t.Helper()
	stub := sbxstub.Install(t, agentFixture)
	return &Runner{Sbx: &sbx.Client{Bin: stub.Bin}, WorktreeRoot: t.TempDir()}, stub
}

func TestAttachDispatchesOnTheAgentSbxReports(t *testing.T) {
	tests := []struct {
		name    string
		sandbox string
		args    []string
		want    []string
		absent  string
	}{
		{
			name:    "opencode starts the web server and shows the port",
			sandbox: "web",
			want:    []string{"exec web sh -c", "ports web"},
			absent:  "run --name",
		},
		{
			name:    "claude enables remote control first",
			sandbox: "auth",
			want: []string{
				"settings set claude.remoteControl true",
				"run --name auth -- --remote-control",
			},
		},
		{
			name:    "copilot runs with --remote",
			sandbox: "gh",
			want:    []string{"run --name gh -- --remote"},
		},
		{
			// gh is a prefix of gh-remote; a substring match would attach the wrong agent.
			name:    "an unknown agent runs bare, matched by exact name",
			sandbox: "gh-remote",
			want:    []string{"run --name gh-remote --"},
			absent:  "--remote ",
		},
		{
			// The script had to pass this name through the environment to stop awk
			// processing its backslash. Decoding JSON properly removes the whole class.
			name:    "a name containing a backslash resolves",
			sandbox: `a\tb`,
			want:    []string{`run --name a\tb -- --remote`},
		},
		{
			name:    "extra arguments reach the agent",
			sandbox: "auth",
			args:    []string{"--continue"},
			want:    []string{"run --name auth -- --remote-control --continue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, stub := attachRunner(t)
			var out, errOut bytes.Buffer

			if _, err := r.Attach(context.Background(), "personal", tt.sandbox,
				sbx.Streams{Stdout: &out, Stderr: &errOut}, tt.args); err != nil {
				t.Fatalf("Attach: %v", err)
			}
			for _, want := range tt.want {
				callMatching(t, stub, want)
			}
			if tt.absent != "" {
				noCallMatching(t, stub, tt.absent)
			}
		})
	}
}

func TestAttachIsScopedByAppName(t *testing.T) {
	r, stub := attachRunner(t)
	var out, errOut bytes.Buffer

	if _, err := r.Attach(context.Background(), "work", "auth",
		sbx.Streams{Stdout: &out, Stderr: &errOut}, nil); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for _, want := range []string{
		"--app-name work ls --json",
		"--app-name work settings set claude.remoteControl true",
		"--app-name work run --name auth --",
	} {
		callMatching(t, stub, want)
	}
}

func TestAttachPrintsThePortForOpenCode(t *testing.T) {
	r, stub := attachRunner(t)
	stub.Ports(t, "4096/tcp -> 127.0.0.1:49155\n")
	var out, errOut bytes.Buffer

	if _, err := r.Attach(context.Background(), "personal", "web",
		sbx.Streams{Stdout: &out, Stderr: &errOut}, nil); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if !strings.Contains(out.String(), "OpenCode Web port:") {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(out.String(), "49155") {
		t.Errorf("stdout = %q, want the mapping", out.String())
	}
}

// A second attach must find the running server and do nothing, which is the PID
// file's job inside the sandbox — so the launcher sluss sends must carry it.
func TestAttachSendsTheIdempotentLauncher(t *testing.T) {
	r, stub := attachRunner(t)
	var out, errOut bytes.Buffer

	if _, err := r.Attach(context.Background(), "personal", "web",
		sbx.Streams{Stdout: &out, Stderr: &errOut}, nil); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	call := callMatching(t, stub, "exec web")
	for _, want := range []string{
		"/tmp/sluss-opencode-web.pid",
		"kill -0",
		"opencode web --hostname 0.0.0.0 --port 4096",
	} {
		if !strings.Contains(call, want) {
			t.Errorf("launcher does not contain %q; call = %q", want, call)
		}
	}
}

func TestAgentForWarnsAndFallsBackWhenSbxNamesNoAgent(t *testing.T) {
	r, _ := attachRunner(t)
	var warn bytes.Buffer

	agent, err := r.AgentFor(context.Background(), "personal", "nameless", &warn)
	if err != nil {
		t.Fatalf("AgentFor: %v", err)
	}
	if agent != DefaultAgent {
		t.Errorf("agent = %q, want %q", agent, DefaultAgent)
	}
	if !strings.Contains(warn.String(), "sbx reported no agent for nameless") {
		t.Errorf("warning = %q", warn.String())
	}
}

func TestAgentForBlamesSbxWhenTheListingFails(t *testing.T) {
	r, stub := attachRunner(t)
	stub.FailCommand(t, "ls --json", 1, "cannot connect: daemon is not running")
	var out, errOut bytes.Buffer

	_, err := r.Attach(context.Background(), "personal", "web",
		sbx.Streams{Stdout: &out, Stderr: &errOut}, nil)
	if err == nil {
		t.Fatal("Attach succeeded although the listing failed")
	}
	if !strings.Contains(err.Error(), "sbx ls failed") {
		t.Errorf("error = %q, want it to blame sbx rather than the agent", err)
	}
	// Guessing an agent here would run the wrong one against a live sandbox.
	noCallMatching(t, stub, "run --name")
}

func TestExecIn(t *testing.T) {
	r, stub := attachRunner(t)
	ctx := context.Background()
	var sink bytes.Buffer
	streams := sbx.Streams{Stdout: &sink, Stderr: &sink}

	if _, err := r.ExecIn(ctx, "personal", "web", streams, nil); err != nil {
		t.Fatalf("ExecIn shell: %v", err)
	}
	if _, err := r.ExecIn(ctx, "personal", "web", streams, []string{"git", "status"}); err != nil {
		t.Fatalf("ExecIn command: %v", err)
	}
	callMatching(t, stub, "exec -it web sh")
	callMatching(t, stub, "exec -it web git status")
}

func TestExecInReturnsTheChildsExitCode(t *testing.T) {
	r, stub := attachRunner(t)
	stub.Fail(t, 7, "")
	var sink bytes.Buffer

	code, err := r.ExecIn(context.Background(), "personal", "web",
		sbx.Streams{Stdout: &sink, Stderr: &sink}, []string{"false"})
	if err != nil {
		t.Fatalf("ExecIn reported an error for a non-zero child: %v", err)
	}
	if code != 7 {
		t.Errorf("exit code = %d, want 7", code)
	}
}
