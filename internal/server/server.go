// Package server exposes the fleet over HTTP: the embedded dashboard, a JSON
// snapshot, and a server-sent event stream that pushes every new snapshot.
//
// Authentication is deliberately absent (see the spec): reachability is the auth
// story, and internal/config decides reachability by binding loopback unless lan is
// set. Do not add a token gate here without changing that decision first.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"regexp"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/fleet"
	"github.com/waldemarsson/sluss/internal/kits"
	"github.com/waldemarsson/sluss/internal/proxy"
	"github.com/waldemarsson/sluss/internal/sbx"
	"github.com/waldemarsson/sluss/internal/script"
)

// heartbeat keeps an idle SSE connection from being closed by an intermediary.
const heartbeat = 15 * time.Second

// lifecycleTimeout bounds a script run. It is generous because "sbx create" can
// pull an image, and the alternative — cancelling halfway — is what leaves an
// orphan worktree behind.
const lifecycleTimeout = 30 * time.Minute

// Server wires the fleet to HTTP. Assets are injected rather than embedded here so
// tests need no dashboard build.
type Server struct {
	cfg    *config.Config
	poller *fleet.Poller
	runner *script.Runner
	client *sbx.Client
	assets fs.FS
}

// New builds the server. The runner drives lifecycle through scripts/sluss; the sbx
// client is used directly only for secrets.
func New(cfg *config.Config, poller *fleet.Poller, runner *script.Runner, client *sbx.Client, assets fs.FS) *Server {
	return &Server{cfg: cfg, poller: poller, runner: runner, client: client, assets: assets}
}

// Handler returns the complete route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/fleet", s.handleFleet)
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("POST /api/sandboxes", guard(s.handleCreate))
	mux.HandleFunc("POST /api/sandboxes/{scope}/{name}/stop", guard(s.handleStop))
	mux.HandleFunc("DELETE /api/sandboxes/{scope}/{name}", guard(s.handleDestroy))
	mux.HandleFunc("GET /api/secrets/{scope}", s.handleSecretList)
	mux.HandleFunc("PUT /api/secrets/{scope}/{name}", guard(s.handleSecretSet))
	mux.HandleFunc("DELETE /api/secrets/{scope}/{name}", guard(s.handleSecretDelete))
	mux.HandleFunc("GET /api/kits", s.handleKitList)
	mux.HandleFunc("GET /api/kits/{name}/spec", s.handleKitRead)
	mux.HandleFunc("PUT /api/kits/{name}/spec", guard(s.handleKitWrite))
	// Anything else under /api is a mistake in the dashboard, not a file request.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no such endpoint: %s", r.URL.Path))
	})
	// Registered without a method: a method-qualified "GET /" would conflict with
	// the "/api/" pattern above, which matches more paths but fewer methods.
	mux.Handle("/", http.FileServerFS(s.assets))

	router := proxy.New(s.cfg, s.poller)
	// Path mode is a route like any other. Host mode cannot be, because the
	// hostname — not the path — says which sandbox is meant, so it is checked
	// before the mux ever sees the request.
	mux.Handle(proxy.PathPrefix, router.Handler())
	if s.cfg.Access == config.AccessHost {
		return hostDispatch(router, mux)
	}
	return mux
}

// hostDispatch sends <prefix><name>.<domain> to the proxy and everything else —
// the dashboard's own hostname included — to the mux.
func hostDispatch(router *proxy.Router, mux http.Handler) http.Handler {
	sandboxes := router.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, _, ok := router.Match(r.Host, r.URL.Path); ok {
			sandboxes.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// guard protects the mutating routes from a page the user happens to be visiting.
//
// There is no authentication by decision, on the premise that reachability is the
// boundary — but a browser makes 127.0.0.1 reachable from any site on the web, so
// that premise needs two cheap defences:
//
//   - Sec-Fetch-Site, which every current browser sends and no page can forge,
//     rejects anything initiated cross-site.
//   - Requiring JSON on POST rejects the one shape a cross-origin request can take
//     without a preflight: a "simple request" is limited to text/plain, form or
//     multipart content types. PUT and DELETE are always preflighted.
//
// Neither is authentication, and neither pretends to be: they close the drive-by
// path, not the "someone hostile is on your LAN" one the spec already accepts.
func guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Sec-Fetch-Site") {
		case "", "same-origin", "none":
			// Not a browser, or the dashboard itself.
		default:
			writeError(w, http.StatusForbidden, "cross-site requests are refused")
			return
		}
		if r.Method == http.MethodPost && !isJSON(r.Header.Get("Content-Type")) {
			writeError(w, http.StatusUnsupportedMediaType, "this endpoint requires Content-Type: application/json")
			return
		}
		next(w, r)
	}
}

