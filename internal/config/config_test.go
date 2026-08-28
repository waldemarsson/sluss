package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write puts a config file in a temp directory and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestLoadAppliesDefaults(t *testing.T) {
	path := write(t, `{"appNames": ["personal"], "worktreeRoot": "/w"}`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != DefaultPort {
		t.Errorf("Port = %d, want %d", c.Port, DefaultPort)
	}
	if c.Access != AccessPath {
		t.Errorf("Access = %q, want %q", c.Access, AccessPath)
	}
	if c.HostPrefix != DefaultHostPrefix {
		t.Errorf("HostPrefix = %q, want %q", c.HostPrefix, DefaultHostPrefix)
	}
}

func TestLoadDefaultsWorktreeRoot(t *testing.T) {
	path := write(t, `{"appNames": ["personal"]}`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory in this environment")
	}
	if want := filepath.Join(home, "src", "worktrees"); c.WorktreeRoot != want {
		t.Errorf("WorktreeRoot = %q, want %q", c.WorktreeRoot, want)
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		body string
		want error
	}{
		{"host without domain", `{"appNames":["p"],"worktreeRoot":"/w","access":"host"}`, ErrMissingDomain},
		{"unknown access mode", `{"appNames":["p"],"worktreeRoot":"/w","access":"tunnel"}`, ErrInvalidAccess},
		{"port too high", `{"appNames":["p"],"worktreeRoot":"/w","port":70000}`, ErrPortRange},
		{"port negative", `{"appNames":["p"],"worktreeRoot":"/w","port":-1}`, ErrPortRange},
		{"no scopes", `{"worktreeRoot":"/w"}`, ErrNoAppNames},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(t, tt.body)
			_, err := Load(path)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Load error = %v, want %v", err, tt.want)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error %q does not name the file %q", err, path)
			}
		})
	}
}

func TestLoadAcceptsHostModeWithDomain(t *testing.T) {
	path := write(t, `{"appNames":["p"],"worktreeRoot":"/w","access":"host","domain":"example.test"}`)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Access != AccessHost || c.Domain != "example.test" {
		t.Errorf("got access %q domain %q", c.Access, c.Domain)
	}
}

func TestLoadMissingFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")

	_, err := Load(path)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load error = %v, want fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file %q", err, path)
	}
}

func TestLoadMalformedNamesPath(t *testing.T) {
	path := write(t, `{"appNames": ["personal"`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load succeeded on malformed JSON")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the file %q", err, path)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := write(t, `{"appNames":["p"],"worktreeRoot":"/w","appName":"typo"}`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted an unknown key")
	}
	if !strings.Contains(err.Error(), "appName") {
		t.Errorf("error %q does not name the offending key", err)
	}
}

func TestBindAddr(t *testing.T) {
	tests := []struct {
		name string
		lan  bool
		want string
	}{
		{"loopback", false, "127.0.0.1:8420"},
		{"exposed", true, ":8420"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Port: 8420, LAN: tt.lan}
			if got := c.BindAddr(); got != tt.want {
				t.Errorf("BindAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasScopeAndRepo(t *testing.T) {
	c := &Config{AppNames: []string{"personal", "work"}, Repos: []string{"/src/a"}}

	if !c.HasScope("work") || c.HasScope("other") {
		t.Error("HasScope did not match the configured scopes exactly")
	}
	if !c.HasRepo("/src/a") || c.HasRepo("/src/b") {
		t.Error("HasRepo did not match the configured repos exactly")
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")

	if want := filepath.Join("/xdg", "sluss", "config.json"); DefaultPath() != want {
		t.Errorf("DefaultPath() = %q, want %q", DefaultPath(), want)
	}
}
