// Package web embeds the built dashboard into the binary.
//
// The embed lives here rather than in internal/server because go:embed cannot
// reach outside its own package directory. web/dashboard/build/.gitkeep is
// committed so this compiles from a fresh clone with no dashboard build.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dashboard/build
var files embed.FS

// Assets is the built dashboard, rooted at the build directory.
var Assets fs.FS = mustSub()

func mustSub() fs.FS {
	sub, err := fs.Sub(files, "dashboard/build")
	if err != nil {
		// Only reachable if the embed directive above stops matching, which is a
		// build-time mistake, not a runtime condition.
		panic("embedding dashboard: " + err.Error())
	}
	return sub
}
