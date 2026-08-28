// Package sbxstub installs a fake sbx executable for tests.
//
// sbx needs hardware virtualisation and its own daemon, so it cannot run in the
// devcontainer (AGENTS.md "Where things run"). Tests stub it the way
// scripts/test-sluss already does: a shell script on PATH records the argv it was
// called with, captures stdin, and serves a JSON fixture for "ls --json".
//
// This is a test-only package, but not a _test.go file: internal/sbx,
// internal/fleet and internal/script all use it.
package sbxstub

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Fixture mirrors the sbx output recorded in docs/SPIKE.md — including a stopped
// sandbox that omits "ports" entirely, and the two-workspace shape (worktree plus
// the repository's .git directory) that internal/fleet derives the repo from.
const Fixture = `{
  "sandboxes": [
    { "name": "web", "id": "d1e0", "agent": "opencode", "status": "stopped",
      "workspaces": ["/src/worktrees/mercurius/web", "/src/mercurius/.git"] },
    { "name": "auth", "id": "50fe", "agent": "claude", "status": "running",
      "ports": [{"host_ip":"127.0.0.1","host_port":49155,"sandbox_port":4096,"protocol":"tcp"}],
      "workspaces": ["/src/worktrees/mercurius/auth", "/src/mercurius/.git"] }
  ]
}`

// Empty is what a scope with no sandboxes returns.
const Empty = `{"sandboxes": []}`

const script = `#!/bin/sh
printf '%s\n' "$*" >> "$SBX_LOG"
if [ -n "${SBX_SLEEP:-}" ]; then sleep "$SBX_SLEEP"; fi
app=""
prev=""
for a in "$@"; do
	if [ "$prev" = "--app-name" ]; then app="$a"; fi
	prev="$a"
done
fixture="$SBX_FIXTURE"
if [ -n "$app" ] && [ -f "$SBX_SCOPE_DIR/$app.json" ]; then fixture="$SBX_SCOPE_DIR/$app.json"; fi
if [ -n "$app" ] && [ -f "$SBX_SCOPE_DIR/$app.fail" ]; then
	cat "$SBX_SCOPE_DIR/$app.fail" >&2
	exit 1
fi
calls=$(wc -l < "$SBX_LOG" | tr -d ' ')
cat > "$SBX_STDIN_DIR/$calls" 2>/dev/null || :
if [ -n "${SBX_STDERR:-}" ]; then printf '%s\n' "$SBX_STDERR" >&2; fi
if [ "${SBX_EXIT:-0}" -ne 0 ]; then exit "$SBX_EXIT"; fi
if [ -n "${SBX_BANNER:-}" ]; then printf '%s\n' "$SBX_BANNER"; fi
case " $* " in
	*" --json "* | *" --json") cat "$fixture" ;;
	*" ls "*) cat "$SBX_SECRETS" ;;
esac
`

// Stub is a fake sbx on PATH. Its zero value is not useful; call Install.
type Stub struct {
	Bin      string // absolute path to the fake executable
	log      string
	stdinDir string
	fixture  string
	scopeDir string
}

// Install writes the fake sbx into a temp directory, puts that directory first on
// PATH for the duration of the test, and points it at fixture as the "ls --json"
// output. Because it uses t.Setenv, a test calling Install cannot be parallel.
func Install(t *testing.T, fixture string) *Stub {
	t.Helper()
	dir := t.TempDir()
	s := &Stub{
		Bin:      filepath.Join(dir, "sbx"),
		log:      filepath.Join(dir, "calls.log"),
		stdinDir: filepath.Join(dir, "stdin"),
		fixture:  filepath.Join(dir, "fixture.json"),
		scopeDir: filepath.Join(dir, "scopes"),
	}
	writeFile(t, s.Bin, script, 0o755)
	writeFile(t, s.fixture, fixture, 0o644)
	writeFile(t, s.log, "", 0o644)
	writeFile(t, filepath.Join(dir, "secrets.txt"), "", 0o644)
	for _, d := range []string{s.stdinDir, s.scopeDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("creating %s: %v", d, err)
		}
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SBX_LOG", s.log)
	t.Setenv("SBX_FIXTURE", s.fixture)
	t.Setenv("SBX_STDIN_DIR", s.stdinDir)
	t.Setenv("SBX_SECRETS", filepath.Join(dir, "secrets.txt"))
	t.Setenv("SBX_SCOPE_DIR", s.scopeDir)
	t.Setenv("SBX_EXIT", "0")
	t.Setenv("SBX_STDERR", "")
	t.Setenv("SBX_BANNER", "")
	t.Setenv("SBX_SLEEP", "")
	return s
}

// Fail makes every later call exit with code and print stderr.
func (s *Stub) Fail(t *testing.T, code int, stderr string) {
	t.Helper()
	t.Setenv("SBX_EXIT", strconv.Itoa(code))
	t.Setenv("SBX_STDERR", stderr)
}

// Banner makes every later call print text before its real output — what sbx does
// on first use of an unseen app-name (docs/SPIKE.md assumption 5).
func (s *Stub) Banner(t *testing.T, text string) {
	t.Helper()
	t.Setenv("SBX_BANNER", text)
}

// ScopeFixture makes "ls --json" answer differently for one app-name, so a test can
// give each scope its own sandboxes.
func (s *Stub) ScopeFixture(t *testing.T, scope, fixture string) {
	t.Helper()
	writeFile(t, filepath.Join(s.scopeDir, scope+".json"), fixture, 0o644)
}

// FailScope makes every call for one app-name exit non-zero with stderr, leaving
// the other scopes working.
func (s *Stub) FailScope(t *testing.T, scope, stderr string) {
	t.Helper()
	writeFile(t, filepath.Join(s.scopeDir, scope+".fail"), stderr, 0o644)
}

// Slow makes every later call take seconds before answering, for tests that need a
// poll to outlive its interval.
func (s *Stub) Slow(t *testing.T, seconds string) {
	t.Helper()
	t.Setenv("SBX_SLEEP", seconds)
}

// Secrets sets what a non-JSON "ls" prints, used by the secret listing.
func (s *Stub) Secrets(t *testing.T, body string) {
	t.Helper()
	writeFile(t, os.Getenv("SBX_SECRETS"), body, 0o644)
}

// Calls returns the argv of every invocation so far, one string per call.
func (s *Stub) Calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(s.log)
	if err != nil {
		t.Fatalf("reading call log: %v", err)
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// Stdin returns what was piped into the nth call, counting from 1.
func (s *Stub) Stdin(t *testing.T, n int) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.stdinDir, strconv.Itoa(n)))
	if err != nil {
		return ""
	}
	return string(data)
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
