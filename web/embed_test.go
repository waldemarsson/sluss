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
