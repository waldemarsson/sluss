package kits_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/kits"
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

// kitsDir builds a kits directory with one complete kit and one without a spec.
func kitsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dotnet-svelte"), 0o755); err != nil {
		t.Fatalf("creating kit: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "empty-kit"), 0o755); err != nil {
		t.Fatalf("creating kit: %v", err)
	}
	spec := "name: dotnet-svelte\ntools:\n  - dotnet\n"
	if err := os.WriteFile(filepath.Join(dir, "dotnet-svelte", kits.SpecFile), []byte(spec), 0o644); err != nil {
		t.Fatalf("writing spec: %v", err)
	}
	return dir
}

func TestList(t *testing.T) {
	dir := kitsDir(t)

	got, err := kits.List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("kits = %+v, want 2", got)
	}
	byName := map[string]bool{}
	for _, kit := range got {
		byName[kit.Name] = kit.HasSpec
	}
	if !byName["dotnet-svelte"] {
		t.Error("dotnet-svelte should report a spec")
	}
	if byName["empty-kit"] {
		t.Error("empty-kit has no spec.yaml but reported one")
	}
}

func TestListMissingDirectory(t *testing.T) {
	got, err := kits.List(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("List on a missing directory: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("kits = %+v, want none", got)
	}
}

func TestSpecRoundTripsByteForByte(t *testing.T) {
	dir := kitsDir(t)
	// Trailing newline, CRLF and a tab: nothing may be normalised on the way through.
	body := []byte("name: dotnet-svelte\r\ntools:\n\t- dotnet\n\n")

	if err := kits.WriteSpec(dir, "dotnet-svelte", body); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}
	got, err := kits.ReadSpec(dir, "dotnet-svelte")
	if err != nil {
		t.Fatalf("ReadSpec: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("round trip changed the bytes:\n got %q\nwant %q", got, body)
	}

	onDisk, err := os.ReadFile(filepath.Join(dir, "dotnet-svelte", kits.SpecFile))
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if string(onDisk) != string(body) {
		t.Errorf("on-disk bytes = %q, want %q", onDisk, body)
	}

	// The write goes through a temporary file; none of them may survive it, and the
	// spec must keep its normal permissions.
	entries, err := os.ReadDir(filepath.Join(dir, "dotnet-svelte"))
	if err != nil {
		t.Fatalf("reading the kit: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != kits.SpecFile {
			t.Errorf("left behind %q", entry.Name())
		}
	}
	info, err := os.Stat(filepath.Join(dir, "dotnet-svelte", kits.SpecFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}
}

func TestReadSpecOfAKitWithoutOne(t *testing.T) {
	dir := kitsDir(t)

	got, err := kits.ReadSpec(dir, "empty-kit")
	if err != nil {
		t.Fatalf("ReadSpec on a kit with no spec: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("body = %q, want empty", got)
	}
}

func TestNamesCannotEscapeTheKitsDirectory(t *testing.T) {
	dir := kitsDir(t)
	outside := filepath.Join(t.TempDir(), "victim")

	for _, name := range []string{"..", ".", "", "../victim", "a/b", filepath.Join("..", "..", "etc")} {
		if _, err := kits.ReadSpec(dir, name); !errors.Is(err, kits.ErrBadName) {
			t.Errorf("ReadSpec(%q) error = %v, want ErrBadName", name, err)
		}
		if err := kits.WriteSpec(dir, name, []byte("x")); !errors.Is(err, kits.ErrBadName) {
			t.Errorf("WriteSpec(%q) error = %v, want ErrBadName", name, err)
		}
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a rejected name still wrote something")
	}
}

func TestWriteSpecRefusesAnUnknownKit(t *testing.T) {
	dir := kitsDir(t)

	if err := kits.WriteSpec(dir, "not-a-kit", []byte("x")); err == nil {
		t.Fatal("WriteSpec created a kit that does not exist")
	}
}

func TestStatusReportsDirtyAndClean(t *testing.T) {
	dir := kitsDir(t)
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "kits")

	clean, err := kits.Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !clean.Repository || clean.Dirty {
		t.Errorf("status = %+v, want a clean repository", clean)
	}

	if err := kits.WriteSpec(dir, "dotnet-svelte", []byte("name: edited\n")); err != nil {
		t.Fatalf("WriteSpec: %v", err)
	}
	dirty, err := kits.Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !dirty.Dirty {
		t.Errorf("status = %+v, want dirty after an edit", dirty)
	}
	if !strings.Contains(dirty.Detail, "spec.yaml") {
		t.Errorf("detail = %q, want it to name the changed file", dirty.Detail)
	}
}

func TestStatusOutsideAGitRepository(t *testing.T) {
	got, err := kits.Status(context.Background(), kitsDir(t))
	if err != nil {
		t.Fatalf("Status outside a repository: %v", err)
	}
	if got.Repository || got.Dirty {
		t.Errorf("status = %+v, want a plain not-a-repository report", got)
	}
	if got.Detail == "" {
		t.Error("status gave no reason")
	}
}

func TestStatusOfAMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")

	got, err := kits.Status(context.Background(), missing)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if got.Repository || !strings.Contains(got.Detail, missing) {
		t.Errorf("status = %+v, want it to name the missing directory", got)
	}
}
