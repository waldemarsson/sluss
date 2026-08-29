package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/waldemarsson/sluss/internal/release"
)

// archive builds the tar.gz a release publishes: one executable named sluss.
func archive(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: "sluss", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("writing tar header: %v", err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatalf("writing tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing gzip: %v", err)
	}
	return buf.Bytes()
}

// fakeRelease serves one release the way GitHub does: an API document naming the
// tag, an asset per platform, and a checksums.txt over them.
type fakeRelease struct {
	tag      string
	asset    []byte
	checksum string // overrides the real SHA-256 when set
	server   *httptest.Server
	apiCalls int
}

func serveRelease(t *testing.T, tag string, asset []byte) *fakeRelease {
	t.Helper()
	f := &fakeRelease{tag: tag, asset: asset}
	name := release.AssetName(runtime.GOOS, runtime.GOARCH)

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		f.apiCalls++
		fmt.Fprintf(w, `{"tag_name": %q}`, f.tag)
	})
	mux.HandleFunc("/releases/download/"+tag+"/"+name, func(w http.ResponseWriter, r *http.Request) {
		w.Write(f.asset)
	})
	mux.HandleFunc("/releases/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		sum := f.checksum
		if sum == "" {
			raw := sha256.Sum256(f.asset)
			sum = hex.EncodeToString(raw[:])
		}
		fmt.Fprintf(w, "%s  %s\n%s  sluss_other_arch.tar.gz\n", sum, name, strings.Repeat("0", 64))
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeRelease) client() *release.Client {
	return &release.Client{
		APIURL:      f.server.URL + "/api",
		DownloadURL: f.server.URL + "/releases",
		HTTP:        f.server.Client(),
	}
}

// installed writes a stand-in for the running binary and returns its path.
func installed(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sluss")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing the installed binary: %v", err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(body)
}

func TestLatest(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "new"))

	got, err := f.client().Latest(context.Background())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got != "v1.2.0" {
		t.Errorf("Latest = %q, want v1.2.0", got)
	}
}

func TestUpdateReplacesTheBinary(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "new sluss"))
	target := installed(t, "old sluss")
	var out bytes.Buffer

	if err := f.client().Update(context.Background(), "v1.1.0", "", target, &out); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := read(t, target); got != "new sluss" {
		t.Errorf("binary = %q, want the released one", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("mode = %v, want the new binary executable", info.Mode())
	}
	if !strings.Contains(out.String(), "v1.2.0") {
		t.Errorf("output = %q, want the version named", out.String())
	}
}

// The common case: nothing to do, and nothing downloaded to find that out.
func TestUpdateIsANoOpWhenAlreadyLatest(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "new sluss"))
	target := installed(t, "current sluss")
	var out bytes.Buffer

	if err := f.client().Update(context.Background(), "v1.2.0", "", target, &out); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := read(t, target); got != "current sluss" {
		t.Errorf("binary = %q, want it untouched", got)
	}
	if !strings.Contains(out.String(), "already the latest") {
		t.Errorf("output = %q", out.String())
	}
}

// A binary built by "task build" reports a git describe string, not a tag; the
// leading "v" must not make an identical version look stale.
func TestUpdateToleratesAMissingVPrefix(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "new sluss"))
	target := installed(t, "current sluss")
	var out bytes.Buffer

	if err := f.client().Update(context.Background(), "1.2.0", "", target, &out); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := read(t, target); got != "current sluss" {
		t.Error("an identically versioned binary was replaced anyway")
	}
}

// An explicit version installs that release without asking which one is newest.
func TestUpdateWithAnExplicitVersionSkipsTheAPI(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "pinned sluss"))
	target := installed(t, "old sluss")
	var out bytes.Buffer

	if err := f.client().Update(context.Background(), "v9.9.9", "v1.2.0", target, &out); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := read(t, target); got != "pinned sluss" {
		t.Errorf("binary = %q, want the pinned release", got)
	}
	if f.apiCalls != 0 {
		t.Errorf("api calls = %d, want none for a pinned version", f.apiCalls)
	}
}

func TestUpdateRefusesABadDownload(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*fakeRelease)
		wantErr string
	}{
		{
			name:    "checksum mismatch",
			mutate:  func(f *fakeRelease) { f.checksum = strings.Repeat("a", 64) },
			wantErr: "checksum mismatch",
		},
		{
			name:    "truncated archive",
			mutate:  func(f *fakeRelease) { f.asset = f.asset[:len(f.asset)/2] },
			wantErr: "archive",
		},
		{
			name:    "not an archive at all",
			mutate:  func(f *fakeRelease) { f.asset = []byte("<html>404</html>") },
			wantErr: "archive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := serveRelease(t, "v1.2.0", archive(t, "new sluss"))
			tt.mutate(f)
			// The checksum is computed over whatever the server now serves, so a
			// truncated archive still passes verification and fails at extraction —
			// which is the point: neither may replace the installed binary.
			target := installed(t, "old sluss")
			var out bytes.Buffer

			err := f.client().Update(context.Background(), "v1.1.0", "", target, &out)
			if err == nil {
				t.Fatal("Update succeeded on a bad download")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
			}
			if got := read(t, target); got != "old sluss" {
				t.Errorf("binary = %q, want the original left in place", got)
			}
		})
	}
}

func TestUpdateRefusesASymlink(t *testing.T) {
	f := serveRelease(t, "v1.2.0", archive(t, "new sluss"))
	real := installed(t, "checkout sluss")
	link := filepath.Join(t.TempDir(), "sluss")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}
	var out bytes.Buffer

	err := f.client().Update(context.Background(), "v1.1.0", "", link, &out)
	if err == nil {
		t.Fatal("Update replaced a symlink")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error = %q, want the symlink refusal", err)
	}
	if got := read(t, real); got != "checkout sluss" {
		t.Error("the checkout the symlink pointed at was replaced")
	}
}

func TestUpdateReportsRateLimiting(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit exceeded", http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &release.Client{APIURL: srv.URL + "/api", HTTP: srv.Client()}

	_, err := c.Latest(context.Background())
	if err == nil {
		t.Fatal("Latest succeeded against a rate-limited API")
	}
	if !strings.Contains(err.Error(), "rate limiting") {
		t.Errorf("error = %q, want a readable rate-limit message", err)
	}
}

func TestAssetNamesAreVersionFree(t *testing.T) {
	// Stable names are what let the install script use
	// "releases/latest/download/<name>" with no API call and no rate limit.
	if got := release.AssetName("darwin", "arm64"); got != "sluss_darwin_arm64.tar.gz" {
		t.Errorf("AssetName = %q", got)
	}
}
