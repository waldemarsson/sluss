package proxy_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/proxy"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/sbxstub"
)

// fixture renders a listing whose published port points at a real test server.
func fixture(name, status string, hostPort int) string {
	ports := ""
	if hostPort != 0 {
		ports = fmt.Sprintf(`"ports": [{"host_ip":"127.0.0.1","host_port":%d,"sandbox_port":4096,"protocol":"tcp"}],`, hostPort)
	}
	return fmt.Sprintf(`{"sandboxes": [{"name": %q, "id": "id", "agent": "opencode", "status": %q, %s
		"workspaces": ["/absent/worktrees/repo/%s", "/absent/repo/.git"]}]}`, name, status, ports, name)
}

func portOf(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing %s: %v", rawURL, err)
	}
	var port int
	if _, err := fmt.Sscanf(u.Port(), "%d", &port); err != nil {
		t.Fatalf("parsing port of %s: %v", rawURL, err)
	}
	return port
}

// routed starts a poller over a stubbed sbx and returns a live sluss front door.
func routed(t *testing.T, cfg *config.Config, stub *sbxstub.Stub) *httptest.Server {
	t.Helper()
	poller := fleet.New(cfg, &sbx.Client{Bin: stub.Bin}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = poller.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	waitForSnapshot(t, poller)

	front := httptest.NewServer(proxy.New(cfg, poller).Handler())
	t.Cleanup(front.Close)
	return front
}

func waitForSnapshot(t *testing.T, poller *fleet.Poller) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for poller.Current() == nil {
		select {
		case <-deadline:
			t.Fatal("poller produced no snapshot")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// noRedirect keeps the client from following the trailing-slash redirect.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}}

func TestPathModeStripsThePrefix(t *testing.T) {
	var seen string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RequestURI()
		fmt.Fprint(w, "opencode")
	}))
	defer upstream.Close()

	stub := sbxstub.Install(t, fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Access: config.AccessPath}
	front := routed(t, cfg, stub)

	resp, err := http.Get(front.URL + "/s/personal/web/assets/app.js?v=1")
	if err != nil {
		t.Fatalf("GET through the proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK || string(body) != "opencode" {
		t.Fatalf("status %d body %q", resp.StatusCode, body)
	}
	if seen != "/assets/app.js?v=1" {
		t.Errorf("upstream saw %q, want the prefix stripped and the query kept", seen)
	}
}

func TestPathModeAddsTheTrailingSlash(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	stub := sbxstub.Install(t, fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Access: config.AccessPath}
	front := routed(t, cfg, stub)

	resp, err := noRedirect.Get(front.URL + "/s/personal/web")
	if err != nil {
		t.Fatalf("GET without a trailing slash: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want 308", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/s/personal/web/" {
		t.Errorf("Location = %q, want /s/personal/web/", got)
	}

	// The query is part of the address the app was given; the redirect must keep it.
	withQuery, err := noRedirect.Get(front.URL + "/s/personal/web?tab=chat&id=7")
	if err != nil {
		t.Fatalf("GET with a query: %v", err)
	}
	defer withQuery.Body.Close()
	if got := withQuery.Header.Get("Location"); got != "/s/personal/web/?tab=chat&id=7" {
		t.Errorf("Location = %q, want the query preserved", got)
	}
}

func TestUnknownAndStoppedSandboxes(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	tests := []struct {
		name    string
		fixture string
		path    string
		want    string
	}{
		{"unknown name", fixture("web", "running", portOf(t, upstream.URL)), "/s/personal/ghost/", "no sandbox \"ghost\""},
		{"unknown scope", fixture("web", "running", portOf(t, upstream.URL)), "/s/other/web/", "no sandbox \"web\""},
		{"stopped", fixture("web", "stopped", 0), "/s/personal/web/", "publishes no web port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := sbxstub.Install(t, tt.fixture)
			cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Access: config.AccessPath}
			front := routed(t, cfg, stub)

			done := make(chan struct{})
			var resp *http.Response
			var err error
			go func() { resp, err = http.Get(front.URL + tt.path); close(done) }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("request hung instead of failing fast")
			}
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", resp.StatusCode)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tt.want) {
				t.Errorf("body = %q, want it to mention %q", body, tt.want)
			}
		})
	}
}

