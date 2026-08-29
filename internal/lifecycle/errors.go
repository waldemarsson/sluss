// Package lifecycle owns the sandbox and worktree lifecycle: creating a branch and
// worktree, handing them to sbx, and taking them away again safely.
//
// It replaces the shell script sluss used to be. The dashboard and the command line
// both call this package, so "created from the browser" and "created in a terminal"
// are the same code path by construction rather than by testing (docs/DECISIONS.md
// D23, superseding D14).
package lifecycle

import (
	"errors"
	"fmt"
)

// Kind says what a caller should do with an error. Two of the three are answers
// rather than faults: sluss declining to destroy dirty work is working correctly,
// and so is telling someone they typed the command wrong.
type Kind int

const (
	// Failure is something that actually went wrong — sbx unreachable, git broken.
	Failure Kind = iota
	// Refused is a deliberate no: dirty work, an existing branch, a missing worktree.
	// The command line exits 1, and the dashboard shows the message.
	Refused
	// Misuse is a wrong invocation. The command line exits 2.
	Misuse
)

// Error carries a Kind alongside its message. It exists so the exit code is decided
// where the reason is known, rather than by matching on message text later.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// refuse builds a deliberate refusal.
func refuse(format string, args ...any) error {
	return &Error{Kind: Refused, Msg: fmt.Sprintf(format, args...)}
}

// misuse builds a "you typed it wrong".
func misuse(format string, args ...any) error {
	return &Error{Kind: Misuse, Msg: fmt.Sprintf(format, args...)}
}

// KindOf reports how err should be treated. Anything that is not a *Error — an sbx
// or git failure, say — is a Failure.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return Failure
}

// ExitCode is the process exit status for err: 0 success, 1 refused or failed,
// 2 misused. It matches the shell script it replaces.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if KindOf(err) == Misuse {
		return 2
	}
	return 1
}
