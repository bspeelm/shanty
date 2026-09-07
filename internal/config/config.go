package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ConfigMode says in the mode that nothing here is secret, which is what makes
// the credentials file's 0600 mean something rather than being habit.
const ConfigMode os.FileMode = 0o644

// Config is everything a user could paste into a bug report without regret.
type Config struct {
	// Server is the base URL of the one host shanty talks to (§9).
	Server string `toml:"server"`
	// Username is not a secret, and lives here rather than beside the
	// credential so that the credential file holds exactly one thing.
	Username string `toml:"username"`
}

// LoadConfig treats a missing file as a first run, not a fault; Validate is
// what separates that from broken.
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

func (c Config) Save(path string) error {
	raw, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, ConfigMode)
}

// Validate names what to type on every failure: "invalid configuration" tells
// a user nothing they did not already suspect.
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
	// http is accepted: a LAN server and a test container are real setups, and
	// refusing them pushes this project's own user onto a client that asks
	// fewer questions. doctor warns, and the user can see the scheme they
	// typed. TLS *verification* is the one that cannot be weakened without
	// lying to them about what the connection proves, so it has no off switch
	// at all (ADR-004).
	return nil
}

// Insecure reports a plain-HTTP server, so doctor says so from one place
// rather than re-parsing the URL wherever it matters.
func (c Config) Insecure() bool {
	u, err := url.Parse(c.Server)
	return err == nil && u.Scheme == "http"
}
