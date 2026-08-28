package doctor_test

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/doctor"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// fakeScript answers "help <sub>" for the subcommands the GUI needs.
func fakeScript(t *testing.T, supported string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sluss")
	body := "#!/bin/sh\ncase \"$2\" in " + supported + ") exit 0 ;; *) exit 2 ;; esac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing fake script: %v", err)
	}
	return path
}

func find(t *testing.T, report doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("no check named %q in %+v", name, report.Checks)
	return doctor.Check{}
}

func healthy(t *testing.T) (*config.Config, *sbx.Client, string) {
	t.Helper()
	stub := sbxstub.Install(t, sbxstub.Empty)
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if out, err := exec.Command("git", "init", "-q", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cfg := &config.Config{
		AppNames: []string{"personal"}, WorktreeRoot: filepath.Join(base, "worktrees"),
		Access: config.AccessPath, Port: freePort(t), Repos: []string{repo},
		KitsDir: filepath.Join(base, "kits"),
	}
	if err := os.MkdirAll(cfg.KitsDir, 0o755); err != nil {
		t.Fatalf("creating kits dir: %v", err)
	}
	return cfg, &sbx.Client{Bin: stub.Bin}, fakeScript(t, "start|stop|destroy|path")
}

// freePort asks the kernel for a port that is currently unused.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func TestHealthyDeploymentPasses(t *testing.T) {
	cfg, client, scriptPath := healthy(t)

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	if report.Failed() {
		t.Fatalf("healthy deployment reported a failure: %+v", report.Checks)
	}
	for _, name := range []string{"listener", "access", "sbx", "scope personal", "sluss script", "kits"} {
		if find(t, report, name).Name == "" {
			t.Errorf("missing check %q", name)
		}
	}
	if detail := find(t, report, "listener").Detail; !strings.Contains(detail, "loopback") {
		t.Errorf("listener detail = %q, want it to say loopback", detail)
	}
	if detail := find(t, report, "access").Detail; !strings.Contains(detail, "/s/<scope>/<name>/") {
		t.Errorf("access detail = %q, want the URL form", detail)
	}
}

func TestExposedBindingIsSaidPlainly(t *testing.T) {
	cfg, client, scriptPath := healthy(t)
	cfg.LAN = true

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	detail := find(t, report, "listener").Detail
	if !strings.Contains(detail, "exposed") || !strings.Contains(detail, "no authentication") {
		t.Errorf("listener detail = %q, want the exposure spelled out", detail)
	}
}

// Running doctor against a slussd that is already serving is the normal case, not
// a broken deployment: it must not report failure.
func TestPortInUseAndAnsweringWarns(t *testing.T) {
	cfg, client, scriptPath := healthy(t)
	listener, err := net.Listen("tcp", cfg.BindAddr())
	if err != nil {
		t.Fatalf("occupying the port: %v", err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	check := find(t, report, "listener")
	if check.State != doctor.Warn {
		t.Errorf("listener check = %+v, want a warning", check)
	}
	if !strings.Contains(check.Detail, cfg.BindAddr()) {
		t.Errorf("listener detail = %q, does not name the address", check.Detail)
	}
	if report.Failed() {
		t.Error("doctor failed against a deployment that is simply already running")
	}
}

// An address that cannot be bound and answers nothing is a real failure. The port
// here is one config.Load would refuse; it is used precisely because it can neither
// be bound nor dialled on any platform.
func TestUnbindableAddressFails(t *testing.T) {
	cfg, client, scriptPath := healthy(t)
	cfg.Port = 70000

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	check := find(t, report, "listener")
	if check.State != doctor.Fail || !strings.Contains(check.Detail, cfg.BindAddr()) {
		t.Errorf("listener check = %+v, want a failure naming the address", check)
	}
	if !report.Failed() {
		t.Error("Failed() = false with an unbindable listener")
	}
}

func TestBrokenSbxIsReportedPerScope(t *testing.T) {
	cfg, client, scriptPath := healthy(t)
	cfg.AppNames = []string{"personal", "work"}
	stub := sbxstub.Install(t, sbxstub.Empty)
	client.Bin = stub.Bin
	stub.FailScope(t, "work", "Error: not logged in")

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	if find(t, report, "scope personal").State != doctor.OK {
		t.Error("a healthy scope was reported as broken")
	}
	broken := find(t, report, "scope work")
	if broken.State != doctor.Fail || !strings.Contains(broken.Detail, "login") {
		t.Errorf("scope work = %+v, want a failure with the mapped hint", broken)
	}
}

func TestMissingSbxFails(t *testing.T) {
	cfg, _, scriptPath := healthy(t)

	report := doctor.Run(context.Background(), cfg, &sbx.Client{Bin: "/nonexistent/sbx"}, scriptPath, nil)

	if check := find(t, report, "sbx"); check.State != doctor.Fail {
		t.Errorf("sbx check = %+v, want a failure", check)
	}
	if !report.Failed() {
		t.Error("Failed() = false with no sbx installed")
	}
}

func TestScriptChecks(t *testing.T) {
	cfg, client, _ := healthy(t)

	missing := doctor.Run(context.Background(), cfg, client, "", errors.New("looked in ~/.local/bin/sluss"))
	if check := find(t, missing, "sluss script"); check.State != doctor.Fail || !strings.Contains(check.Detail, ".local/bin/sluss") {
		t.Errorf("script check = %+v, want the resolution error", check)
	}

	old := doctor.Run(context.Background(), cfg, client, fakeScript(t, "start|stop"), nil)
	if check := find(t, old, "sluss script"); check.State != doctor.Fail || !strings.Contains(check.Detail, "destroy") {
		t.Errorf("script check = %+v, want the unsupported subcommand named", check)
	}
}

func TestRepositoryChecks(t *testing.T) {
	cfg, client, scriptPath := healthy(t)
	notARepo := t.TempDir()
	cfg.Repos = append(cfg.Repos, notARepo)

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	if check := find(t, report, "repository "+notARepo); check.State != doctor.Fail {
		t.Errorf("check = %+v, want a failure for a non-checkout", check)
	}
}

func TestKitsWarnWithoutAGitRepository(t *testing.T) {
	cfg, client, scriptPath := healthy(t)

	report := doctor.Run(context.Background(), cfg, client, scriptPath, nil)

	check := find(t, report, "kits")
	if check.State != doctor.Warn {
		t.Errorf("kits check = %+v, want a warning", check)
	}
	if report.Failed() {
		t.Error("a warning made the whole report fail")
	}
}
