package lifecycle

import (
	"regexp"
	"strings"
)

// OpenCode's web UI listens on sandbox port 4096, and sluss publishes it for you
// unless you published it yourself.
const openCodePort = "4096"

// These mirror the two shapes the retired script matched, character for character.
// The separate-argument form is anchored only at the end, so "14096:4096" matches
// on its ":4096" — that call really does publish sandbox port 4096 — while a bare
// "14096" does not.
var (
	publishValue  = regexp.MustCompile(`(^|:)` + openCodePort + `(/tcp[46]?)?$`)
	publishInline = regexp.MustCompile(`^(--publish|-p)=(.*:)?` + openCodePort + `(/tcp[46]?)?$`)
)

// A name becomes a branch, a directory and a sandbox, so it is deliberately narrow:
// at least two characters, starting with a letter or digit.
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]+$`)

// SplitStartArgs pulls --agent out of a start invocation and leaves everything else
// alone. Order is preserved: what sbx receives is what the caller wrote, minus
// --agent, because sbx's tolerance for reordering is unverified.
//
// "--" ends sluss's own scanning; everything after it is forwarded untouched, so a
// sandbox can be given its own --agent flag if it wants one.
//
// It is exported because the command line parses argv and the dashboard does not:
// the browser sends an agent and its extras as separate fields already.
func SplitStartArgs(args []string) (agent string, extra []string, err error) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--":
			extra = append(extra, args[i+1:]...)
			return agent, extra, nil
		case args[i] == "--agent":
			if i+1 >= len(args) {
				return "", nil, misuse("--agent requires an agent name")
			}
			agent = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--agent="):
			agent = strings.TrimPrefix(args[i], "--agent=")
			if agent == "" {
				return "", nil, misuse("--agent requires an agent name")
			}
		default:
			extra = append(extra, args[i])
		}
	}
	return agent, extra, nil
}

// publishesOpenCodePort reports whether the caller already published sandbox port
// 4096, in which case sluss must not publish it a second time.
func publishesOpenCodePort(args []string) bool {
	previous := ""
	for _, arg := range args {
		if (previous == "--publish" || previous == "-p") && publishValue.MatchString(arg) {
			return true
		}
		if publishInline.MatchString(arg) {
			return true
		}
		previous = arg
	}
	return false
}

// checkName rejects a name that cannot safely become a branch and a directory.
func checkName(name string) error {
	if !validName.MatchString(name) {
		return misuse("NAME must be at least two characters and contain only letters, numbers, dots, and hyphens")
	}
	return nil
}
