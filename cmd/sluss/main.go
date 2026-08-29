// Command sluss is the whole of sluss: the sandbox and worktree lifecycle tool, the
// dashboard that drives it from a browser, and the reverse proxy that puts each
// sandbox's web UI on one port.
//
// It used to be two things — a shell script called sluss and a Go daemon called
// slussd. There is now one implementation and one name, so a sandbox created from
// the browser and one created in a terminal cannot drift apart (docs/DECISIONS.md
// D23 and D24, superseding D14 and D16).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/doctor"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/lifecycle"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/server"
	"github.com/waldemarsson/sluss/web"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
//
// A package-level var (not a const) is what makes -X able to write to it.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is separated from main so tests can drive it without spawning a process.
// The three streams are parameters for the same reason, and because "exec" and
// "attach" hand them to a child process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}

	command, rest := args[0], args[1:]

	// "sluss start --help" and "sluss help start" reach the same topic.
	if wantsHelp(rest) {
		if topic, ok := helpFor(command); ok {
			fmt.Fprint(stdout, topic)
			return 0
		}
	}

	ctx := context.Background()
	streams := sbx.Streams{Stdin: stdin, Stdout: stdout, Stderr: stderr}

	switch command {
	case "start":
		return start(ctx, rest, stdout, stderr)
	case "list", "ls":
		return list(ctx, rest, stdout, stderr)
	case "attach", "run":
		return attach(ctx, rest, streams, stderr)
	case "exec":
		return execIn(ctx, rest, streams, stderr)
	case "stop":
		return stop(ctx, rest, stdout, stderr)
	case "destroy", "rm":
		return destroy(ctx, rest, stdout, stderr)
	case "path":
		return path(ctx, rest, stdout, stderr)
	case "serve":
		return serve(rest, stdout, stderr)
	case "doctor":
		return diagnose(rest, stdout, stderr)
	case "update":
		return update(ctx, rest, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help":
		return help(rest, stdout, stderr)
	case "--help", "-h":
		fmt.Fprint(stdout, usageText)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", command, usageText)
		return 2
	}
}

// wantsHelp matches the shell script's rule: help is asked for by passing exactly
// one argument, and that argument is --help or -h.
func wantsHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

func help(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usageText)
		return 0
	}
	if topic, ok := helpFor(args[0]); ok {
		fmt.Fprint(stdout, topic)
		return 0
	}
	fmt.Fprint(stderr, usageText)
	return 2
}

// environment is what the lifecycle commands need: which sbx scope to act in, and
// a runner pointed at the right worktree root.
type environment struct {
	runner  *lifecycle.Runner
	appName string
}

// loadEnvironment resolves the scope, worktree root and default agent from the
// environment first and the configuration file second, so a shell that exports
// SLUSS_APP_NAME keeps working exactly as it did.
//
// A missing or unreadable configuration file is not an error here: the command
// line predates the config file, and the environment alone is enough to run.
func loadEnvironment() (*environment, error) {
	appName := os.Getenv("SLUSS_APP_NAME")
	worktreeRoot := os.Getenv("SLUSS_WORKTREE_ROOT")
	configPath := config.DefaultPath()

	if appName == "" || worktreeRoot == "" {
		if cfg, err := config.Load(configPath); err == nil {
			if appName == "" && len(cfg.AppNames) > 0 {
				appName = cfg.AppNames[0]
			}
			if worktreeRoot == "" {
				worktreeRoot = cfg.WorktreeRoot
			}
		}
	}
	if worktreeRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("no worktree root: set SLUSS_WORKTREE_ROOT")
		}
		worktreeRoot = filepath.Join(home, "src", "worktrees")
	}
	// Every sbx command is scoped by app-name, with no default — that rule is what
	// keeps profiles a small later addition rather than a rewrite.
	if appName == "" {
		return nil, fmt.Errorf("no sbx scope: set SLUSS_APP_NAME, or list appNames in %s", configPath)
	}

	return &environment{
		appName: appName,
		runner: &lifecycle.Runner{
			Sbx:          &sbx.Client{},
			WorktreeRoot: worktreeRoot,
			Agent:        os.Getenv("SLUSS_AGENT"),
		},
	}, nil
}

// emit writes a lifecycle result to the terminal and reports its exit code.
func emit(stdout, stderr io.Writer, result lifecycle.Result, err error) int {
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	io.WriteString(stdout, result.Stdout)
	io.WriteString(stderr, result.Stderr)
	return result.ExitCode
}

func fail(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, "error: "+format+"\n", args...)
	return 1
}

func misuse(stderr io.Writer, topic string) int {
	fmt.Fprint(stderr, topic)
	return 2
}

// repoFor finds the repository the current directory belongs to. The message names
// the command, because "run it somewhere else" is only useful when it says where.
func repoFor(ctx context.Context, stderr io.Writer, advice string) (string, bool) {
	repo, err := lifecycle.RepoRoot(ctx, ".")
	if err != nil {
		fail(stderr, "%s", advice)
		return "", false
	}
	return repo, true
}