func isJSON(header string) bool {
	mediaType, _, err := mime.ParseMediaType(header)
	return err == nil && mediaType == "application/json"
}

// lifecycleContext detaches a script run from the request that started it, so
// closing the browser tab cannot SIGKILL "sluss start" between creating the
// worktree and creating the sandbox — the script's unwind only runs when sbx
// itself fails, never when the script is killed.
func lifecycleContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), lifecycleTimeout)
}

// handleConfig tells the dashboard how to form sandbox URLs and what it may create
// them from. Nothing secret lives in the configuration file, but this still exposes
// only the fields the UI actually needs.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Access     string   `json:"access"`
		HostPrefix string   `json:"hostPrefix"`
		Domain     string   `json:"domain"`
		Repos      []string `json:"repos"`
		Scopes     []string `json:"scopes"`
	}{
		Access:     s.cfg.Access,
		HostPrefix: s.cfg.HostPrefix,
		Domain:     s.cfg.Domain,
		Repos:      s.cfg.Repos,
		Scopes:     s.cfg.AppNames,
	})
}

func (s *Server) handleFleet(w http.ResponseWriter, r *http.Request) {
	snap := s.poller.Current()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "the fleet has not been polled yet")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleEvents streams snapshots. Every write is flushed immediately: the same
// no-buffering rule the reverse proxy lives by (docs/DECISIONS.md D5).
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	updates, unsubscribe := s.poller.Subscribe()
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Tells any front proxy that understands it not to buffer this response.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// ResponseController is the Go 1.20+ way to flush without asserting on the
	// concrete ResponseWriter type.
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	if snap := s.poller.Current(); snap != nil {
		if !writeEvent(w, rc, snap) {
			return
		}
	}

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case snap, open := <-updates:
			if !open {
				return
			}
			if !writeEvent(w, rc, snap) {
				return
			}
		case <-ticker.C:
			// A comment line: valid SSE, ignored by the browser, keeps the
			// connection warm.
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}

// writeEvent sends one snapshot and reports whether the connection is still usable.
func writeEvent(w http.ResponseWriter, rc *http.ResponseController, snap *fleet.Snapshot) bool {
	body, err := json.Marshal(snap)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: fleet\ndata: %s\n\n", body); err != nil {
		return false
	}
	return rc.Flush() == nil
}

