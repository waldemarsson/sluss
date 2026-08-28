// Package fleet turns sbx and git into one snapshot of every sandbox sluss knows
// about.
//
// It is the only source of truth in the process: the JSON API, the SSE stream, the
// reverse proxy's routing map and doctor all read the same snapshot. Nothing is
// persisted, so a sandbox removed by hand disappears within one tick and there is
// nothing to reconcile.
package fleet

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/waldemarsson/sluss/internal/config"
	"github.com/waldemarsson/sluss/internal/gitfacts"
	"github.com/waldemarsson/sluss/internal/sbx"
)

// OpenCodePort is the sandbox port scripts/sluss publishes for OpenCode Web.
const OpenCodePort = 4096

// Sandbox is one row of the fleet table. Identity is the pair (Scope, Name): sbx
// names are only unique within an app-name, so nothing is ever keyed on Name alone.
type Sandbox struct {
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	ID       string `json:"id"`
	Agent    string `json:"agent"`
	Status   string `json:"status"`
	Repo     string `json:"repo"`     // primary checkout path, "" when unknown
	RepoName string `json:"repoName"` // its base name, what the UI groups under
	Worktree string `json:"worktree"`
	Branch   string `json:"branch"`
	Dirty    bool   `json:"dirty"`
	Unmerged int    `json:"unmerged"`
	Missing  bool   `json:"missing"` // the worktree is gone from disk
	WebPort  int    `json:"webPort"` // host port for OpenCode Web, 0 when unpublished
}

// RepoGroup is the "Projects" concept: sandboxes grouped by the repository they
// were created from.
type RepoGroup struct {
	Repo      string    `json:"repo"`
	Name      string    `json:"name"`
	Sandboxes []Sandbox `json:"sandboxes"`
}

// ScopeError reports a scope (or one sandbox in it) that could not be read. One bad
// scope must not empty the whole dashboard.
type ScopeError struct {
	Scope string `json:"scope"`
	Error string `json:"error"`
}

// Snapshot is immutable once published. Consumers keep the pointer; the poller
// replaces it wholesale.
type Snapshot struct {
	At          time.Time    `json:"at"`
	Repos       []RepoGroup  `json:"repos"`
	ScopeErrors []ScopeError `json:"scopeErrors"`
}

// Find returns one sandbox by its scope-qualified identity.
func (s *Snapshot) Find(scope, name string) (Sandbox, bool) {
	if s == nil {
		return Sandbox{}, false
	}
	for _, g := range s.Repos {
		for _, sb := range g.Sandboxes {
			if sb.Scope == scope && sb.Name == name {
				return sb, true
			}
		}
	}
	return Sandbox{}, false
}

// Poller polls every configured scope on an interval and publishes snapshots.
type Poller struct {
	cfg      *config.Config
	client   *sbx.Client
	interval time.Duration

	// atomic.Pointer gives lock-free reads to the many consumers of the current
	// snapshot; only the poller writes, and it writes a whole new value each time.
	current atomic.Pointer[Snapshot]

	mu   sync.Mutex
	subs map[chan *Snapshot]struct{}
}

// New builds a poller. It does not start; call Run.
func New(cfg *config.Config, client *sbx.Client, interval time.Duration) *Poller {
	return &Poller{
		cfg:      cfg,
		client:   client,
		interval: interval,
		subs:     make(map[chan *Snapshot]struct{}),
	}
}

// Run polls until the context is cancelled, publishing each snapshot. It returns
// the context's error, which is how a caller distinguishes shutdown from a crash.
func (p *Poller) Run(ctx context.Context) error {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		p.publish(p.Poll(ctx))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// A ticker buffers exactly one missed tick. Draining it means a poll
			// that outlives its interval skips the tick it missed instead of
			// stacking a second poll immediately behind the first.
			select {
			case <-ticker.C:
			default:
			}
		}
	}
}

// Current returns the most recent snapshot, or nil before the first poll finishes.
func (p *Poller) Current() *Snapshot {
	return p.current.Load()
}

// Subscribe returns a channel of snapshots and a function that stops delivery. The
// channel holds one snapshot: a subscriber that falls behind misses intermediate
// updates rather than stalling the poll loop.
func (p *Poller) Subscribe() (<-chan *Snapshot, func()) {
	ch := make(chan *Snapshot, 1)

	p.mu.Lock()
	p.subs[ch] = struct{}{}
	p.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			p.mu.Lock()
			delete(p.subs, ch)
			p.mu.Unlock()
			// Safe to close only after removal: publish holds the same mutex, so
			// no send can be in flight here.
			close(ch)
		})
	}
	return ch, cancel
}

func (p *Poller) publish(s *Snapshot) {
	p.current.Store(s)

	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.subs {
		select {
		case ch <- s:
		default: // subscriber still holds an older snapshot; it will get the next one
		}
	}
}

// gitWorkers caps how many worktrees are inspected at once. Each one costs three
// git processes, so a large fleet would otherwise fork a crowd every tick.
const gitWorkers = 8

