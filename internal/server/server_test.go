package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/lifecycle"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
	"github.com/waldemarsson/sluss/internal/server"
)

// fakeRunner records what the routes asked lifecycle to do, so these tests assert
// on the request/response contract — guards, status codes, JSON shape — rather than
// on git behaviour, which internal/lifecycle already covers against a real
// repository. It records in the shape a person would type, because that is what the
// assertions are actually about.
type fakeRunner struct {
	mu     sync.Mutex
	log    strings.Builder
	delay  time.Duration
	marker string
}

func (f *fakeRunner) record(command, repo string) {
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fmt.Fprintf(&f.log, "%s\npwd=%s\n", command, repo)
	if f.marker != "" {
		_ = os.WriteFile(f.marker, nil, 0o644)
	}
}

func (f *fakeRunner) recorded() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.log.String()
}

func (f *fakeRunner) Start(_ context.Context, opts lifecycle.StartOpts) (lifecycle.Result, error) {
	command := "start " + opts.Name
	if len(opts.Extra) > 0 {
		command += " " + strings.Join(opts.Extra, " ")
	}
	f.record(command, opts.Repo)
	return lifecycle.Result{}, nil
}

func (f *fakeRunner) Stop(_ context.Context, repo, _ string, names ...string) (lifecycle.Result, error) {
	f.record("stop "+strings.Join(names, " "), repo)
	return lifecycle.Result{}, nil
}

// Destroy stands in for the real refusal on dirty or unmerged work: without force
// it declines, and that refusal is what the routes must surface unchanged.
func (f *fakeRunner) Destroy(_ context.Context, repo, _, name string, force bool) (lifecycle.Result, error) {
	command := "destroy " + name
	if force {
		command += " --force"
	}
	f.record(command, repo)
	if force {
		return lifecycle.Result{}, nil
	}
	return lifecycle.Result{
		ExitCode: 1,
		Stderr:   "error: agent/" + name + " has uncommitted changes; commit them or use --force\n",
	}, nil
}

// runner installs a fresh recorder for one test and returns it.
func runner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{}
	currentRunner = f
	t.Cleanup(func() { currentRunner = nil })
	return f
}

// currentRunner is what recordedArgv reads, so the existing assertions keep their
// shape without every test threading the recorder through.
var currentRunner *fakeRunner

func recordedArgv(t *testing.T) string {
	t.Helper()
	if currentRunner == nil {
		return ""
	}
	return currentRunner.recorded()
}

// Shaped like a real dashboard build: the nested routes prerender to a directory
// index, because that is the only nested form a plain file server resolves.
var assets = fstest.MapFS{
	"index.html":             &fstest.MapFile{Data: []byte("<title>sluss</title>")},
	"app/main.css":           &fstest.MapFile{Data: []byte("body{}")},
	"config/kits/index.html": &fstest.MapFile{Data: []byte("<title>kits</title>")},
}

// fleetFixture is the sbx listing the server tests run against: two sandboxes in
// one repository, one running with a published web port and one stopped.
func fleetFixture(repo, worktreeRoot string) string {
	return fmt.Sprintf(`{"sandboxes": [
		{"name": "web", "id": "d1e0", "agent": "opencode", "status": "stopped",
		 "workspaces": [%q, %q]},
		{"name": "auth", "id": "50fe", "agent": "claude", "status": "running",
		 "ports": [{"host_ip":"127.0.0.1","host_port":49155,"sandbox_port":4096,"protocol":"tcp"}],
		 "workspaces": [%q, %q]}
	]}`,
		filepath.Join(worktreeRoot, "mercurius", "web"), filepath.Join(repo, ".git"),
		filepath.Join(worktreeRoot, "mercurius", "auth"), filepath.Join(repo, ".git"))
}