// handleCreate starts a sandbox by running scripts/sluss in the chosen repository.
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Repo  string   `json:"repo"`
		Name  string   `json:"name"`
		Agent string   `json:"agent"`
		Scope string   `json:"scope"`
		Extra []string `json:"extra"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "malformed request body")
		return
	}
	// Only configured repositories and scopes are reachable: the browser must not
	// be able to point the script at an arbitrary directory.
	if !s.cfg.HasRepo(body.Repo) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a configured repository", body.Repo))
		return
	}
	if !s.cfg.HasScope(body.Scope) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a configured scope", body.Scope))
		return
	}

	extra, err := allowedExtras(body.Extra)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := lifecycleContext(r)
	defer cancel()
	result, err := s.runner.Start(ctx, script.StartOpts{
		Repo: body.Repo, Name: body.Name, Agent: body.Agent, AppName: body.Scope, Extra: extra,
	})
	writeResult(w, result, err)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	sandbox, ok := s.sandboxFor(w, r)
	if !ok {
		return
	}
	ctx, cancel := lifecycleContext(r)
	defer cancel()
	result, err := s.runner.Stop(ctx, sandbox.Repo, sandbox.Scope, sandbox.Name)
	writeResult(w, result, err)
}

// handleDestroy never forces. The script refuses dirty or unmerged work, and that
// refusal is what reaches the user (AC10).
func (s *Server) handleDestroy(w http.ResponseWriter, r *http.Request) {
	sandbox, ok := s.sandboxFor(w, r)
	if !ok {
		return
	}
	if sandbox.Repo == "" {
		writeError(w, http.StatusConflict, fmt.Sprintf(
			"cannot tell which repository %q belongs to; destroy it from a terminal", sandbox.Name))
		return
	}
	ctx, cancel := lifecycleContext(r)
	defer cancel()
	result, err := s.runner.Destroy(ctx, sandbox.Repo, sandbox.Scope, sandbox.Name)
	writeResult(w, result, err)
}

// sizeValue matches what --cpus and --memory accept: a number, optionally with a
// single-letter unit.
var sizeValue = regexp.MustCompile(`^[0-9]+[a-zA-Z]?$`)

// allowedExtras keeps the browser from passing arbitrary flags into "sbx create".
// Nothing here is a shell, so this is not about injection — it is that an
// unfiltered list could mount another host path as a workspace or publish another
// port. Only the sizing flags the create form offers get through.
func allowedExtras(extra []string) ([]string, error) {
	for i := 0; i < len(extra); i += 2 {
		flag := extra[i]
		if flag != "--cpus" && flag != "--memory" {
			return nil, fmt.Errorf("%q is not an option this endpoint accepts (only --cpus and --memory)", flag)
		}
		if i+1 >= len(extra) {
			return nil, fmt.Errorf("%s needs a value", flag)
		}
		if !sizeValue.MatchString(extra[i+1]) {
			return nil, fmt.Errorf("%s value %q is not a number", flag, extra[i+1])
		}
	}
	return extra, nil
}

// sandboxFor resolves the {scope}/{name} pair of a lifecycle route against the
// current snapshot, writing the error response itself when it cannot.
func (s *Server) sandboxFor(w http.ResponseWriter, r *http.Request) (fleet.Sandbox, bool) {
	scope, name := r.PathValue("scope"), r.PathValue("name")
	if !s.cfg.HasScope(scope) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a configured scope", scope))
		return fleet.Sandbox{}, false
	}
	snap := s.poller.Current()
	if snap == nil {
		writeError(w, http.StatusServiceUnavailable, "the fleet has not been polled yet")
		return fleet.Sandbox{}, false
	}
	sandbox, ok := snap.Find(scope, name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no sandbox %q in scope %q", name, scope))
		return fleet.Sandbox{}, false
	}
	return sandbox, true
}

// secretName is deliberately strict: the name reaches sbx as an argument, and an
// environment-variable-shaped name is all a secret ever needs.
var secretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// maxSecretValue caps what a single PUT may carry.
const maxSecretValue = 1 << 20

// handleSecretList returns names only. Values are write-only by design: sbx and the
// sandbox are the only things that ever see them (docs/DECISIONS.md D1).
func (s *Server) handleSecretList(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.scopeOf(w, r)
	if !ok {
		return
	}
	names, err := s.client.SecretNames(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string][]string{"names": names})
}

// handleSecretSet stores a value. The body is the value, and it is never echoed,
// never logged, and never returned by any endpoint.
func (s *Server) handleSecretSet(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.scopeOf(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !secretName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "secret names may contain letters, digits and underscores only")
		return
	}
	value, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSecretValue))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "secret value is too large")
		return
	}
	if len(value) == 0 {
		writeError(w, http.StatusBadRequest, "secret value is empty")
		return
	}
	if err := s.client.SecretSet(r.Context(), scope, name, string(value)); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSecretDelete(w http.ResponseWriter, r *http.Request) {
	scope, ok := s.scopeOf(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	if !secretName.MatchString(name) {
		writeError(w, http.StatusBadRequest, "secret names may contain letters, digits and underscores only")
		return
	}
	if err := s.client.SecretDelete(r.Context(), scope, name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxSpec caps a kit spec at a size no hand-written YAML approaches.
const maxSpec = 1 << 20

func (s *Server) handleKitList(w http.ResponseWriter, r *http.Request) {
	listed, err := kits.List(s.cfg.KitsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := kits.Status(r.Context(), s.cfg.KitsDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if listed == nil {
		listed = []kits.Kit{}
	}
	writeJSON(w, http.StatusOK, struct {
		Kits   []kits.Kit     `json:"kits"`
		Status kits.GitStatus `json:"status"`
	}{listed, status})
}

// handleKitRead serves the spec as bytes. It is not JSON and not parsed: sluss
// parses no YAML.
func (s *Server) handleKitRead(w http.ResponseWriter, r *http.Request) {
	body, err := kits.ReadSpec(s.cfg.KitsDir, r.PathValue("name"))
	if err != nil {
		writeError(w, kitStatus(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// handleKitWrite stores the body byte for byte, exactly as typed.
func (s *Server) handleKitWrite(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSpec))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "kit spec is too large")
		return
	}
	if err := kits.WriteSpec(s.cfg.KitsDir, r.PathValue("name"), body); err != nil {
		writeError(w, kitStatus(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func kitStatus(err error) int {
	if errors.Is(err, kits.ErrBadName) {
		return http.StatusBadRequest
	}
	return http.StatusNotFound
}

// scopeOf reads and validates the {scope} path segment.
func (s *Server) scopeOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	scope := r.PathValue("scope")
	if !s.cfg.HasScope(scope) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not a configured scope", scope))
		return "", false
	}
	return scope, true
}

// writeResult reports a script run. A refusal is the script's answer, so it keeps
// the script's own message and comes back as 409 rather than 500.
func writeResult(w http.ResponseWriter, result script.Result, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	code := http.StatusOK
	if result.Refused() {
		code = http.StatusConflict
	}
	writeJSON(w, code, result)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The status line is already sent; there is nothing useful left to say to
		// the client, and this is not worth killing the process over.
		return
	}
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]string{"error": message})
}
