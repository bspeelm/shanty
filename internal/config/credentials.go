package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CredentialsMode is the most permissive mode credentials.toml may have.
// Stricter permissions are accepted.
const CredentialsMode os.FileMode = 0o600

// AuthMode is a kind of credential.
type AuthMode string

const (
	AuthAPIKey       AuthMode = "api key"            // revocable server-side in a click
	AuthToken        AuthMode = "token and salt"     // replayable here, useless elsewhere
	AuthPasswordFile AuthMode = "password file"      // a secret the user's tooling manages
	AuthPassword     AuthMode = "plaintext password" // read, never written
)

// AuthModes lists every mode, strongest first.
var AuthModes = []AuthMode{AuthAPIKey, AuthToken, AuthPasswordFile, AuthPassword}

// PermissionError reports that the credentials file can be read by other users
// of the machine. Its message includes the chmod command that fixes it.
type PermissionError struct {
	Path string
	Mode os.FileMode
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf(
		"%s is mode %04o, so every account on this machine can read the credential in it.\nFix it with:  chmod 600 %s",
		e.Path, e.Mode, e.Path)
}

// ErrWontWritePassword is returned by Save when the credential is a plain
// password. shanty does not write passwords to disk.
var ErrWontWritePassword = errors.New(
	"shanty does not write plaintext passwords; remove the password line by hand, or set an api_key or token and salt instead")

// Credentials holds the credential. More than one kind may be set; Mode
// reports which is used.
type Credentials struct {
	APIKey       string `toml:"api_key,omitempty"`
	Token        string `toml:"token,omitempty"`
	Salt         string `toml:"salt,omitempty"`
	PasswordFile string `toml:"password_file,omitempty"`
	// Password is read if present and is never written. See persisted.
	Password string `toml:"password,omitempty"`
}

// persisted is the set of fields Save writes. It has no password field.
type persisted struct {
	APIKey       string `toml:"api_key,omitempty"`
	Token        string `toml:"token,omitempty"`
	Salt         string `toml:"salt,omitempty"`
	PasswordFile string `toml:"password_file,omitempty"`
}

// LoadCredentials reads credentials.toml. It returns a PermissionError if the
// file is readable by others, and the zero Credentials if it does not exist.
func LoadCredentials(path string) (Credentials, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, err
	}
	if perm := info.Mode().Perm(); perm&^CredentialsMode != 0 {
		return Credentials{}, &PermissionError{Path: path, Mode: perm}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := toml.Unmarshal(raw, &c); err != nil {
		return Credentials{}, fmt.Errorf("%s is not valid TOML: %w", path, err)
	}
	return c, nil
}

// Save writes credentials.toml atomically at CredentialsMode. It returns
// ErrWontWritePassword rather than storing a plain password.
func (c Credentials) Save(path string) error {
	if c.Password != "" {
		return ErrWontWritePassword
	}
	raw, err := toml.Marshal(persisted{
		APIKey:       c.APIKey,
		Token:        c.Token,
		Salt:         c.Salt,
		PasswordFile: c.PasswordFile,
	})
	if err != nil {
		return err
	}
	return writeAtomic(path, raw, CredentialsMode)
}

// Mode reports the strongest credential present. The second result is false
// when none is configured.
func (c Credentials) Mode() (AuthMode, bool) {
	switch {
	case c.APIKey != "":
		return AuthAPIKey, true
	case c.Token != "" && c.Salt != "":
		return AuthToken, true
	case c.PasswordFile != "":
		return AuthPasswordFile, true
	case c.Password != "":
		return AuthPassword, true
	}
	return "", false
}

// ResolvePassword returns the password for a password or password-file
// credential. The other modes return an error.
func (c Credentials) ResolvePassword() (string, error) {
	mode, ok := c.Mode()
	if !ok {
		return "", errors.New("no credential is configured")
	}
	switch mode {
	case AuthPassword:
		return c.Password, nil
	case AuthPasswordFile:
		return readPasswordFile(c.PasswordFile)
	default:
		return "", fmt.Errorf("the %s credential has no password behind it", mode)
	}
}

func readPasswordFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("password_file %s: %w", path, err)
	}
	// Permissions are checked for ordinary files only. A password_file may be a
	// pipe.
	if info.Mode().IsRegular() {
		if perm := info.Mode().Perm(); perm&^CredentialsMode != 0 {
			return "", &PermissionError{Path: path, Mode: perm}
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("password_file %s: %w", path, err)
	}
	// Trailing newlines are removed.
	return strings.TrimRight(string(raw), "\r\n"), nil
}