func start(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		return misuse(stderr, startHelp)
	}
	agent, extra, err := lifecycle.SplitStartArgs(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return lifecycle.ExitCode(err)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	repo, ok := repoFor(ctx, stderr, "run sluss start inside a Git repository")
	if !ok {
		return 2
	}
	result, err := env.runner.Start(ctx, lifecycle.StartOpts{
		Repo: repo, Name: args[0], Agent: agent, AppName: env.appName, Extra: extra,
	})
	return emit(stdout, stderr, result, err)
}

func list(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		return misuse(stderr, listHelp)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	// Outside a repository the worktree section is simply omitted.
	repo, _ := lifecycle.RepoRoot(ctx, ".")
	result, err := env.runner.List(ctx, repo, env.appName)
	return emit(stdout, stderr, result, err)
}

func attach(ctx context.Context, args []string, streams sbx.Streams, stderr io.Writer) int {
	if len(args) < 1 {
		return misuse(stderr, attachHelp)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	// Attach reads the agent back from sbx, so it needs no worktree context.
	ignoreInterrupt()
	code, err := env.runner.Attach(ctx, env.appName, args[0], streams, args[1:])
	if err != nil {
		return fail(stderr, "%v", err)
	}
	return code
}

func execIn(ctx context.Context, args []string, streams sbx.Streams, stderr io.Writer) int {
	if len(args) < 1 {
		return misuse(stderr, execHelp)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	ignoreInterrupt()
	code, err := env.runner.ExecIn(ctx, env.appName, args[0], streams, args[1:])
	if err != nil {
		return fail(stderr, "%v", err)
	}
	return code
}

// ignoreInterrupt stops sluss reacting to Ctrl-C while a child owns the terminal.
//
// A terminal delivers SIGINT to the whole foreground process group, so the child
// already receives it. Without this sluss would die too, leaving the child writing
// to a terminal whose shell has taken the prompt back. The change is process-wide
// and never undone, which is fine because the only thing sluss does afterwards is
// report the child's exit code and exit.
func ignoreInterrupt() {
	signal.Ignore(os.Interrupt)
}

func stop(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		return misuse(stderr, stopHelp)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	result, err := env.runner.Stop(ctx, "", env.appName, args...)
	return emit(stdout, stderr, result, err)
}

func destroy(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || len(args) > 2 {
		return misuse(stderr, destroyHelp)
	}
	force := false
	if len(args) == 2 {
		if args[1] != "--force" {
			return misuse(stderr, destroyHelp)
		}
		force = true
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	repo, ok := repoFor(ctx, stderr, "run sluss destroy inside the repository or one of its worktrees")
	if !ok {
		return 2
	}
	result, err := env.runner.Destroy(ctx, repo, env.appName, args[0], force)
	return emit(stdout, stderr, result, err)
}

func path(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		return misuse(stderr, pathHelp)
	}
	env, err := loadEnvironment()
	if err != nil {
		return fail(stderr, "%v", err)
	}
	repo, ok := repoFor(ctx, stderr, "run sluss path inside the repository or one of its worktrees")
	if !ok {
		return 2
	}
	result, err := env.runner.Path(repo, args[0])
	return emit(stdout, stderr, result, err)
}

func serve(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", config.DefaultPath(), "configuration file")
	interval := flags.Duration("interval", 3*time.Second, "how often to poll every scope")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	// Bind before anything else: a NAS that silently fell back to loopback would
	// look healthy and be unreachable, so a failure here is fatal and says which
	// address was tried.
	listener, err := net.Listen("tcp", cfg.BindAddr())
	if err != nil {
		fmt.Fprintf(stderr, "cannot bind %s: %v\n", cfg.BindAddr(), err)
		return 1
	}

	client := &sbx.Client{}
	poller := fleet.New(cfg, client, *interval)
	runner := &lifecycle.Runner{Sbx: client, WorktreeRoot: cfg.WorktreeRoot}
	handler := server.New(cfg, poller, runner, client, web.Assets).Handler()

	// SIGINT and SIGTERM both mean "stop": Ctrl-C on the laptop, systemd on the VM.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	go func() { _ = poller.Run(ctx) }()

	httpServer := &http.Server{Handler: handler}
	serverDone := make(chan error, 1)
	go func() { serverDone <- httpServer.Serve(listener) }()

	fmt.Fprintf(stdout, "sluss %s serving on http://%s (access: %s)\n", version, cfg.BindAddr(), cfg.Access)

	select {
	case err := <-serverDone:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(stderr, err)
			return 1
		}
	case <-ctx.Done():
		// Streaming responses never end on their own, so give shutdown a short
		// deadline and then close them.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
		}
	}
	return 0
}

func diagnose(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", config.DefaultPath(), "configuration file")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	report := doctor.Run(ctx, cfg, &sbx.Client{})
	for _, check := range report.Checks {
		fmt.Fprintf(stdout, "%-4s %-28s %s\n", check.State, check.Name, check.Detail)
	}
	if report.Failed() {
		return 1
	}
	return 0
}