func TestVanishedSandboxStopsResolving(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	stub := sbxstub.Install(t, sbxstub.Empty)
	stub.ScopeFixture(t, "personal", fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Access: config.AccessPath}
	front := routed(t, cfg, stub)

	resp, err := http.Get(front.URL + "/s/personal/web/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d before removal, want 200", resp.StatusCode)
	}

	stub.ScopeFixture(t, "personal", sbxstub.Empty) // "sbx rm -f web" by hand

	deadline := time.After(3 * time.Second)
	for {
		resp, err := http.Get(front.URL + "/s/personal/web/")
		if err != nil {
			t.Fatalf("GET after removal: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return
		}
		select {
		case <-deadline:
			t.Fatal("route still resolving after the sandbox was removed")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestHostModeServesRootAndKeepsThePath(t *testing.T) {
	var seen string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
	}))
	defer upstream.Close()

	stub := sbxstub.Install(t, fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{
		AppNames: []string{"personal"}, WorktreeRoot: "/w",
		Access: config.AccessHost, HostPrefix: "sluss-", Domain: "example.test",
	}
	front := routed(t, cfg, stub)

	req, err := http.NewRequest(http.MethodGet, front.URL+"/assets/app.js", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Host = "sluss-web.example.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET in host mode: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if seen != "/assets/app.js" {
		t.Errorf("upstream saw %q, want the path untouched", seen)
	}
}

func TestHostModeRefusesAnAmbiguousName(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer upstream.Close()

	// The same fixture answers for both scopes, so "web" exists twice.
	stub := sbxstub.Install(t, fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{
		AppNames: []string{"personal", "work"}, WorktreeRoot: "/w",
		Access: config.AccessHost, HostPrefix: "sluss-", Domain: "example.test",
	}
	front := routed(t, cfg, stub)

	req, err := http.NewRequest(http.MethodGet, front.URL+"/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Host = "sluss-web.example.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "personal") || !strings.Contains(string(body), "work") {
		t.Errorf("body = %q, want it to name both scopes", body)
	}
}

func TestStreamingIsNotBuffered(t *testing.T) {
	// The upstream holds the response open until the test says it has read the
	// first chunk. If the proxy buffered, that read would never complete and the
	// test would fail on its own timeout rather than on a wall-clock guess.
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: first\n\n")
		http.NewResponseController(w).Flush()
		<-release
		fmt.Fprint(w, "data: second\n\n")
	}))
	defer func() {
		close(release)
		upstream.Close()
	}()

	stub := sbxstub.Install(t, fixture("web", "running", portOf(t, upstream.URL)))
	cfg := &config.Config{AppNames: []string{"personal"}, WorktreeRoot: "/w", Access: config.AccessPath}
	front := routed(t, cfg, stub)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(front.URL + "/s/personal/web/events")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, len("data: first\n\n"))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("reading the first chunk before the response ended: %v", err)
	}
	if string(buf) != "data: first\n\n" {
		t.Errorf("first chunk = %q", buf)
	}
}

func TestMatch(t *testing.T) {
	pathMode := proxy.New(&config.Config{Access: config.AccessPath}, nil)
	hostMode := proxy.New(&config.Config{
		Access: config.AccessHost, HostPrefix: "sluss-", Domain: "example.test",
	}, nil)

	tests := []struct {
		name                     string
		router                   *proxy.Router
		host, path               string
		wantOK                   bool
		wantScope, wantName, res string
	}{
		{"path root", pathMode, "sluss.local", "/s/personal/web/", true, "personal", "web", "/"},
		{"path deep", pathMode, "sluss.local", "/s/personal/web/a/b", true, "personal", "web", "/a/b"},
		{"path no slash", pathMode, "sluss.local", "/s/personal/web", true, "personal", "web", "/"},
		{"path missing name", pathMode, "sluss.local", "/s/personal", false, "", "", ""},
		{"path dashboard", pathMode, "sluss.local", "/api/fleet", false, "", "", ""},
		{"host match", hostMode, "sluss-web.example.test", "/x", true, "", "web", "/x"},
		{"host with port", hostMode, "sluss-web.example.test:8420", "/", true, "", "web", "/"},
		{"host wrong domain", hostMode, "sluss-web.other.test", "/", false, "", "", ""},
		{"host extra label", hostMode, "sluss-web.sub.example.test", "/", false, "", "", ""},
		{"host no prefix", hostMode, "web.example.test", "/", false, "", "", ""},
		{"host bare domain", hostMode, "sluss-.example.test", "/", false, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, name, rest, ok := tt.router.Match(tt.host, tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if scope != tt.wantScope || name != tt.wantName || rest != tt.res {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", scope, name, rest, tt.wantScope, tt.wantName, tt.res)
			}
		})
	}
}
