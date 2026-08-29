package web_test

import (
	"io/fs"
	"testing"

	"github.com/waldemarsson/sluss/web"
)

// The embed must compile and open from a fresh clone, where build/ holds only
// .gitkeep — CI builds no dashboard.
func TestAssetsOpen(t *testing.T) {
	entries, err := fs.ReadDir(web.Assets, ".")
	if err != nil {
		t.Fatalf("reading embedded assets: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded asset tree is empty; .gitkeep should always be there")
	}
}

// The dashboard's nested routes must prerender to a directory index rather than a
// sibling .html file: sluss serves the build with a plain http.FileServerFS, which
// resolves the first and not the second. This is what proves `trailingSlash: 'always'`
// in web/dashboard/src/routes/+layout.ts is still in effect.
func TestBuiltDashboardNestsConfigRoutes(t *testing.T) {
	if _, err := fs.Stat(web.Assets, "index.html"); err != nil {
		t.Skip("no dashboard build in this tree (run: task web)")
	}
	for _, page := range []string{"config/index.html", "config/secrets/index.html", "config/kits/index.html"} {
		if _, err := fs.Stat(web.Assets, page); err != nil {
			t.Errorf("%s missing from the build: %v", page, err)
		}
	}
}

// When a dashboard has been built, its entry point must be servable.
func TestBuiltDashboardHasAnIndex(t *testing.T) {
	body, err := fs.ReadFile(web.Assets, "index.html")
	if err != nil {
		t.Skip("no dashboard build in this tree (run: task web)")
	}
	if len(body) == 0 {
		t.Error("index.html is empty")
	}
}