// serve starts a running poller behind a test server and returns its base URL. The
// repository exists on disk because the script runs with it as its working
// directory; its git state does not matter here.
func serve(t *testing.T) (string, *fleet.Poller) {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "mercurius")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("creating %s: %v", repo, err)
	}
	worktreeRoot := filepath.Join(base, "worktrees")

	stub := sbxstub.Install(t, fleetFixture(repo, worktreeRoot))
	cfg := &config.Config{
		AppNames: []string{"personal"}, WorktreeRoot: worktreeRoot,
		Repos: []string{repo},
	}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = poller.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	// Wait for the first snapshot so tests are not racing the poller.
	deadline := time.After(2 * time.Second)
	for poller.Current() == nil {
		select {
		case <-deadline:
			t.Fatal("poller produced no snapshot")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{Bin: stub.Bin}, assets).Handler())
	t.Cleanup(srv.Close)
	t.Setenv("SLUSS_TEST_REPO", repo)
	return srv.URL, poller
}

func TestFleetEndpoint(t *testing.T) {
	base, _ := serve(t)

	resp, err := http.Get(base + "/api/fleet")
	if err != nil {
		t.Fatalf("GET /api/fleet: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	var snap fleet.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}
	sandbox, ok := snap.Find("personal", "auth")
	if !ok {
		t.Fatalf("personal/auth missing from %+v", snap.Repos)
	}
	// Every field the dashboard table binds to must survive the round trip.
	if sandbox.Agent != "claude" || sandbox.Status != "running" || sandbox.WebPort != 49155 {
		t.Errorf("sandbox = %+v", sandbox)
	}
	if sandbox.RepoName != "mercurius" {
		t.Errorf("RepoName = %q, want mercurius", sandbox.RepoName)
	}
}

func TestFleetBeforeFirstPoll(t *testing.T) {
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}
	poller := fleet.New(cfg, &sbx.Client{}, time.Second) // never run
	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{}, assets).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/fleet")
	if err != nil {
		t.Fatalf("GET /api/fleet: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 before the first poll", resp.StatusCode)
	}
}

func TestEventsStream(t *testing.T) {
	base, _ := serve(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		t.Errorf("Content-Length = %q on a stream; it must not be set", cl)
	}

	// The first event arrives immediately, before any new poll: the stream opens
	// with the current snapshot rather than an empty page.
	scanner := bufio.NewScanner(resp.Body)
	var data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if data == "" {
		t.Fatalf("no data line in the stream: %v", scanner.Err())
	}
	var snap fleet.Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		t.Fatalf("decoding event: %v", err)
	}
	if _, ok := snap.Find("personal", "auth"); !ok {
		t.Errorf("first event carried no fleet: %+v", snap)
	}

	// A second event follows on the next poll without reconnecting.
	events := 0
	for scanner.Scan() && events == 0 {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			events++
		}
	}
	if events == 0 {
		t.Error("stream delivered only one snapshot")
	}
}

func TestEventsEndsWhenTheClientDisconnects(t *testing.T) {
	base, poller := serve(t)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/events: %v", err)
	}
	cancel()
	resp.Body.Close()

	// The handler must unsubscribe on the way out; the poller keeps running, which
	// is what a leaked subscriber would eventually stall.
	time.Sleep(50 * time.Millisecond)
	if poller.Current() == nil {
		t.Error("poller stopped after a client disconnected")
	}
}

