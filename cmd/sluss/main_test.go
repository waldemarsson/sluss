package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/release"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// exercise drives run() the way a terminal would and returns its exit code.
func exercise(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, strings.NewReader(""), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// deployment writes a configuration file and returns its path.
func deployment(t *testing.T, port int) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	body := `{"appNames":["personal"],"worktreeRoot":"` + filepath.Join(dir, "worktrees") + `","port":` +
		strconv.Itoa(port) + `,"access":"path"}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return configPath
}

// chdir moves the process into dir for one test and back afterwards. The lifecycle
// commands find their repository from the working directory, the way a terminal
// invocation does, so a test has to stand somewhere real.
func chdir(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("entering %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// workspace puts the test inside a real git repository with a stubbed sbx and the
// environment a terminal would carry, which is all the lifecycle commands read.
func workspace(t *testing.T) (*sbxstub.Stub, string) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "sluss tests")
	t.Setenv("GIT_AUTHOR_EMAIL", "tests@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "sluss tests")
	t.Setenv("GIT_COMMITTER_EMAIL", "tests@example.invalid")

	repo := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"commit", "-q", "--allow-empty", "-m", "seed"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	stub := sbxstub.Install(t, sbxstub.Fixture)
	worktrees := t.TempDir()
	t.Setenv("SLUSS_APP_NAME", "personal")
	t.Setenv("SLUSS_WORKTREE_ROOT", worktrees)
	chdir(t, repo)
	return stub, worktrees
}

func TestVersion(t *testing.T) {
	code, stdout, _ := exercise(t, "version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout) != version {
		t.Errorf("stdout = %q, want %q", stdout, version)
	}
}

func TestNoCommand(t *testing.T) {
	if code, _, _ := exercise(t); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, stderr := exercise(t, "launch")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr = %q, want the usage text", stderr)
	}
	if !strings.Contains(stderr, `"launch"`) {
		t.Errorf("stderr = %q, want the unknown command named", stderr)
	}
}

// Every command documents itself two ways, as the shell script did.
func TestEveryCommandHasHelp(t *testing.T) {
	commands := []string{
		"start", "list", "ls", "attach", "run", "exec", "stop",
		"destroy", "rm", "path", "serve", "doctor", "update",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			for _, args := range [][]string{{command, "--help"}, {command, "-h"}, {"help", command}} {
				code, stdout, stderr := exercise(t, args...)
				if code != 0 {
					t.Errorf("%v: exit code = %d, want 0 (stderr %q)", args, code, stderr)
				}
				if !strings.Contains(stdout, "Usage: sluss ") {
					t.Errorf("%v: stdout = %q, want its own usage line", args, stdout)
				}
			}
		})
	}
}

func TestTopLevelHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		code, stdout, _ := exercise(t, args...)
		if code != 0 {
			t.Errorf("%v: exit code = %d, want 0", args, code)
		}
		if !strings.Contains(stdout, "sluss - isolated agent sandboxes") {
			t.Errorf("%v: stdout = %q, want the overview", args, stdout)
		}
	}
	if code, _, _ := exercise(t, "help", "nonsense"); code != 2 {
		t.Errorf("help for an unknown command: exit code = %d, want 2", code)
	}
}

func TestStartCreatesASandbox(t *testing.T) {
	stub, worktrees := workspace(t)

	code, stdout, stderr := exercise(t, "start", "budget", "--cpus", "4")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "Created budget on agent/budget") {
		t.Errorf("stdout = %q", stdout)
	}
	calls := strings.Join(stub.Calls(t), "\n")
	if !strings.Contains(calls, "create --name budget --publish 4096 --cpus 4 opencode") {
		t.Errorf("sbx calls = %q", calls)
	}
	if _, err := os.Stat(filepath.Join(worktrees)); err != nil {
		t.Errorf("worktree root missing: %v", err)
	}
}

func TestStartRejectsABareAgentFlag(t *testing.T) {
	stub, _ := workspace(t)

	code, _, stderr := exercise(t, "start", "budget", "--agent")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--agent requires an agent name") {
		t.Errorf("stderr = %q", stderr)
	}
	if calls := strings.Join(stub.Calls(t), "\n"); strings.Contains(calls, "create") {
		t.Errorf("a sandbox was created anyway: %q", calls)
	}
}

func TestDestroyRefusalExitsOne(t *testing.T) {
	stub, _ := workspace(t)
	if code, _, stderr := exercise(t, "start", "budget"); code != 0 {
		t.Fatalf("start: %d %q", code, stderr)
	}

	code, _, stderr := exercise(t, "destroy", "ghost")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for a refusal", code)
	}
	if !strings.Contains(stderr, "worktree not found") {
		t.Errorf("stderr = %q", stderr)
	}
	if calls := strings.Join(stub.Calls(t), "\n"); strings.Contains(calls, " rm ") {
		t.Errorf("a refused destroy still called sbx rm: %q", calls)
	}
}

func TestDestroyRejectsAnUnknownFlag(t *testing.T) {
	workspace(t)
	if code, _, _ := exercise(t, "destroy", "budget", "--yes"); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestCommandsAndAliasesDispatch(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"list"}, "ls"},
		{[]string{"ls"}, "ls"},
		{[]string{"stop", "web"}, "stop web"},
		{[]string{"exec", "web", "true"}, "exec -it web true"},
		{[]string{"attach", "auth"}, "run --name auth -- --remote-control"},
		{[]string{"run", "auth"}, "run --name auth -- --remote-control"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stub, _ := workspace(t)
			if code, _, stderr := exercise(t, tt.args...); code != 0 {
				t.Fatalf("exit code = %d, stderr %q", code, stderr)
			}
			if calls := strings.Join(stub.Calls(t), "\n"); !strings.Contains(calls, tt.want) {
				t.Errorf("sbx calls = %q, want one containing %q", calls, tt.want)
			}
		})
	}
}

// "sluss exec NAME false" must exit 1 without claiming sluss failed.
func TestExecReturnsTheChildsExitCode(t *testing.T) {
	stub, _ := workspace(t)
	stub.Fail(t, 4, "")

	code, _, stderr := exercise(t, "exec", "web", "false")
	if code != 4 {
		t.Fatalf("exit code = %d, want 4", code)
	}
	if strings.Contains(stderr, "error:") {
		t.Errorf("stderr = %q, want no error for a non-zero child", stderr)
	}
}

func TestPathNeedsNoSbx(t *testing.T) {
	stub, worktrees := workspace(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := exercise(t, "path", "budget")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	want := filepath.Join(worktrees, filepath.Base(cwd), "budget")
	if strings.TrimSpace(stdout) != want {
		t.Errorf("stdout = %q, want %q", strings.TrimSpace(stdout), want)
	}
	if calls := stub.Calls(t); len(calls) != 0 {
		t.Errorf("path invoked sbx: %v", calls)
	}
}

func TestRepositoryCommandsRefuseOutsideARepository(t *testing.T) {
	sbxstub.Install(t, sbxstub.Fixture)
	t.Setenv("SLUSS_APP_NAME", "personal")
	t.Setenv("SLUSS_WORKTREE_ROOT", t.TempDir())
	chdir(t, t.TempDir())

	for _, args := range [][]string{{"start", "budget"}, {"destroy", "budget"}, {"path", "budget"}} {
		code, _, stderr := exercise(t, args...)
		if code == 0 {
			t.Errorf("%v succeeded outside a repository", args)
		}
		if !strings.Contains(stderr, "run sluss "+args[0]+" inside") {
			t.Errorf("%v: stderr = %q, want advice naming the command", args, stderr)
		}
	}
}

// Every sbx call is scoped by app-name and there is no default, so a missing scope
// is named rather than guessed.
func TestLifecycleCommandsNeedAScope(t *testing.T) {
	sbxstub.Install(t, sbxstub.Fixture)
	t.Setenv("SLUSS_APP_NAME", "")
	t.Setenv("SLUSS_WORKTREE_ROOT", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no configuration file to fall back on
	chdir(t, t.TempDir())

	code, _, stderr := exercise(t, "list")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "SLUSS_APP_NAME") {
		t.Errorf("stderr = %q, want the environment variable named", stderr)
	}
}

// The environment wins, but the configuration file is a working fallback for a
// deployment that has one and no exported variables.
func TestScopeAndWorktreeRootFallBackToTheConfigFile(t *testing.T) {
	stub, _ := workspace(t)
	configHome := t.TempDir()
	worktrees := filepath.Join(configHome, "from-config")
	if err := os.MkdirAll(filepath.Join(configHome, "sluss"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"appNames":["from-config"],"worktreeRoot":"` + worktrees + `","access":"path"}`
	if err := os.WriteFile(filepath.Join(configHome, "sluss", "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("SLUSS_APP_NAME", "")
	t.Setenv("SLUSS_WORKTREE_ROOT", "")

	if code, _, stderr := exercise(t, "list"); code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if calls := strings.Join(stub.Calls(t), "\n"); !strings.Contains(calls, "--app-name from-config") {
		t.Errorf("sbx calls = %q, want the configured scope", calls)
	}
}

func TestServeRefusesAnOccupiedPort(t *testing.T) {
	sbxstub.Install(t, sbxstub.Empty)
	port := freePort(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("occupying the port: %v", err)
	}
	defer occupied.Close()
	configPath := deployment(t, port)

	code, stdout, stderr := exercise(t, "serve", "-config", configPath)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	// The address it tried must be in the message: a NAS that quietly bound
	// something else would look healthy and be unreachable.
	if !strings.Contains(stderr, "127.0.0.1:"+strconv.Itoa(port)) {
		t.Errorf("stderr = %q, want the address it tried", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing served", stdout)
	}
}

func TestServeReportsAMissingConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.json")

	code, _, stderr := exercise(t, "serve", "-config", missing)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, missing) {
		t.Errorf("stderr = %q, want the config path", stderr)
	}
}

// doctor must work with nothing installed but the binary itself — there is no
// script to resolve any more.
func TestDoctorReportsEveryCheck(t *testing.T) {
	sbxstub.Install(t, sbxstub.Empty)
	configPath := deployment(t, freePort(t))

	code, stdout, _ := exercise(t, "doctor", "-config", configPath)

	for _, want := range []string{"listener", "access", "sbx", "scope personal", "git", "worktree root", "kits"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("doctor output missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "script") {
		t.Errorf("doctor still reports on a script:\n%s", stdout)
	}
	if code != 0 && code != 1 {
		t.Errorf("exit code = %d, want 0 or 1", code)
	}
}

func TestUpdateIsWiredToTheReleaseClient(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("new sluss")
	if err := tw.WriteHeader(&tar.Header{Name: "sluss", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	tw.Write(body)
	tw.Close()
	gz.Close()
	asset := buf.Bytes()
	sum := sha256.Sum256(asset)
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v9.0.0"}`)
	})
	mux.HandleFunc("/releases/download/v9.0.0/"+name, func(w http.ResponseWriter, r *http.Request) {
		w.Write(asset)
	})
	mux.HandleFunc("/releases/download/v9.0.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	target := filepath.Join(t.TempDir(), "sluss")
	if err := os.WriteFile(target, []byte("old sluss"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := updateClient
	updateClient = &release.Client{APIURL: srv.URL + "/api", DownloadURL: srv.URL + "/releases", HTTP: srv.Client()}
	updateTarget = target
	t.Cleanup(func() { updateClient = previous; updateTarget = "" })

	code, stdout, stderr := exercise(t, "update")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "v9.0.0") {
		t.Errorf("stdout = %q, want the new version named", stdout)
	}
	installed, err := os.ReadFile(target)
	if err != nil || string(installed) != "new sluss" {
		t.Errorf("binary = %q (%v), want the released one", installed, err)
	}
}

func TestUpdateTakesNoArguments(t *testing.T) {
	if code, _, _ := exercise(t, "update", "now"); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
