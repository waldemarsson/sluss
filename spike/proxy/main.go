// Command spike-proxy is throwaway. It exists to answer one question from
// docs/SPIKE.md assumption 3: does OpenCode's SSE token stream still feel like
// the TUI after passing through a Go reverse proxy?
//
// Delete this directory once M0 has a verdict.
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// Flags rather than a hardcoded 14096: assumption 4 may find that sbx
	// assigns the host port instead of letting us choose it.
	target := flag.String("target", "http://127.0.0.1:14096", "sandbox's published OpenCode address")
	listen := flag.String("listen", "127.0.0.1:8420", "address to serve on")
	flag.Parse()

	u, err := url.Parse(*target)
	if err != nil {
		log.Fatalf("parsing -target %q: %v", *target, err)
	}

	p := &httputil.ReverseProxy{
		// Rewrite replaced Director in Go 1.20; it also sets the X-Forwarded-*
		// headers for us. SetURL points the outbound request at the sandbox.
		Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(u) },

		// The whole experiment. -1 means "flush after every write" instead of
		// buffering; without it SSE tokens arrive in clumps. See DECISIONS.md D5.
		FlushInterval: -1,
	}

	log.Printf("spike-proxy: http://%s -> %s", *listen, u)
	log.Fatal(http.ListenAndServe(*listen, p))
}
