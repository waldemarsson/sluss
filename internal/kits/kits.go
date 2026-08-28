// Package kits gives the browser plain-text access to hand-authored kit specs.
//
// sluss parses no YAML and never generates a kit: spec.yaml is opaque bytes on the
// way in and on the way out, and sbx is the only validator. The kits directory is a
// git repository the human commits by hand, so its status is reported, never acted
// on.
package kits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SpecFile is the one file per kit that sluss will read or write.
const SpecFile = "spec.yaml"

// ErrBadName rejects anything that is not a plain directory name.
var ErrBadName = errors.New("kit name must be a single path segment")

// Kit is one directory under the configured kits root.
type Kit struct {
	Name    string `json:"name"`
	HasSpec bool   `json:"hasSpec"`
}

// GitStatus describes the kits directory itself.
type GitStatus struct {
	Repository bool   `json:"repository"` // is it a git checkout at all
	Dirty      bool   `json:"dirty"`
	Detail     string `json:"detail"` // porcelain output, or why the status is unknown
}

// List returns the kits in dir. A missing directory is empty, not an error: an
// unconfigured kits directory should show an empty pane, not break the dashboard.
func List(dir string) ([]Kit, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading kits directory %s: %w", dir, err)
	}

	var kits []Kit
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		_, statErr := os.Stat(filepath.Join(dir, entry.Name(), SpecFile))
		kits = append(kits, Kit{Name: entry.Name(), HasSpec: statErr == nil})
	}
	return kits, nil
}

// ReadSpec returns a kit's spec.yaml exactly as it is on disk.
func ReadSpec(dir, name string) ([]byte, error) {
	path, err := specPath(dir, name)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// A kit that has no spec yet is editable, not broken.
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return body, nil
}

// WriteSpec stores a kit's spec.yaml byte for byte, with no parsing, formatting or
// validation of any kind.
func WriteSpec(dir, name string, body []byte) error {
	path, err := specPath(dir, name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("no kit named %q in %s", name, dir)
	}
	// Write beside the target and rename over it: a crash mid-write then leaves the
	// previous spec intact instead of a truncated one.
	temp, err := os.CreateTemp(filepath.Dir(path), ".spec-*.tmp")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// Status reports whether the kits directory has uncommitted changes. Committing is
// the human's job; sluss only says whether there is something to commit.
func Status(ctx context.Context, dir string) (GitStatus, error) {
	if dir == "" {
		return GitStatus{Detail: "no kits directory configured"}, nil
	}
	if _, err := os.Stat(dir); err != nil {
		return GitStatus{Detail: fmt.Sprintf("%s does not exist", dir)}, nil
	}

	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Not a git repository is a normal state to report, not a failure.
		return GitStatus{Detail: strings.TrimSpace(firstLine(stderr.String()))}, nil
	}

	changes := strings.TrimSpace(stdout.String())
	return GitStatus{Repository: true, Dirty: changes != "", Detail: changes}, nil
}

// specPath refuses any name that could escape the kits directory.
func specPath(dir, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("%q: %w", name, ErrBadName)
	}
	if dir == "" {
		return "", errors.New("no kits directory configured")
	}
	return filepath.Join(dir, name, SpecFile), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
