package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ConfigMode is the permission config.toml is written with.
const ConfigMode os.FileMode = 0o644

// Config holds the server's address and the username.
type Config struct {
	// Server is the base URL of the server shanty talks to.
	Server string `toml:"server"`
	// Username is the account name on that server.
	Username string `toml:"username"`
	// Keys binds an action to the keys that do it, replacing the ones shanty
	// comes with. An action not named here keeps its own.
	Keys map[string][]string `toml:"keys"`
	// AutoVolume levels tracks against each other using the loudness a server
	// recorded in their tags. It changes the volume and nothing else: an
	// untagged track is played as it is.
	AutoVolume bool `toml:"auto_volume"`
}

// LoadConfig reads config.toml. A missing file returns the zero Config and no
// error.
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

// Validate reports whether the settings can be used. Each error names the line
// to change.
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
	// Plain HTTP is accepted and reported by Insecure. Certificate verification
	// cannot be disabled by any setting.
	return nil
}

// Insecure reports whether the configured server uses plain HTTP.
func (c Config) Insecure() bool {
	u, err := url.Parse(c.Server)
	return err == nil && u.Scheme == "http"
}