func TestUnknownAPIPath(t *testing.T) {
	base, _ := serve(t)

	resp, err := http.Get(base + "/api/nope")
	if err != nil {
		t.Fatalf("GET /api/nope: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if !strings.Contains(body["error"], "/api/nope") {
		t.Errorf("error body = %v, want it to name the path", body)
	}
}

func TestStaticAssets(t *testing.T) {
	base, _ := serve(t)

	tests := []struct {
		path string
		code int
		body string
	}{
		{"/", http.StatusOK, "<title>sluss</title>"},
		{"/app/main.css", http.StatusOK, "body{}"},
		{"/missing.js", http.StatusNotFound, ""},
		// A bookmarked configuration route, with and without the trailing slash the
		// dashboard's own links carry. Without the slash the file server redirects,
		// which http.Get follows, so both land on the same page.
		{"/config/kits/", http.StatusOK, "<title>kits</title>"},
		{"/config/kits", http.StatusOK, "<title>kits</title>"},
		{"/config/nope/", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get(base + tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.code {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.code)
			}
			if tt.body == "" {
				return
			}
			buf := make([]byte, len(tt.body))
			if _, err := resp.Body.Read(buf); err != nil && err.Error() != "EOF" {
				t.Fatalf("reading body: %v", err)
			}
			if string(buf) != tt.body {
				t.Errorf("body = %q, want %q", buf, tt.body)
			}
		})
	}
}

func TestConfigEndpoint(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{
		AppNames: []string{"personal"}, WorktreeRoot: "/w",
		Access: "host", HostPrefix: "sluss-", Domain: "example.test",
		Repos: []string{"/src/secret-repo"},
	}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Second)
	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{Bin: stub.Bin}, assets).Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer resp.Body.Close()

	var got struct {
		Access     string   `json:"access"`
		HostPrefix string   `json:"hostPrefix"`
		Domain     string   `json:"domain"`
		Repos      []string `json:"repos"`
		Scopes     []string `json:"scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decoding config: %v", err)
	}
	if got.Access != "host" || got.HostPrefix != "sluss-" || got.Domain != "example.test" {
		t.Errorf("routing fields = %+v", got)
	}
	if len(got.Repos) != 1 || got.Repos[0] != "/src/secret-repo" || len(got.Scopes) != 1 {
		t.Errorf("create-form fields = %+v", got)
	}

	// Nothing beyond those five fields is exposed.
	var raw map[string]any
	body, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config: %v", err)
	}
	defer body.Body.Close()
	if err := json.NewDecoder(body.Body).Decode(&raw); err != nil {
		t.Fatalf("decoding config: %v", err)
	}
	if len(raw) != 5 {
		t.Errorf("config exposed %d fields: %v", len(raw), raw)
	}
}

func TestProxyIsMountedInPathMode(t *testing.T) {
	base, _ := serve(t) // "auth" is running but publishes 49155, nothing listens

	resp, err := http.Get(base + "/s/personal/ghost/")
	if err != nil {
		t.Fatalf("GET a sandbox route: %v", err)
	}
	defer resp.Body.Close()

	// 404 from sluss's own router, not the file server's plain 404 page.
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no sandbox") {
		t.Errorf("body = %q, want the router's message", body)
	}
}

func TestHostModeLeavesTheDashboardReachable(t *testing.T) {
	stub := sbxstub.Install(t, sbxstub.Fixture)
	cfg := &config.Config{
		AppNames: []string{"personal"}, WorktreeRoot: "/w",
		Access: config.AccessHost, HostPrefix: "sluss-", Domain: "example.test",
	}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = poller.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	for poller.Current() == nil {
		time.Sleep(time.Millisecond)
	}

	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{Bin: stub.Bin}, assets).Handler())
	defer srv.Close()

	// The dashboard's own hostname is not a sandbox address, so it still gets the UI.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET the dashboard in host mode: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want the dashboard", resp.StatusCode)
	}

	// A sandbox hostname is routed instead.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Host = "sluss-ghost.example.test"
	sandboxResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET a sandbox hostname: %v", err)
	}
	defer sandboxResp.Body.Close()
	if sandboxResp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 from the router", sandboxResp.StatusCode)
	}
}

func postJSON(t *testing.T, url string, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func TestCreateRefusesUnconfiguredTargets(t *testing.T) {
	base, _ := serve(t)

	tests := []struct {
		name string
		body string
	}{
		{"unknown repo", `{"repo":"/etc","name":"web","agent":"opencode","scope":"personal"}`},
		{"unknown scope", `{"repo":"` + os.Getenv("SLUSS_TEST_REPO") + `","name":"web","agent":"opencode","scope":"stranger"}`},
		{"malformed", `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := postJSON(t, base+"/api/sandboxes", tt.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
		})
	}
	if argv := recordedArgv(t); argv != "" {
		t.Errorf("the script ran anyway: %q", argv)
	}
}

