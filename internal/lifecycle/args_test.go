package lifecycle

import (
	"slices"
	"testing"
)

func TestSplitStartArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantAgent string
		wantExtra []string
	}{
		{
			name: "no arguments at all",
		},
		{
			name:      "--agent is consumed and never forwarded",
			args:      []string{"--agent", "claude"},
			wantAgent: "claude",
		},
		{
			name:      "--agent= is consumed mid-list and forwarding order is preserved",
			args:      []string{"--cpus", "4", "--agent=copilot", "--memory", "8g"},
			wantAgent: "copilot",
			wantExtra: []string{"--cpus", "4", "--memory", "8g"},
		},
		{
			name:      "everything else forwards untouched",
			args:      []string{"--kit", "/kits/dotnet", "--publish", "5173"},
			wantExtra: []string{"--kit", "/kits/dotnet", "--publish", "5173"},
		},
		{
			// The point of "--": a sandbox may want its own --agent flag.
			name:      "-- ends scanning and forwards the rest verbatim",
			args:      []string{"--cpus", "4", "--", "--agent", "not-ours"},
			wantExtra: []string{"--cpus", "4", "--agent", "not-ours"},
		},
		{
			name:      "the last --agent wins",
			args:      []string{"--agent", "claude", "--agent=copilot"},
			wantAgent: "copilot",
		},
		{
			name:      "-- with nothing after it forwards nothing",
			args:      []string{"--agent", "claude", "--"},
			wantAgent: "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, extra, err := SplitStartArgs(tt.args)
			if err != nil {
				t.Fatalf("splitStartArgs: %v", err)
			}
			if agent != tt.wantAgent {
				t.Errorf("agent = %q, want %q", agent, tt.wantAgent)
			}
			if !slices.Equal(extra, tt.wantExtra) {
				t.Errorf("extra = %v, want %v", extra, tt.wantExtra)
			}
		})
	}
}

func TestSplitStartArgsRejectsABareAgentFlag(t *testing.T) {
	for _, args := range [][]string{
		{"--agent"},
		{"--cpus", "4", "--agent"},
		{"--agent="},
	} {
		_, _, err := SplitStartArgs(args)
		if err == nil {
			t.Fatalf("SplitStartArgs(%v) accepted a valueless --agent", args)
		}
		if KindOf(err) != Misuse {
			t.Errorf("SplitStartArgs(%v) kind = %v, want Misuse", args, KindOf(err))
		}
		if err.Error() != "--agent requires an agent name" {
			t.Errorf("message = %q", err)
		}
	}
}

func TestPublishesOpenCodePort(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"nothing published", nil, false},
		{"an unrelated port", []string{"--publish", "5173"}, false},
		{"the bare port", []string{"--publish", "4096"}, true},
		{"short flag", []string{"-p", "4096"}, true},
		{"host and sandbox halves", []string{"--publish", "14096:4096"}, true},
		{"with an address", []string{"--publish", "0.0.0.0:14096:4096"}, true},
		{"with a protocol", []string{"--publish", "4096/tcp"}, true},
		{"with an IPv6 protocol", []string{"--publish", "14096:4096/tcp6"}, true},
		{"inline long form", []string{"--publish=4096"}, true},
		{"inline short form", []string{"-p=14096:4096"}, true},
		{"a host port that merely ends in 4096", []string{"--publish", "14096"}, false},
		{"a sandbox port that merely ends in 4096", []string{"--publish", "8080:14096"}, false},
		{"4096 as some other flag's value", []string{"--memory", "4096"}, false},
		{"4096 as a bare argument", []string{"4096"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publishesOpenCodePort(tt.args); got != tt.want {
				t.Errorf("publishesOpenCodePort(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestCheckName(t *testing.T) {
	valid := []string{"web", "auth", "gh-remote", "v1.2", "a1", "A-B.c"}
	invalid := []string{"", "a", "-web", ".web", "web/auth", "web auth", "web_auth", "wéb"}

	for _, name := range valid {
		if err := checkName(name); err != nil {
			t.Errorf("checkName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range invalid {
		err := checkName(name)
		if err == nil {
			t.Errorf("checkName(%q) accepted it", name)
			continue
		}
		if KindOf(err) != Misuse {
			t.Errorf("checkName(%q) kind = %v, want Misuse", name, KindOf(err))
		}
	}
}

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(refuse("dirty")); got != 1 {
		t.Errorf("ExitCode(refusal) = %d, want 1", got)
	}
	if got := ExitCode(misuse("wrong")); got != 2 {
		t.Errorf("ExitCode(misuse) = %d, want 2", got)
	}
	if got := ExitCode(errFailure()); got != 1 {
		t.Errorf("ExitCode(failure) = %d, want 1", got)
	}
}

func errFailure() error { return &Error{Kind: Failure, Msg: "sbx exploded"} }