// Poll builds one snapshot. Exported so tests (and doctor) can take a single
// reading without running the loop.
func (p *Poller) Poll(ctx context.Context) *Snapshot {
	snap := &Snapshot{At: time.Now()}
	byRepo := make(map[string][]Sandbox)

	// Listing is per scope and cheap; the git facts behind each sandbox are not, so
	// they are gathered concurrently below.
	type listed struct {
		scope   string
		sandbox sbx.Sandbox
	}
	var pending []listed
	for _, scope := range p.cfg.AppNames {
		sandboxes, err := p.client.List(ctx, scope)
		if err != nil {
			snap.ScopeErrors = append(snap.ScopeErrors, ScopeError{Scope: scope, Error: err.Error()})
			continue
		}
		for _, s := range sandboxes {
			pending = append(pending, listed{scope: scope, sandbox: s})
		}
	}

	var (
		mu sync.Mutex // guards byRepo and snap.ScopeErrors below
		wg sync.WaitGroup
	)
	limit := make(chan struct{}, gitWorkers)
	for _, item := range pending {
		wg.Add(1)
		// item is passed in rather than captured, which keeps the goroutine's view
		// of it obvious.
		go func(item listed) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()

			sb, err := p.describe(ctx, item.scope, item.sandbox)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				snap.ScopeErrors = append(snap.ScopeErrors, ScopeError{
					Scope: item.scope,
					Error: fmt.Sprintf("%s: %v", item.sandbox.Name, err),
				})
			}
			byRepo[sb.Repo] = append(byRepo[sb.Repo], sb)
		}(item)
	}
	wg.Wait()

	// Concurrency makes arrival order arbitrary, so errors are sorted for a stable
	// snapshot just as the groups below are.
	sort.Slice(snap.ScopeErrors, func(i, j int) bool {
		if snap.ScopeErrors[i].Scope != snap.ScopeErrors[j].Scope {
			return snap.ScopeErrors[i].Scope < snap.ScopeErrors[j].Scope
		}
		return snap.ScopeErrors[i].Error < snap.ScopeErrors[j].Error
	})

	for repo, sandboxes := range byRepo {
		sort.Slice(sandboxes, func(i, j int) bool {
			if sandboxes[i].Name != sandboxes[j].Name {
				return sandboxes[i].Name < sandboxes[j].Name
			}
			return sandboxes[i].Scope < sandboxes[j].Scope
		})
		snap.Repos = append(snap.Repos, RepoGroup{Repo: repo, Name: repoName(repo), Sandboxes: sandboxes})
	}
	sort.Slice(snap.Repos, func(i, j int) bool { return snap.Repos[i].Name < snap.Repos[j].Name })
	return snap
}

// describe enriches one sbx sandbox with the facts sbx does not know. An
// unreadable worktree is returned as an error beside the row, never instead of it.
func (p *Poller) describe(ctx context.Context, scope string, s sbx.Sandbox) (Sandbox, error) {
	repo, worktree := p.locate(s)
	sb := Sandbox{
		Scope:    scope,
		Name:     s.Name,
		ID:       s.ID,
		Agent:    s.Agent,
		Status:   s.Status,
		Repo:     repo,
		RepoName: repoName(repo),
		Worktree: worktree,
	}
	if port, ok := s.PortFor(OpenCodePort); ok {
		sb.WebPort = port.HostPort
	}
	if worktree == "" {
		return sb, nil
	}

	facts, err := gitfacts.Read(ctx, repo, worktree)
	if err != nil {
		return sb, err
	}
	sb.Branch = facts.Branch
	sb.Dirty = facts.Dirty
	sb.Unmerged = facts.Unmerged
	sb.Missing = facts.Missing
	return sb, nil
}

// locate works out the repository and worktree from the workspaces sbx reports.
// scripts/sluss mounts two: the worktree, and the repository's .git directory that
// makes git work inside a linked worktree (docs/SPIKE.md assumption 1).
func (p *Poller) locate(s sbx.Sandbox) (repo, worktree string) {
	for _, w := range s.Workspaces {
		if filepath.Base(w) == ".git" {
			repo = filepath.Dir(w)
			continue
		}
		if worktree == "" {
			worktree = w
		}
	}
	if worktree == "" && repo != "" && p.cfg.WorktreeRoot != "" {
		worktree = filepath.Join(p.cfg.WorktreeRoot, filepath.Base(repo), s.Name)
	}
	if repo == "" && worktree != "" {
		// Only the worktree was mounted: <worktreeRoot>/<repo>/<name> names the
		// repository, so match that against the configured checkouts.
		wanted := filepath.Base(filepath.Dir(worktree))
		for _, candidate := range p.cfg.Repos {
			if filepath.Base(candidate) == wanted {
				repo = candidate
				break
			}
		}
	}
	return repo, worktree
}

func repoName(repo string) string {
	if repo == "" {
		return "unknown"
	}
	return filepath.Base(repo)
}