func TestCreateRunsTheScript(t *testing.T) {
	base, _ := serve(t)

	resp := postJSON(t, base+"/api/sandboxes",
		`{"repo":"`+os.Getenv("SLUSS_TEST_REPO")+`","name":"budget","agent":"claude","scope":"personal","extra":["--cpus","4"]}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body %s, want 200", resp.StatusCode, body)
	}
	if argv := recordedArgv(t); !strings.Contains(argv, "start budget --cpus 4") {
		t.Errorf("script argv = %q, want the start command with its extra arguments", argv)
	}
}

func TestStopRunsTheScript(t *testing.T) {
	base, _ := serve(t)

	resp := postJSON(t, base+"/api/sandboxes/personal/auth/stop", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body %s, want 200", resp.StatusCode, body)
	}
	if argv := recordedArgv(t); !strings.Contains(argv, "stop auth") {
		t.Errorf("script argv = %q, want the stop command", argv)
	}
}

func TestDestroySurfacesTheScriptRefusal(t *testing.T) {
	base, _ := serve(t)

	req, err := http.NewRequest(http.MethodDelete, base+"/api/sandboxes/personal/auth", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for a refusal", resp.StatusCode)
	}
	var result struct {
		ExitCode int    `json:"exitCode"`
		Stderr   string `json:"stderr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding result: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("exit code 0 on a refusal")
	}
	// The script's own wording reaches the user unchanged.
	if !strings.Contains(result.Stderr, "uncommitted changes; commit them or use --force") {
		t.Errorf("stderr = %q, want the script's message verbatim", result.Stderr)
	}
	if argv := recordedArgv(t); strings.Contains(argv, "--force") {
		t.Errorf("script argv = %q, want no --force from the GUI", argv)
	}
}

func TestLifecycleOnAnUnknownSandbox(t *testing.T) {
	base, _ := serve(t)

	resp := postJSON(t, base+"/api/sandboxes/personal/ghost/stop", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}

	scoped := postJSON(t, base+"/api/sandboxes/stranger/auth/stop", "")
	defer scoped.Body.Close()
	if scoped.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d for an unconfigured scope, want 400", scoped.StatusCode)
	}

	started := postJSON(t, base+"/api/sandboxes/personal/ghost/start", "")
	defer started.Body.Close()
	if started.StatusCode != http.StatusNotFound {
		t.Errorf("start status = %d, want 404", started.StatusCode)
	}
	if argv := recordedArgv(t); strings.Contains(argv, "start ghost") {
		t.Errorf("script argv = %q, want no run for an unknown sandbox", argv)
	}
}

// The fixture's "web" is stopped, which is the only state the dashboard offers start
// for. The repository is taken from the snapshot, never from the request: the script
// must run in the configured checkout even though the caller named no path.
func TestStartResumesAKnownSandbox(t *testing.T) {
	base, _ := serve(t)

	resp := postJSON(t, base+"/api/sandboxes/personal/web/start", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body %s, want 200", resp.StatusCode, body)
	}
	argv := recordedArgv(t)
	if !strings.Contains(argv, "start web") {
		t.Errorf("script argv = %q, want the start command", argv)
	}
	if want := "pwd=" + os.Getenv("SLUSS_TEST_REPO"); !strings.Contains(argv, want) {
		t.Errorf("recording = %q, want the script run in %q", argv, want)
	}
}

