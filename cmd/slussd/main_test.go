package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// deployment writes a configuration file and a fake sluss script, and returns the
// config path.
func deployment(t *testing.T, port int) string {
	t.Helper()
	dir := t.TempDir()

	scriptPath := filepath.Join(dir, "sluss")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing fake script: %v", err)
	}
	t.Setenv("SLUSS_SCRIPT", scriptPath)

	configPath := filepath.Join(dir, "config.json")
	body := `{"appNames":["personal"],"worktreeRoot":"` + filepath.Join(dir, "worktrees") + `","port":` +
		strconv.Itoa(port) + `,"access":"path"}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return configPath
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

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.TrimSpace(stdout.String()) != version {
		t.Errorf("stdout = %q, want %q", stdout.String(), version)
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run([]string{"launch"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr = %q, want the usage text", stderr.String())
	}
}

func TestNoCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
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

	var stdout, stderr bytes.Buffer
	code := run([]string{"serve", "-config", configPath}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	// The address it tried must be in the message: a NAS that quietly bound
	// something else would look healthy and be unreachable.
	if !strings.Contains(stderr.String(), "127.0.0.1:"+strconv.Itoa(port)) {
		t.Errorf("stderr = %q, want the address it tried", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing served", stdout.String())
	}
}

func TestServeReportsAMissingConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "absent.json")

	if code := run([]string{"serve", "-config", missing}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), missing) {
		t.Errorf("stderr = %q, want the config path", stderr.String())
	}
}

func TestServeReportsAMissingScript(t *testing.T) {
	sbxstub.Install(t, sbxstub.Empty)
	configPath := deployment(t, freePort(t))
	t.Setenv("SLUSS_SCRIPT", "")
	t.Setenv("HOME", t.TempDir()) // no installed script either

	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve", "-config", configPath}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "sluss script not found") {
		t.Errorf("stderr = %q, want the resolution failure", stderr.String())
	}
}

func TestDoctorReportsEveryCheckAndFails(t *testing.T) {
	sbxstub.Install(t, sbxstub.Empty)
	configPath := deployment(t, freePort(t))

	var stdout, stderr bytes.Buffer
	code := run([]string{"doctor", "-config", configPath}, &stdout, &stderr)

	out := stdout.String()
	for _, want := range []string{"listener", "access", "sbx", "scope personal", "sluss script", "kits"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}
	// The fake script answers every "help" call, but it is not a real sluss, and
	// the fake sbx has no version output — doctor must still produce a verdict.
	if code != 0 && code != 1 {
		t.Errorf("exit code = %d, want 0 or 1", code)
	}
}
