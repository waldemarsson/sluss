package sbx

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
)

// hints turn sbx's stderr into something a person can act on. AGENTS.md: these
// messages are the difference between a script and a tool — don't dump raw stderr.
//
// Each match is lowercase and compared against lowercased stderr; each message is
// formatted with the app-name.
var hints = []struct {
	match   string
	message string
}{
	{"not logged in", "sbx is not logged in for scope %[1]s; run: sbx --app-name %[1]s login"},
	{"login required", "sbx is not logged in for scope %[1]s; run: sbx --app-name %[1]s login"},
	{"daemon", "the sbx daemon for scope %[1]s is not running; run: sbx --app-name %[1]s ls to start it"},
	{"connection refused", "the sbx daemon for scope %[1]s is not reachable; run: sbx --app-name %[1]s ls to start it"},
	{"already exists", "that name already exists in scope %[1]s; choose another name or destroy the existing sandbox"},
	{"network policy", "sbx's network policy blocked the operation; review it with: sbx --app-name %[1]s policy"},
	{"kit", "kit fetch failed; check the kit path or URL, and that the kit directory is readable"},
}

// classify wraps a failed sbx call with what was attempted plus a readable cause.
func classify(appName, command, stderr string, err error) error {
	// exec.ErrNotFound comes from a PATH lookup; a configured absolute path that is
	// missing surfaces as fs.ErrNotExist instead. Both mean "no sbx here".
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("running %s: sbx is not on PATH; install it from https://docs.docker.com/ai/sandboxes/install/: %w", command, err)
	}
	return fmt.Errorf("%s: %s", command, cause(appName, stderr, err))
}

func cause(appName, stderr string, err error) string {
	lower := strings.ToLower(stderr)
	for _, h := range hints {
		if strings.Contains(lower, h.match) {
			return fmt.Sprintf(h.message, appName)
		}
	}
	// Nothing recognised: the first non-empty line of stderr is the most useful
	// thing we have, and one line is not a stderr dump.
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return err.Error()
}
