// Package release installs sluss over itself from a GitHub release.
//
// It is the same job scripts/install.sh does for a first install, done in Go so
// "sluss update" needs nothing but the binary already on disk: resolve the latest
// release, download the archive for this platform, check its SHA-256 against the
// release's checksums.txt, and swap the running executable for it atomically.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Defaults point at the project's own repository. They are fields on Client rather
// than constants so tests can serve a fake release from httptest.
const (
	DefaultAPIURL      = "https://api.github.com/repos/waldemarsson/sluss/releases/latest"
	DefaultDownloadURL = "https://github.com/waldemarsson/sluss/releases"
)

// maxDownload caps what will be read from the network. A release archive is a few
// megabytes; anything past this is a redirect to something that is not one.
const maxDownload = 64 << 20

// Client resolves and installs releases.
type Client struct {
	APIURL      string       // where the latest release is described
	DownloadURL string       // the releases base, under which assets live
	HTTP        *http.Client // nil means a client with a sane timeout
}

func (c *Client) apiURL() string {
	if c.APIURL != "" {
		return c.APIURL
	}
	return DefaultAPIURL
}

func (c *Client) downloadURL() string {
	if c.DownloadURL != "" {
		return c.DownloadURL
	}
	return DefaultDownloadURL
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

// AssetName is the archive for one platform. The name carries no version, so
// "<releases>/latest/download/<name>" resolves without asking the API first —
// which is what lets scripts/install.sh work with no API call and no rate limit.
func AssetName(goos, goarch string) string {
	return fmt.Sprintf("sluss_%s_%s.tar.gz", goos, goarch)
}

// Latest reports the tag of the newest release.
func (c *Client) Latest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("asking GitHub for the latest release: %w", err)
	}
	defer resp.Body.Close()

	// Unauthenticated API calls are rate limited per IP, and a person who hits that
	// deserves the way out rather than a status code.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return "", fmt.Errorf("GitHub is rate limiting this machine; try again later, or reinstall with the install script")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("asking GitHub for the latest release: %s", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("GitHub named no release tag")
	}
	return payload.TagName, nil
}

// Update replaces the executable at execPath with the release named by version, or
// with the latest release when version is empty.
//
// It reports what it did on out. An already-current binary is not an error: it is
// the normal answer, and downloads nothing.
func (c *Client) Update(ctx context.Context, current, version, execPath string, out io.Writer) error {
	if version == "" {
		latest, err := c.Latest(ctx)
		if err != nil {
			return err
		}
		version = latest
		if sameVersion(current, latest) {
			fmt.Fprintf(out, "sluss %s is already the latest release\n", current)
			return nil
		}
	}

	target, err := resolveTarget(execPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Downloading sluss %s for %s/%s...\n", version, runtime.GOOS, runtime.GOARCH)
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	archive, err := c.download(ctx, version, asset)
	if err != nil {
		return err
	}
	sums, err := c.download(ctx, version, "checksums.txt")
	if err != nil {
		return err
	}
	if err := verify(archive, string(sums), asset); err != nil {
		return err
	}

	binary, err := extract(archive)
	if err != nil {
		return err
	}
	if err := replace(target, binary); err != nil {
		return err
	}
	fmt.Fprintf(out, "Updated %s to %s\n", target, version)
	return nil
}

// sameVersion compares a built version string with a release tag, tolerating the
// leading "v" one carries and the other may not.
func sameVersion(current, tag string) bool {
	return strings.TrimPrefix(current, "v") == strings.TrimPrefix(tag, "v")
}

// resolveTarget decides what may be overwritten.
//
// A symlinked sluss normally points into a checkout, which git should update, and
// replacing it would silently detach the link from what it pointed at. An
// unwritable directory is named rather than discovered halfway through.
func resolveTarget(execPath string) (string, error) {
	if execPath == "" {
		found, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("finding the running sluss: %w", err)
		}
		execPath = found
	}
	info, err := os.Lstat(execPath)
	if err != nil {
		return "", fmt.Errorf("looking at %s: %w", execPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a symlink; update the checkout it points at instead", execPath)
	}
	dir := filepath.Dir(execPath)
	probe, err := os.CreateTemp(dir, ".sluss-update-*")
	if err != nil {
		return "", fmt.Errorf("%s is not writable, so sluss cannot replace itself there: %w", dir, err)
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return execPath, nil
}

func (c *Client) download(ctx context.Context, version, name string) ([]byte, error) {
	url := fmt.Sprintf("%s/download/%s/%s", strings.TrimSuffix(c.downloadURL(), "/"), version, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload))
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", name, err)
	}
	return body, nil
}

// verify checks the archive against the release's checksums.txt, whose lines are
// "<sha256>  <filename>" — the format sha256sum writes and shasum reads.
func verify(archive []byte, sums, asset string) error {
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		if fields[0] != got {
			return fmt.Errorf("checksum mismatch for %s: the download is not the released archive, so nothing was replaced", asset)
		}
		return nil
	}
	return fmt.Errorf("checksums.txt does not list %s", asset)
}

// extract pulls the sluss binary out of the downloaded archive.
func extract(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("reading the downloaded archive: %w", err)
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the downloaded archive: %w", err)
		}
		// The entry must be exactly "sluss" at the archive root, and a regular file.
		// Matching on the base name would also accept "decoy/sluss", letting a
		// hostile archive hide the entry that gets installed behind a nested one;
		// TypeReg rejects a symlink, which scripts/install.sh checks with -L.
		if filepath.Clean(header.Name) != "sluss" || header.Typeflag != tar.TypeReg {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(reader, maxDownload))
		if err != nil {
			return nil, fmt.Errorf("reading sluss out of the archive: %w", err)
		}
		return body, nil
	}
	return nil, fmt.Errorf("the downloaded archive contains no sluss binary")
}

// replace swaps the binary at path for body.
//
// The new binary is written beside the old one and renamed over it, because rename
// within one directory is atomic: an interrupted update leaves either the old sluss
// or the new one, never half of either. The running process keeps executing the
// image it started with, so replacing it under itself is safe.
func replace(path string, body []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".sluss-new-*")
	if err != nil {
		return fmt.Errorf("writing the new sluss into %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename has succeeded

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing the new sluss: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing the new sluss: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("making the new sluss executable: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}
