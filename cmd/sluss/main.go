// Command sluss is a control plane over Docker Sandboxes (sbx).
// See docs/SPEC.md for what it will do and docs/ROADMAP.md for the order.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
//
// A package-level var (not a const) is what makes -X able to write to it.
var version = "dev"

func main() {
	// os.Args[0] is the program name, so the arguments start at index 1.
	args := os.Args[1:]

	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println(version)
		return
	}

	// Nothing else is implemented yet: docs/SPIKE.md is unanswered, and every
	// command below depends on an assumption it verifies. Exit non-zero so a
	// script never mistakes this stub for a working binary.
	fmt.Fprintln(os.Stderr, "sluss "+version+": not implemented yet — see docs/ROADMAP.md (M0 spike is unanswered)")
	os.Exit(1)
}
