// Package config owns every file shanty reads or writes, and every directory
// it is allowed to write one into.
//
// The confinement is the point. §8 names four directories and says "nothing
// else, ever"; keeping the resolution of those four in one package is what
// lets the isolation suite ask a single question -- did anything land outside
// Paths.All() -- rather than auditing call sites forever.
package config

import (
	"errors"
	"os"
	"path/filepath"
)

const appName = "shanty"

// Paths is the whole filesystem contract. Nothing outside these four
// directories is shanty's to touch.
type Paths struct {
	// Config holds settings and credentials, both user-editable.
	Config string
	// State holds resume positions and the scrobble backlog: losing it costs
	// a little history.
	State string
	// Cache holds cover art. Deleting it costs bandwidth only.
	Cache string
	// Runtime holds the mpv socket and is gone at logout.
	Runtime string
}

// Discover resolves the four directories from the environment without
// creating any of them. A command that only reports -- doctor, or a --help --
// must not leave a directory behind as the price of running.
func Discover() (Paths, error) {
	configHome, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	cacheHome, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, err
	}
	stateHome, err := xdgDir("XDG_STATE_HOME", filepath.Join(".local", "state"))
	if err != nil {
		return Paths{}, err
	}

	// A shared /tmp is never the fallback. The socket name is predictable, so
	// a directory another account can write to is a directory where somebody
	// else's socket can be waiting under our name.
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime != "" && !filepath.IsAbs(runtime) {
		return Paths{}, errors.New("path in $XDG_RUNTIME_DIR is relative")
	}
	if runtime == "" {
		runtime = filepath.Join(cacheHome, appName, "run")
	} else {
		runtime = filepath.Join(runtime, appName)
	}

	return Paths{
		Config:  filepath.Join(configHome, appName),
		State:   filepath.Join(stateHome, appName),
		Cache:   filepath.Join(cacheHome, appName),
		Runtime: runtime,
	}, nil
}

// xdgDir mirrors what os.UserConfigDir does for the variables the standard
// library has no helper for, including its refusal of a relative path: an XDG
// variable that is not absolute is ignored by the specification, and silently
// resolving it against the working directory would write shanty's state into
// whatever directory it happened to be started from.
func xdgDir(env string, fallback string) (string, error) {
	if dir := os.Getenv(env); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", errors.New("path in $" + env + " is relative")
		}
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback), nil
}

// ConfigFile is the settings file: 0644, nothing secret in it.
func (p Paths) ConfigFile() string { return filepath.Join(p.Config, "config.toml") }

// CredentialsFile is the secret half, 0600, in a file of its own so that the
// settings can be pasted into a bug report and the credential cannot.
func (p Paths) CredentialsFile() string { return filepath.Join(p.Config, "credentials.toml") }

// Socket is where mpv listens. §7 puts it under Runtime in a 0700 directory.
func (p Paths) Socket() string { return filepath.Join(p.Runtime, "mpv.sock") }

// All is the write set, and it is closed. uninstall removes exactly these, the
// isolation suite asserts nothing landed outside them, and a fifth directory
// added anywhere else fails both.
func (p Paths) All() []string { return []string{p.Config, p.State, p.Cache, p.Runtime} }

// EnsureRuntime creates the runtime directory at 0700 and returns it. The mode
// is not advisory: the socket inside carries a live connection to a player
// holding a credential-bearing URL.
func (p Paths) EnsureRuntime() error {
	if err := os.MkdirAll(p.Runtime, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves an existing directory's mode alone, so a runtime
	// directory from an older version -- or from anything else that got there
	// first -- is tightened rather than trusted.
	return os.Chmod(p.Runtime, 0o700)
}
