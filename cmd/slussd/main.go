// Command slussd is the GUI control plane over Docker Sandboxes (sbx).
//
// It is a separate binary from the sluss script on purpose: `sluss` is the shell
// lifecycle tool this daemon calls, and two things of the same name on PATH would
// make which one runs depend on install order.
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
	"syscall"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/doctor"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/script"
	"github.com/waldemarsson/sluss/internal/server"
	"github.com/waldemarsson/sluss/web"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
//
// A package-level var (not a const) is what makes -X able to write to it.
var version = "dev"

const usage = `slussd - GUI control plane over sbx

Usage:
  slussd serve [-config PATH] [-interval DURATION]
  slussd doctor [-config PATH]
  slussd version

The sandbox lifecycle itself belongs to the sluss script, which this daemon runs.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is separated from main so tests can drive it without spawning a process.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "serve":
		return serve(args[1:], stdout, stderr)
	case "doctor":
		return diagnose(args[1:], stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
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
	scriptPath, err := script.ResolveScript()
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
	runner := &script.Runner{Script: scriptPath, WorktreeRoot: cfg.WorktreeRoot}
	handler := server.New(cfg, poller, runner, client, web.Assets).Handler()

	// SIGINT and SIGTERM both mean "stop": Ctrl-C on the laptop, systemd on the VM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	scriptPath, scriptErr := script.ResolveScript()
	report := doctor.Run(ctx, cfg, &sbx.Client{}, scriptPath, scriptErr)

	for _, check := range report.Checks {
		fmt.Fprintf(stdout, "%-4s %-28s %s\n", check.State, check.Name, check.Detail)
	}
	if report.Failed() {
		return 1
	}
	return 0
}
