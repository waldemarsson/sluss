// Package config loads sluss's single hand-edited JSON configuration file.
//
// sluss holds no persistent state, so this file is the only thing it reads that a
// human wrote. It is never written back: the GUI edits kits and secrets, not its
// own configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Access modes. "path" needs no DNS, no wildcard certificate and no reverse proxy;
// "host" serves each sandbox at its own hostname and needs both.
const (
	AccessPath = "path"
	AccessHost = "host"
)

// Defaults applied to any field the file leaves out.
const (
	DefaultPort       = 8420
	DefaultHostPrefix = "sluss-"
)

// Sentinel errors, so callers (and tests) can match a cause with errors.Is rather
// than comparing message strings. Wrapping with %w keeps the detail *and* the cause.
var (
	ErrInvalidAccess  = errors.New("access must be \"path\" or \"host\"")
	ErrMissingDomain  = errors.New("access \"host\" requires domain")
	ErrPortRange      = errors.New("port must be between 1 and 65535")
	ErrNoAppNames     = errors.New("appNames must list at least one sbx scope")
	ErrNoWorktreeRoot = errors.New("worktreeRoot must be set")
)

// Config mirrors ~/.config/sluss/config.json one-to-one. The JSON tags are the
// file's real key names; Go's field names are capitalised because that is what
// makes them visible outside this package.
type Config struct {
	LAN          bool     `json:"lan"`          // bind beyond loopback
	Port         int      `json:"port"`         // the one listener
	Access       string   `json:"access"`       // AccessPath or AccessHost
	HostPrefix   string   `json:"hostPrefix"`   // host mode only
	Domain       string   `json:"domain"`       // host mode only
	AppNames     []string `json:"appNames"`     // sbx scopes to poll
	Repos        []string `json:"repos"`        // repositories sandboxes may be created from
	KitsDir      string   `json:"kitsDir"`      // directory of hand-authored kits
	WorktreeRoot string   `json:"worktreeRoot"` // where scripts/sluss puts worktrees
}

// DefaultPath is where Load looks when no path is given on the command line.
func DefaultPath() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "sluss", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home directory is not worth failing over here; the caller will get a
		// readable "reading config" error from the relative path instead.
		return filepath.Join(".config", "sluss", "config.json")
	}
	return filepath.Join(home, ".config", "sluss", "config.json")
}

// Load reads, defaults and validates the configuration file. Every error names the
// file it came from, because a wrong path is the most likely mistake.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var c Config
	dec := json.NewDecoder(bytes.NewReader(data))
	// A typo in a hand-edited file should be an error, not a silently ignored key.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.Access == "" {
		c.Access = AccessPath
	}
	if c.HostPrefix == "" {
		c.HostPrefix = DefaultHostPrefix
	}
	if c.WorktreeRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			c.WorktreeRoot = filepath.Join(home, "src", "worktrees")
		}
	}
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port %d: %w", c.Port, ErrPortRange)
	}
	if c.Access != AccessPath && c.Access != AccessHost {
		return fmt.Errorf("access %q: %w", c.Access, ErrInvalidAccess)
	}
	if c.Access == AccessHost && c.Domain == "" {
		return ErrMissingDomain
	}
	if len(c.AppNames) == 0 {
		return ErrNoAppNames
	}
	if c.WorktreeRoot == "" {
		return ErrNoWorktreeRoot
	}
	return nil
}

// BindAddr is the only place the lan flag is interpreted. Everything else asks here,
// so "sluss binds exactly one listener" stays a property of one function.
func (c *Config) BindAddr() string {
	if c.LAN {
		return fmt.Sprintf(":%d", c.Port)
	}
	return fmt.Sprintf("127.0.0.1:%d", c.Port)
}

// HasScope reports whether an app-name is one this sluss is configured to touch.
// Every request carrying a scope is checked against it before reaching sbx.
func (c *Config) HasScope(appName string) bool {
	for _, s := range c.AppNames {
		if s == appName {
			return true
		}
	}
	return false
}

// HasRepo reports whether a repository path is one sandboxes may be created from.
func (c *Config) HasRepo(path string) bool {
	for _, r := range c.Repos {
		if r == path {
			return true
		}
	}
	return false
}
