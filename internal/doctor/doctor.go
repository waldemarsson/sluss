// Package doctor answers "is this deployment actually working?" before the user
// finds out the hard way.
//
// The checks are deliberately blunt about exposure: a NAS that silently binds
// loopback looks healthy and is unreachable, so the bound address and whether it is
// loopback are the first thing reported.
package doctor

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/kits"
	"github.com/waldemarsson/sluss/internal/sbx"
)

// State is a check's outcome. Warn is for things that are wrong but do not stop
// sluss from serving what it can.
type State string

const (
	OK   State = "ok"
	Warn State = "warn"
	Fail State = "fail"
)

// Check is one line of the report.
type Check struct {
	Name   string
	State  State
	Detail string
}

// Report is every check, in the order they were run.
type Report struct {
	Checks []Check
}

// Failed reports whether anything is broken enough to exit non-zero.
func (r Report) Failed() bool {
	for _, c := range r.Checks {
		if c.State == Fail {
			return true
		}
	}
	return false
}

func (r *Report) add(name string, state State, format string, args ...any) {
	r.Checks = append(r.Checks, Check{Name: name, State: state, Detail: fmt.Sprintf(format, args...)})
}

// Run performs every check. scriptPath may be empty, which is itself a finding.
func Run(ctx context.Context, cfg *config.Config, client *sbx.Client, scriptPath string, scriptErr error) Report {
	var r Report

	checkListener(&r, cfg)
	checkAccess(&r, cfg)
	checkSbx(ctx, &r, client)
	checkScopes(ctx, &r, cfg, client)
	checkScript(ctx, &r, scriptPath, scriptErr)
	checkRepos(ctx, &r, cfg)
	checkKits(ctx, &r, cfg)

	return r
}

func checkListener(r *Report, cfg *config.Config) {
	addr := cfg.BindAddr()
	exposure := "loopback only; nothing outside this machine can reach it"
	if cfg.LAN {
		exposure = "exposed on every interface; anything that can route here can reach it, and there is no authentication"
	}

	listener, err := net.Listen("tcp", addr)
	if err == nil {
		_ = listener.Close()
		r.add("listener", OK, "%s — %s", addr, exposure)
		return
	}

	// The most likely reason the address is taken is that slussd is already serving
	// on it, which is exactly when someone runs doctor. Reporting that as a failure
	// would make the healthy case look broken.
	if conn, dialErr := net.DialTimeout("tcp", dialable(addr), 2*time.Second); dialErr == nil {
		_ = conn.Close()
		r.add("listener", Warn, "%s is already in use and answering — slussd is probably serving there already (%s)", addr, exposure)
		return
	}
	r.add("listener", Fail, "cannot bind %s: %v", addr, err)
}

// dialable turns a listen address into one that can be dialled: ":8420" listens on
// every interface, but nothing connects to an empty host.
func dialable(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

func checkAccess(r *Report, cfg *config.Config) {
	switch cfg.Access {
	case config.AccessPath:
		r.add("access", OK, "path mode — sandboxes at http://%s/s/<scope>/<name>/ ; no DNS, certificate or front proxy needed", cfg.BindAddr())
	case config.AccessHost:
		r.add("access", OK, "host mode — sandboxes at https://%s<name>.%s/ ; needs wildcard DNS and a matching certificate", cfg.HostPrefix, cfg.Domain)
	default:
		r.add("access", Fail, "unknown access mode %q", cfg.Access)
	}
}

func checkSbx(ctx context.Context, r *Report, client *sbx.Client) {
	version, err := client.Version(ctx)
	if err != nil {
		r.add("sbx", Fail, "%v", err)
		return
	}
	r.add("sbx", OK, "%s", strings.TrimSpace(version))
}

func checkScopes(ctx context.Context, r *Report, cfg *config.Config, client *sbx.Client) {
	for _, scope := range cfg.AppNames {
		sandboxes, err := client.List(ctx, scope)
		if err != nil {
			r.add("scope "+scope, Fail, "%v", err)
			continue
		}
		r.add("scope "+scope, OK, "reachable, %d sandbox(es)", len(sandboxes))
	}
}

// checkScript probes the subcommands the GUI depends on. The script carries no
// version, so its help topics are the only thing to ask.
func checkScript(ctx context.Context, r *Report, path string, resolveErr error) {
	if path == "" {
		r.add("sluss script", Fail, "%v", resolveErr)
		return
	}
	for _, sub := range []string{"start", "stop", "destroy", "path"} {
		if err := exec.CommandContext(ctx, path, "help", sub).Run(); err != nil {
			r.add("sluss script", Fail, "%s does not support %q: %v", path, sub, err)
			return
		}
	}
	r.add("sluss script", OK, "%s supports start, stop, destroy and path", path)
}

func checkRepos(ctx context.Context, r *Report, cfg *config.Config) {
	if len(cfg.Repos) == 0 {
		r.add("repositories", Warn, "none configured; the create form will be empty")
		return
	}
	for _, repo := range cfg.Repos {
		cmd := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--git-dir")
		if err := cmd.Run(); err != nil {
			r.add("repository "+repo, Fail, "not a git checkout")
			continue
		}
		r.add("repository "+repo, OK, "git checkout")
	}
}

func checkKits(ctx context.Context, r *Report, cfg *config.Config) {
	if cfg.KitsDir == "" {
		r.add("kits", Warn, "no kits directory configured; the kit editor will be empty")
		return
	}
	listed, err := kits.List(cfg.KitsDir)
	if err != nil {
		r.add("kits", Fail, "%v", err)
		return
	}
	status, err := kits.Status(ctx, cfg.KitsDir)
	if err != nil {
		r.add("kits", Fail, "%v", err)
		return
	}
	switch {
	case !status.Repository:
		r.add("kits", Warn, "%d kit(s) in %s, which is not a git repository (%s)", len(listed), cfg.KitsDir, status.Detail)
	case status.Dirty:
		r.add("kits", Warn, "%d kit(s) in %s, with uncommitted changes", len(listed), cfg.KitsDir)
	default:
		r.add("kits", OK, "%d kit(s) in %s, committed", len(listed), cfg.KitsDir)
	}
}
