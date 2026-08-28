// Package proxy reaches sandboxes through sluss's single listener.
//
// Two addressing schemes over one port, chosen by config.Access:
//
//	path  /s/<scope>/<name>/…            no DNS, no wildcard certificate, no front proxy
//	host  <prefix><name>.<domain>/…      serves root; needs wildcard DNS and a certificate
//
// Routing comes from the current fleet snapshot, so a sandbox that disappears stops
// resolving within one poll instead of hanging against a dead backend.
//
// FlushInterval: -1 is load-bearing (docs/DECISIONS.md D5): it flushes every write
// instead of buffering, which is the difference between a usable web UI and one that
// feels broken. Do not remove it, do not add buffering middleware, and do not set
// Content-Length on a streamed response.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/fleet"
)

// PathPrefix is where path-mode routes live.
const PathPrefix = "/s/"

// Router forwards requests to sandboxes.
type Router struct {
	cfg     *config.Config
	poller  *fleet.Poller
	reverse *httputil.ReverseProxy
}

// contextKey is unexported so nothing outside this package can collide with it.
type contextKey struct{}

// New builds the router. One ReverseProxy serves every sandbox: the target is
// carried on each request's context rather than baked into the proxy.
func New(cfg *config.Config, poller *fleet.Poller) *Router {
	r := &Router{cfg: cfg, poller: poller}
	r.reverse = &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			target, _ := pr.In.Context().Value(contextKey{}).(*url.URL)
			if target != nil {
				pr.SetURL(target)
			}
			pr.SetXForwarded()
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			// A sandbox that died between the snapshot and this request must fail
			// fast and legibly rather than leave the browser waiting.
			http.Error(w, fmt.Sprintf("sandbox unreachable: %v", err), http.StatusBadGateway)
		},
	}
	return r
}

// Match extracts the sandbox a request addresses. In host mode the URL carries no
// scope, so scope is empty and the snapshot decides — see resolve.
func (r *Router) Match(host, path string) (scope, name, rest string, ok bool) {
	if r.cfg.Access == config.AccessHost {
		name, ok = r.matchHost(host)
		if !ok {
			return "", "", "", false
		}
		if path == "" {
			path = "/"
		}
		return "", name, path, true
	}

	if !strings.HasPrefix(path, PathPrefix) {
		return "", "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(path, PathPrefix), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	rest = "/"
	if len(parts) == 3 {
		rest += parts[2]
	}
	return parts[0], parts[1], rest, true
}

// matchHost pulls the sandbox name out of <prefix><name>.<domain>.
func (r *Router) matchHost(host string) (string, bool) {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	suffix := "." + r.cfg.Domain
	if r.cfg.Domain == "" || !strings.HasSuffix(host, suffix) {
		return "", false
	}
	label := strings.TrimSuffix(host, suffix)
	if strings.Contains(label, ".") || !strings.HasPrefix(label, r.cfg.HostPrefix) {
		// A wildcard certificate and a wildcard DNS record each match exactly one
		// label, so a second label is covered by neither and is not ours.
		return "", false
	}
	name := strings.TrimPrefix(label, r.cfg.HostPrefix)
	if name == "" {
		return "", false
	}
	return name, true
}

// Handler serves proxied requests. Anything it cannot route gets a 404 saying why,
// never a hang.
func (r *Router) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		scope, name, rest, ok := r.Match(req.Host, req.URL.Path)
		if !ok {
			http.Error(w, "not a sandbox address", http.StatusNotFound)
			return
		}

		// In path mode the browser needs a trailing slash before the sandbox's own
		// relative URLs resolve under the prefix.
		if r.cfg.Access == config.AccessPath && !strings.HasSuffix(req.URL.Path, "/") && rest == "/" {
			// Keep the query: an app that carries state in its first URL would
			// otherwise lose it on the redirect.
			slashed := *req.URL
			slashed.Path += "/"
			http.Redirect(w, req, slashed.RequestURI(), http.StatusPermanentRedirect)
			return
		}

		sandbox, err := r.resolve(scope, name)
		if err != nil {
			http.Error(w, err.Error(), statusFor(err))
			return
		}

		target := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", sandbox.WebPort)}
		out := req.Clone(contextWithTarget(req, target))
		out.URL.Path = rest
		r.reverse.ServeHTTP(w, out)
	})
}

// routeError carries the status a failure should produce.
type routeError struct {
	status  int
	message string
}

func (e *routeError) Error() string { return e.message }

func statusFor(err error) int {
	// errors.As rather than a type assertion, so a wrapped routeError still carries
	// its status.
	var re *routeError
	if errors.As(err, &re) {
		return re.status
	}
	return http.StatusInternalServerError
}

// contextWithTarget carries the resolved backend to the proxy's Rewrite hook,
// which is shared by every sandbox and so cannot close over one target.
func contextWithTarget(req *http.Request, target *url.URL) context.Context {
	return context.WithValue(req.Context(), contextKey{}, target)
}

// resolve finds the sandbox behind an address in the current snapshot.
func (r *Router) resolve(scope, name string) (fleet.Sandbox, error) {
	snap := r.poller.Current()
	if snap == nil {
		return fleet.Sandbox{}, &routeError{http.StatusServiceUnavailable, "the fleet has not been polled yet"}
	}

	var matches []fleet.Sandbox
	if scope != "" {
		if sandbox, ok := snap.Find(scope, name); ok {
			matches = append(matches, sandbox)
		}
	} else {
		// Host mode drops the scope from the address, so the name must be unique
		// across scopes for the request to be answerable at all.
		for _, group := range snap.Repos {
			for _, sandbox := range group.Sandboxes {
				if sandbox.Name == name {
					matches = append(matches, sandbox)
				}
			}
		}
	}

	switch {
	case len(matches) == 0:
		return fleet.Sandbox{}, &routeError{http.StatusNotFound, fmt.Sprintf("no sandbox %q in the current fleet", name)}
	case len(matches) > 1:
		scopes := make([]string, 0, len(matches))
		for _, m := range matches {
			scopes = append(scopes, m.Scope)
		}
		return fleet.Sandbox{}, &routeError{http.StatusConflict, fmt.Sprintf(
			"sandbox %q exists in scopes %s; host mode cannot tell them apart — use path mode",
			name, strings.Join(scopes, ", "))}
	}

	sandbox := matches[0]
	if sandbox.WebPort == 0 {
		return fleet.Sandbox{}, &routeError{http.StatusNotFound, fmt.Sprintf(
			"sandbox %q is %s and publishes no web port", name, sandbox.Status)}
	}
	return sandbox, nil
}
