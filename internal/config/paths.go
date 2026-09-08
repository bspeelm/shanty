// Package config reads and writes shanty's two configuration files and
// resolves the four directories it may write to.
package config

import (
	"errors"
	"os"
	"path/filepath"
)

const appName = "shanty"

// Paths holds the four directories shanty writes to.
type Paths struct {
	Config  string // settings and credentials, both user-editable
	State   string // resume positions and the scrobble backlog
	Cache   string // cover art; deleting it costs bandwidth only
	Runtime string // the mpv socket, gone at logout
}

// Discover resolves the four directories from the environment. It creates
// none of them.
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

	// Without XDG_RUNTIME_DIR the socket goes under the cache directory rather
	// than a shared temporary directory.
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

// xdgDir returns the directory named by an XDG environment variable, or the
// given path under the home directory. A relative value is an error.
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

// ControlSocket is the path a detached session is commanded through. It exists
// only while a session is detached.
func (p Paths) ControlSocket() string { return filepath.Join(p.Runtime, "control.sock") }

// CredentialsFile is the path to the file holding the credential.
func (p Paths) CredentialsFile() string { return filepath.Join(p.Config, "credentials.toml") }

// All returns the four directories shanty writes to.
func (p Paths) All() []string { return []string{p.Config, p.State, p.Cache, p.Runtime} }

// EnsureRuntime creates the runtime directory with 0700 permissions, and sets
// them on a directory that already exists.
func (p Paths) EnsureRuntime() error {
	if err := os.MkdirAll(p.Runtime, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves an existing directory’s permissions alone.
	return os.Chmod(p.Runtime, 0o700)
}
