// Package config owns every file shanty reads or writes, and every directory
// it may write one into. §8 names four directories and says "nothing else,
// ever"; resolving all four here is what lets the isolation suite ask one
// question -- did anything land outside Paths.All() -- rather than auditing
// call sites forever.
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
	Config  string // settings and credentials, both user-editable
	State   string // resume positions and the scrobble backlog
	Cache   string // cover art; deleting it costs bandwidth only
	Runtime string // the mpv socket, gone at logout
}

// Discover resolves the four directories without creating any of them: a
// command that only reports must not leave a directory behind as the price of
// having run.
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

	// Never a shared /tmp: the socket name is predictable, so a directory
	// another account can write to is one where somebody else's socket can be
	// waiting under our name.
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

// xdgDir mirrors os.UserConfigDir for the variables the standard library has
// no helper for, refusal of relative paths included: resolving one against the
// working directory would write shanty's state into whatever directory it
// happened to be started from.
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

func (p Paths) ConfigFile() string { return filepath.Join(p.Config, "config.toml") }
func (p Paths) Socket() string     { return filepath.Join(p.Runtime, "mpv.sock") }

// CredentialsFile is 0600 and separate from the settings, so config.toml can be
// pasted into a bug report and the credential cannot.
func (p Paths) CredentialsFile() string { return filepath.Join(p.Config, "credentials.toml") }

// All is the write set, and it is closed: uninstall removes exactly these and
// the isolation suite asserts nothing landed outside them.
func (p Paths) All() []string { return []string{p.Config, p.State, p.Cache, p.Runtime} }

// EnsureRuntime creates the runtime directory at 0700. The mode is not
// advisory: the socket inside reaches a player holding a credential-bearing URL.
func (p Paths) EnsureRuntime() error {
	if err := os.MkdirAll(p.Runtime, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves an existing directory's mode alone, so one that was
	// already there is tightened rather than trusted.
	return os.Chmod(p.Runtime, 0o700)
}
