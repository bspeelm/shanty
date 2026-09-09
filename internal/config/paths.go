// Package config reads and writes shanty's two configuration files and
// resolves the four directories it may write to.
package config

import (
	"errors"
	"os"
	"path/filepath"
)

const appName = "shanty"

// Paths holds the four directories shanty writes to, and the completion files
// its installer writes beside the binary.
type Paths struct {
	Config  string // settings and credentials, both user-editable
	State   string // resume positions and the scrobble backlog
	Cache   string // cover art; deleting it costs bandwidth only
	Runtime string // the mpv socket, gone at logout

	// dataHome is where a shell reads completions from. It is not one of the
	// four directories, and nothing but the installer writes there.
	dataHome string
}

// Discover resolves the four directories from the environment. It creates
// none of them.
//
// All four follow the XDG base directory specification on every platform,
// including macOS, where the standard library resolves the same names to
// ~/Library instead.
func Discover() (Paths, error) {
	configHome, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return Paths{}, err
	}
	cacheHome, err := xdgDir("XDG_CACHE_HOME", ".cache")
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

	dataHome, err := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		Config:   filepath.Join(configHome, appName),
		State:    filepath.Join(stateHome, appName),
		Cache:    filepath.Join(cacheHome, appName),
		Runtime:  runtime,
		dataHome: dataHome,
	}, nil
}

// Completions are the files a shell reads to complete shanty's commands, by
// the name of the shell.
//
// They are not shanty's own data. A shell reads completions from a directory
// of its own and nowhere else, so a file kept with the four directories would
// never be read. Nothing writes these but the installer, and `shanty
// uninstall` removes them.
func (p Paths) Completions() map[string]string {
	if p.dataHome == "" {
		return nil
	}
	// fish reads its configuration from the same place shanty does, so the
	// directory holding shanty's own is the one to start from.
	configHome := filepath.Dir(p.Config)
	return map[string]string{
		"bash": filepath.Join(p.dataHome, "bash-completion", "completions", appName),
		"fish": filepath.Join(configHome, "fish", "completions", appName+".fish"),
	}
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

// SocketLimit is the longest a unix socket path may be.
//
// The kernel's own limit is 108 bytes on Linux and 104 on macOS, including the
// end of the string, so the shorter of the two is the one to hold to. A path
// over it fails to bind with "invalid argument", which says nothing about
// length.
const SocketLimit = 103

// TooLongForASocket reports whether a path cannot hold a socket, and by how
// much.
func TooLongForASocket(path string) (int, bool) {
	if over := len(path) - SocketLimit; over > 0 {
		return over, true
	}
	return 0, false
}

// Backlog is the file holding plays the server has not accepted.
func (p Paths) Backlog() string { return filepath.Join(p.State, "plays.jsonl") }

// Covers is the directory cover art is kept in.
func (p Paths) Covers() string { return filepath.Join(p.Cache, "covers") }

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
