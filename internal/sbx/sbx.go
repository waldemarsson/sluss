// Package sbx is the single chokepoint for every sbx invocation in sluss.
//
// Every command takes the app-name (the sbx scope) as a required parameter — never
// an option with a default, never a call constructed outside this package. That is
// what keeps profiles a small later addition instead of a rewrite (AGENTS.md,
// docs/DECISIONS.md D3/D10).
package sbx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Client runs the sbx executable. The zero value runs "sbx" from PATH; tests set
// Bin to a stub.
type Client struct {
	Bin string
}

// Sandbox is one entry of "sbx ls --json". The field names follow the JSON sbx
// emits, recorded in docs/SPIKE.md — that recording is the contract these tests
// hold us to, so a drift in sbx shows up as an empty fleet, not a parse error.
type Sandbox struct {
	Name       string   `json:"name"`
	ID         string   `json:"id"`
	Agent      string   `json:"agent"`
	Status     string   `json:"status"`
	Ports      []Port   `json:"ports"`
	Workspaces []string `json:"workspaces"`
}

// Port is one published mapping. A stopped sandbox reports none at all.
type Port struct {
	HostIP      string `json:"host_ip"`
	HostPort    int    `json:"host_port"`
	SandboxPort int    `json:"sandbox_port"`
	Protocol    string `json:"protocol"`
}

// ErrNoAppName guards the rule this package exists to enforce.
var ErrNoAppName = errors.New("sbx: app-name is required")

// PortFor returns the host mapping for a sandbox port, if it is published.
func (s Sandbox) PortFor(sandboxPort int) (Port, bool) {
	for _, p := range s.Ports {
		if p.SandboxPort == sandboxPort {
			return p, true
		}
	}
	return Port{}, false
}

func (c *Client) binary() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "sbx"
}

// run executes one scoped sbx command. context.Context comes first because this
// shells out; cancelling the context kills the child process.
func (c *Client) run(ctx context.Context, appName string, stdin io.Reader, args ...string) ([]byte, error) {
	if appName == "" {
		return nil, ErrNoAppName
	}
	argv := append([]string{"--app-name", appName}, args...)
	return c.exec(ctx, appName, argv, stdin)
}

func (c *Client) exec(ctx context.Context, appName string, argv []string, stdin io.Reader) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.binary(), argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = stdin
	if err := cmd.Run(); err != nil {
		return nil, classify(appName, strings.Join(argv, " "), stderr.String(), err)
	}
	return stdout.Bytes(), nil
}

// List returns every sandbox in one scope.
func (c *Client) List(ctx context.Context, appName string) ([]Sandbox, error) {
	out, err := c.run(ctx, appName, nil, "ls", "--json")
	if err != nil {
		return nil, err
	}
	// A previously unused app-name starts a fresh daemon and prints a first-run
	// banner before the JSON (docs/SPIKE.md assumption 5). Try each brace in turn
	// rather than assuming the first one opens the payload — banner text is free to
	// contain one.
	var payload struct {
		Sandboxes []Sandbox `json:"sandboxes"`
	}
	for offset := 0; offset < len(out); offset++ {
		if out[offset] != '{' {
			continue
		}
		if err := json.Unmarshal(bytes.TrimSpace(out[offset:]), &payload); err == nil {
			return payload.Sandboxes, nil
		}
	}
	return nil, fmt.Errorf("sbx ls (scope %s): no JSON object in output", appName)
}

// SecretNames lists the names of a scope's secrets. Values are never read back:
// only sbx and the sandbox ever see them (docs/DECISIONS.md D1).
//
// The exact output shape is pending docs/SPIKE.md assumption 5's unfinished half;
// this parses defensively — first whitespace-separated field of each line, minus a
// "NAME" header if sbx prints one.
func (c *Client) SecretNames(ctx context.Context, appName string) ([]string, error) {
	out, err := c.run(ctx, appName, nil, "secret", "ls")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] == "NAME" {
			continue
		}
		names = append(names, fields[0])
	}
	return names, nil
}

// SecretSet stores a value. The value goes in on stdin, never in argv, where any
// process on the machine could read it out of "ps".
func (c *Client) SecretSet(ctx context.Context, appName, name, value string) error {
	_, err := c.run(ctx, appName, strings.NewReader(value), "secret", "set", name)
	return err
}

// SecretDelete removes a secret by name.
//
// UNVERIFIED: "secret rm" is the assumed subcommand, and "secret set" is assumed to
// read the value from stdin. docs/SPIKE.md assumption 5's unfinished half settles
// both. If sbx disagrees, this file is the only place that changes.
func (c *Client) SecretDelete(ctx context.Context, appName, name string) error {
	_, err := c.run(ctx, appName, nil, "secret", "rm", name)
	return err
}

// Version identifies the installed sbx. It is the one command that is not scoped:
// it describes the binary rather than talking to a scoped daemon, and scoping it
// would start a daemon just to print a version string.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, err := c.exec(ctx, "", []string{"version"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
