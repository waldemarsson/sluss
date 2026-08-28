package sbx_test

import (
	"context"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

func TestListParsesFixture(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.List(context.Background(), "personal")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sandboxes, want 2", len(got))
	}
	web, auth := got[0], got[1]
	if web.Name != "web" || web.Agent != "opencode" || web.Status != "stopped" || web.ID != "d1e0" {
		t.Errorf("web = %+v", web)
	}
	if len(web.Ports) != 0 {
		t.Errorf("stopped sandbox reported ports: %+v", web.Ports)
	}
	if len(web.Workspaces) != 2 {
		t.Errorf("web workspaces = %v", web.Workspaces)
	}
	p, ok := auth.PortFor(4096)
	if !ok || p.HostPort != 49155 || p.HostIP != "127.0.0.1" {
		t.Errorf("auth PortFor(4096) = %+v, %v", p, ok)
	}
	if _, ok := auth.PortFor(9999); ok {
		t.Error("PortFor returned a mapping for an unpublished port")
	}
}

func TestEveryCallIsScoped(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}
	ctx := context.Background()

	if _, err := c.List(ctx, "personal"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := c.SecretNames(ctx, "work"); err != nil {
		t.Fatalf("SecretNames: %v", err)
	}
	if err := c.SecretSet(ctx, "work", "TOKEN", "s3cret"); err != nil {
		t.Fatalf("SecretSet: %v", err)
	}
	if err := c.SecretDelete(ctx, "work", "TOKEN"); err != nil {
		t.Fatalf("SecretDelete: %v", err)
	}

	want := []string{
		"--app-name personal ls --json",
		"--app-name work secret ls",
		"--app-name work secret set TOKEN",
		"--app-name work secret rm TOKEN",
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

func TestCallsWithoutAppNameAreRefused(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}

	if _, err := c.List(context.Background(), ""); err == nil {
		t.Fatal("List accepted an empty app-name")
	}
	if calls := stub.Calls(t); len(calls) != 0 {
		t.Errorf("sbx was invoked anyway: %v", calls)
	}
}

func TestSecretValueNeverReachesArgv(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}

	if err := c.SecretSet(context.Background(), "work", "TOKEN", "s3cret"); err != nil {
		t.Fatalf("SecretSet: %v", err)
	}

	for _, call := range stub.Calls(t) {
		if strings.Contains(call, "s3cret") {
			t.Fatalf("secret value appeared in argv: %q", call)
		}
	}
	if got := stub.Stdin(t, 1); got != "s3cret" {
		t.Errorf("stdin = %q, want the secret value", got)
	}
}

func TestSecretSetFailureDoesNotLeakTheValue(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Fail(t, 1, "sbx: not logged in")
	c := &sbx.Client{Bin: stub.Bin}

	err := c.SecretSet(context.Background(), "work", "TOKEN", "s3cret")
	if err == nil {
		t.Fatal("SecretSet succeeded against a failing sbx")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("error leaked the secret value: %v", err)
	}
}

func TestSecretNamesSkipsHeaderAndBlanks(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Secrets(t, "NAME\nGITHUB_TOKEN\n\nANTHROPIC_API_KEY\n")
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.SecretNames(context.Background(), "work")
	if err != nil {
		t.Fatalf("SecretNames: %v", err)
	}
	want := []string{"GITHUB_TOKEN", "ANTHROPIC_API_KEY"}
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("name %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestListToleratesFirstRunBanner(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Banner(t, "Starting sandboxd for app-name fresh...\nImporting settings")
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.List(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sandboxes, want 2", len(got))
	}
}

func TestListToleratesABannerContainingABrace(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	stub.Banner(t, "first run: writing {config} to ~/.sbx")
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.List(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sandboxes, want 2", len(got))
	}
}

func TestListWithoutJSON(t *testing.T) {
	stub := sbxstub.Install(t, "not json at all")
	c := &sbx.Client{Bin: stub.Bin}

	if _, err := c.List(context.Background(), "personal"); err == nil {
		t.Fatal("List succeeded on output that has no JSON")
	}
}

func TestListEmptyScope(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	c := &sbx.Client{Bin: stub.Bin}

	got, err := c.List(context.Background(), "fresh")
	if err != nil {
		t.Fatalf("List on an empty scope: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d sandboxes, want 0", len(got))
	}
}

func TestFailuresAreMappedNotDumped(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   string
	}{
		{"not logged in", "Error: not logged in to Docker", "sbx --app-name personal login"},
		{"daemon down", "cannot connect: daemon is not running", "daemon for scope personal is not running"},
		{"name taken", "Error: sandbox \"web\" already exists", "already exists in scope personal"},
		{"policy", "install hook denied by network policy", "network policy blocked"},
		{"unknown", "line one\nline two\nline three", "line one"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := sbxstub.Install(t, sbxstub.Fixture)
			stub.Fail(t, 1, tt.stderr)
			c := &sbx.Client{Bin: stub.Bin}

			_, err := c.List(context.Background(), "personal")
			if err == nil {
				t.Fatal("List succeeded against a failing sbx")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
			if !strings.Contains(err.Error(), "ls --json") {
				t.Errorf("error = %q, does not say what was attempted", err)
			}
			if strings.Count(err.Error(), "\n") != 0 {
				t.Errorf("error spans multiple lines (stderr dump): %q", err)
			}
		})
	}
}

func TestMissingBinary(t *testing.T) {
	c := &sbx.Client{Bin: "/nonexistent/sbx"}

	_, err := c.List(context.Background(), "personal")
	if err == nil {
		t.Fatal("List succeeded with no sbx binary")
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error = %q, want the install hint", err)
	}
}

func TestVersion(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	c := &sbx.Client{Bin: stub.Bin}

	if _, err := c.Version(context.Background()); err != nil {
		t.Fatalf("Version: %v", err)
	}
	calls := stub.Calls(t)
	if len(calls) != 1 || calls[0] != "version" {
		t.Errorf("calls = %v, want a single unscoped \"version\"", calls)
	}
}
