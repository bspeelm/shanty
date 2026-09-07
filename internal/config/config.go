package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ConfigMode is what config.toml is written as. Nothing in it is secret, and
// saying so in the mode is what makes the credentials file's 0600 meaningful
// rather than habitual.
const ConfigMode os.FileMode = 0o644

// Config is the settings half: everything a user could paste into a bug report
// without regretting it.
type Config struct {
	// Server is the base URL of the one host shanty talks to (§9).
	Server string `toml:"server"`
	// Username is who we are to that server. It is not a secret, and it is
	// here rather than beside the credential so that the credential file holds
	// exactly one thing.
	Username string `toml:"username"`
}

// LoadConfig reads config.toml. A missing file is not an error: a first run
// has no settings yet, and the caller distinguishes "not configured" from
// "broken" by asking Validate.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := toml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("%s is not valid TOML: %w", path, err)
	}
	return c, nil
}

// Save writes config.toml atomically at ConfigMode.
func (c Config) Save(path string) error {
	raw, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, ConfigMode)
}

// Validate says whether these settings can be used, and every failure names
// what to type. "Invalid configuration" tells a user nothing they did not
// already suspect.
func (c Config) Validate() error {
	if c.Server == "" {
		return errors.New(`no server is configured; set it with: server = "https://music.example.org"`)
	}
	u, err := url.Parse(c.Server)
	if err != nil {
		return fmt.Errorf("server %q is not a URL: %w", c.Server, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf(`server %q has scheme %q; it should begin with https://`, c.Server, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf(`server %q names no host; it should look like https://music.example.org`, c.Server)
	}
	// http is accepted rather than refused. A server on a LAN or in a test
	// container is a real setup, and refusing it would push exactly the user
	// this project is for onto a client that asks fewer questions. doctor
	// warns; this does not fail. TLS *verification*, which is the thing that
	// cannot be weakened without lying to the user about it, has no off switch
	// at all (ADR-004).
	return nil
}

// Insecure reports whether the configured server is plain HTTP, so doctor can
// say so in one place rather than re-parsing the URL wherever it matters.
func (c Config) Insecure() bool {
	u, err := url.Parse(c.Server)
	return err == nil && u.Scheme == "http"
}
