package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/waldemarsson/sluss/internal/release"
)

// These are variables so tests can point update at a local fake release and a
// throwaway target instead of GitHub and the running binary. Nothing else
// reassigns them; an empty updateTarget means "the executable that is running".
var (
	updateClient = &release.Client{}
	updateTarget = ""
)

func update(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprint(stderr, updateHelp)
		return 2
	}
	// An explicit tag installs that release without asking which one is newest;
	// without it, update resolves the latest and stops early if it is already here.
	if err := updateClient.Update(ctx, version, os.Getenv("SLUSS_VERSION"), updateTarget, stdout); err != nil {
		return fail(stderr, "%v", err)
	}
	return 0
}