// The dashboard does not offer start for a running sandbox, but the route must not
// punish one that arrives anyway — a second tab, or a click racing the poll (AC7).
func TestStartOnARunningSandboxIsHarmless(t *testing.T) {
	base, _ := serve(t)

	resp := postJSON(t, base+"/api/sandboxes/personal/auth/start", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body %s, want 200", resp.StatusCode, body)
	}
	if argv := recordedArgv(t); !strings.Contains(argv, "start auth") {
		t.Errorf("script argv = %q, want the start command", argv)
	}
}

// force is opt-in and spelled exactly: anything else is not a licence to discard
// work, so a typo in the query string degrades to the safe path.
func TestDestroyForceIsExplicit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		query  string
		forced bool
		status int
	}{
		{"absent", "", false, http.StatusConflict},
		{"true", "?force=true", true, http.StatusOK},
		{"one", "?force=1", false, http.StatusConflict},
		{"yes", "?force=yes", false, http.StatusConflict},
		{"empty", "?force=", false, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, _ := serve(t)

			req, err := http.NewRequest(http.MethodDelete, base+"/api/sandboxes/personal/auth"+tc.query, nil)
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("DELETE: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.status {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if forced := strings.Contains(recordedArgv(t), "--force"); forced != tc.forced {
				t.Errorf("argv = %q, --force present = %v, want %v", recordedArgv(t), forced, tc.forced)
			}
		})
	}
}

// secretServer wires a server whose sbx stub records everything it is asked.
func secretServer(t *testing.T) (string, *sbxstub.Stub) {
	t.Helper()
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w"}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Hour)
	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{Bin: stub.Bin}, assets).Handler())
	t.Cleanup(srv.Close)
	return srv.URL, stub
}

func TestSecretsListNamesOnly(t *testing.T) {
	base, stub := secretServer(t)
	stub.Secrets(t, "GITHUB_TOKEN\nANTHROPIC_API_KEY\n")

	resp, err := http.Get(base + "/api/secrets/personal")
	if err != nil {
		t.Fatalf("GET secrets: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(body.Names) != 2 || body.Names[0] != "GITHUB_TOKEN" {
		t.Errorf("names = %v", body.Names)
	}
	if calls := strings.Join(stub.Calls(t), "\n"); !strings.Contains(calls, "--app-name personal secret ls") {
		t.Errorf("sbx calls = %q, want a scoped secret listing", calls)
	}
}

func TestSecretSetNeverReturnsOrLogsTheValue(t *testing.T) {
	base, stub := secretServer(t)

	req, err := http.NewRequest(http.MethodPut, base+"/api/secrets/personal/GITHUB_TOKEN", strings.NewReader("ghp_supersecret"))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT secret: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "ghp_supersecret") {
		t.Fatal("the response echoed the secret value")
	}
	for _, call := range stub.Calls(t) {
		if strings.Contains(call, "ghp_supersecret") {
			t.Fatalf("the value reached argv: %q", call)
		}
	}
	if got := stub.Stdin(t, 1); got != "ghp_supersecret" {
		t.Errorf("sbx stdin = %q, want the value piped in", got)
	}
}

func TestSecretRoutesValidateInput(t *testing.T) {
	base, _ := secretServer(t)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"unknown scope", http.MethodGet, "/api/secrets/stranger", "", http.StatusBadRequest},
		{"dotted name", http.MethodPut, "/api/secrets/personal/A.B", "x", http.StatusBadRequest},
		{"shell-ish name", http.MethodPut, "/api/secrets/personal/A;B", "x", http.StatusBadRequest},
		{"empty value", http.MethodPut, "/api/secrets/personal/TOKEN", "", http.StatusBadRequest},
		{"delete bad name", http.MethodDelete, "/api/secrets/personal/A B", "", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, base+tt.path, strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tt.method, tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestSecretDelete(t *testing.T) {
	base, stub := secretServer(t)

	req, err := http.NewRequest(http.MethodDelete, base+"/api/secrets/personal/GITHUB_TOKEN", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE secret: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if calls := strings.Join(stub.Calls(t), "\n"); !strings.Contains(calls, "--app-name personal secret rm GITHUB_TOKEN") {
		t.Errorf("sbx calls = %q", calls)
	}
}

func TestSecretFailureIsReadable(t *testing.T) {
	base, stub := secretServer(t)
	stub.Fail(t, 1, "Error: not logged in")

	resp, err := http.Get(base + "/api/secrets/personal")
	if err != nil {
		t.Fatalf("GET secrets: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if !strings.Contains(body["error"], "login") {
		t.Errorf("error = %q, want the mapped hint", body["error"])
	}
}

// kitServer wires a server over a temporary kits directory.
func kitServer(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "dotnet-svelte"), 0o755); err != nil {
		t.Fatalf("creating kit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dotnet-svelte", "spec.yaml"), []byte("name: dotnet\n"), 0o644); err != nil {
		t.Fatalf("writing spec: %v", err)
	}

	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", KitsDir: dir}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Hour)
	srv := httptest.NewServer(server.New(cfg, poller, runner(t), &sbx.Client{Bin: stub.Bin}, assets).Handler())
	t.Cleanup(srv.Close)
	return srv.URL, dir
}

func TestKitListing(t *testing.T) {
	base, _ := kitServer(t)

	resp, err := http.Get(base + "/api/kits")
	if err != nil {
		t.Fatalf("GET /api/kits: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Kits []struct {
			Name    string `json:"name"`
			HasSpec bool   `json:"hasSpec"`
		} `json:"kits"`
		Status struct {
			Repository bool   `json:"repository"`
			Dirty      bool   `json:"dirty"`
			Detail     string `json:"detail"`
		} `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(body.Kits) != 1 || body.Kits[0].Name != "dotnet-svelte" || !body.Kits[0].HasSpec {
		t.Errorf("kits = %+v", body.Kits)
	}
	if body.Status.Repository {
		t.Errorf("status = %+v, want a plain directory reported as no repository", body.Status)
	}
}

func TestKitSpecRoundTrip(t *testing.T) {
	base, dir := kitServer(t)
	body := "name: dotnet-svelte\r\ntools:\n\t- dotnet\n\n"

	req, err := http.NewRequest(http.MethodPut, base+"/api/kits/dotnet-svelte/spec", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT spec: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}

	onDisk, err := os.ReadFile(filepath.Join(dir, "dotnet-svelte", "spec.yaml"))
	if err != nil {
		t.Fatalf("reading spec: %v", err)
	}
	if string(onDisk) != body {
		t.Errorf("on disk = %q, want %q", onDisk, body)
	}

	get, err := http.Get(base + "/api/kits/dotnet-svelte/spec")
	if err != nil {
		t.Fatalf("GET spec: %v", err)
	}
	defer get.Body.Close()
	served, _ := io.ReadAll(get.Body)
	if string(served) != body {
		t.Errorf("served = %q, want %q", served, body)
	}
	if ct := get.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

func TestKitRoutesRefuseBadNames(t *testing.T) {
	base, _ := kitServer(t)

	resp, err := http.Get(base + "/api/kits/not-a-kit/spec")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	// Reading a kit that has no spec yet is allowed; writing to one that does not
	// exist is not.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d for a specless kit, want 200", resp.StatusCode)
	}

	req, err := http.NewRequest(http.MethodPut, base+"/api/kits/not-a-kit/spec", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	write, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	write.Body.Close()
	if write.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d writing an unknown kit, want 404", write.StatusCode)
	}
}

// slowRunner takes long enough for a client to give up mid-run, and records that
// it finished anyway.
func slowRunner(t *testing.T) (*fakeRunner, string) {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "finished")
	return &fakeRunner{delay: 400 * time.Millisecond, marker: marker}, marker
}

// A browser that disconnects mid-create must not cancel the run: it would be
// abandoned between "git worktree add" and "sbx create", and the unwind only runs
// when sbx itself fails.
func TestLifecycleOutlivesTheRequestThatStartedIt(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "mercurius")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("creating %s: %v", repo, err)
	}
	stub := sbxstub.Install(t, sbxstub.Empty)
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Repos: []string{repo}}
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, time.Hour)
	runner, marker := slowRunner(t)
	srv := httptest.NewServer(server.New(cfg, poller, runner, &sbx.Client{Bin: stub.Bin}, assets).Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/api/sandboxes",
		strings.NewReader(`{"repo":"`+repo+`","name":"budget","agent":"opencode","scope":"personal"}`))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(50 * time.Millisecond)
	cancel() // the tab closes while the run is still going
	<-done

	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			return // the run completed
		}
		select {
		case <-deadline:
			t.Fatal("the run was abandoned when the client disconnected")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestMutatingRoutesRefuseCrossSiteRequests(t *testing.T) {
	base, _ := serve(t)
	repo := os.Getenv("SLUSS_TEST_REPO")

	tests := []struct {
		name        string
		method      string
		path        string
		contentType string
		fetchSite   string
		want        int
	}{
		{"cross-site create", http.MethodPost, "/api/sandboxes", "application/json", "cross-site", http.StatusForbidden},
		{"cross-site destroy", http.MethodDelete, "/api/sandboxes/personal/auth", "", "cross-site", http.StatusForbidden},
		{"cross-site start", http.MethodPost, "/api/sandboxes/personal/web/start", "application/json", "cross-site", http.StatusForbidden},
		{"cross-site secret", http.MethodPut, "/api/secrets/personal/TOKEN", "text/plain", "cross-site", http.StatusForbidden},
		{"simple-request create", http.MethodPost, "/api/sandboxes", "text/plain", "", http.StatusUnsupportedMediaType},
		{"form-encoded create", http.MethodPost, "/api/sandboxes", "application/x-www-form-urlencoded", "", http.StatusUnsupportedMediaType},
		{"same-origin create", http.MethodPost, "/api/sandboxes", "application/json", "same-origin", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"repo":"` + repo + `","name":"budget","agent":"opencode","scope":"personal"}`
			req, err := http.NewRequest(tt.method, base+tt.path, strings.NewReader(body))
			if err != nil {
				t.Fatalf("building request: %v", err)
			}
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tt.fetchSite)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tt.method, tt.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				t.Errorf("status = %d, want %d", resp.StatusCode, tt.want)
			}
		})
	}

	// Reading the fleet from another site is not blocked here — the browser's own
	// same-origin policy stops it being read — so the guard stays on mutations.
	req, err := http.NewRequest(http.MethodGet, base+"/api/fleet", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/fleet: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want reads to stay unguarded", resp.StatusCode)
	}
}

func TestCreateRefusesArbitrarySbxFlags(t *testing.T) {
	base, _ := serve(t)
	repo := os.Getenv("SLUSS_TEST_REPO")

	tests := []struct {
		name  string
		extra string
		want  int
	}{
		{"sizing flags", `["--cpus","4","--memory","8"]`, http.StatusOK},
		{"extra workspace", `["--workspace","/etc"]`, http.StatusBadRequest},
		{"extra publish", `["--publish","2222"]`, http.StatusBadRequest},
		{"value without a flag", `["4"]`, http.StatusBadRequest},
		{"non-numeric size", `["--cpus","$(whoami)"]`, http.StatusBadRequest},
		{"missing value", `["--cpus"]`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"repo":"` + repo + `","name":"budget","agent":"opencode","scope":"personal","extra":` + tt.extra + `}`
			resp := postJSON(t, base+"/api/sandboxes", body)
			defer resp.Body.Close()
			if resp.StatusCode != tt.want {
				out, _ := io.ReadAll(resp.Body)
				t.Errorf("status = %d body %s, want %d", resp.StatusCode, out, tt.want)
			}
		})
	}
}
