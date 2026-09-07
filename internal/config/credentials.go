package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CredentialsMode is the only mode a credentials file may have. Stricter is
// accepted -- 0400 is what an agenix secret arrives as -- but anything a
// second account can read is refused outright.
const CredentialsMode os.FileMode = 0o600

// AuthMode is how shanty proves who it is to the server.
type AuthMode string

const (
	// AuthAPIKey is one revocable string, killed server-side in a click.
	AuthAPIKey AuthMode = "api key"
	// AuthToken is replayable against this server and useless anywhere else,
	// because it is not the password.
	AuthToken AuthMode = "token and salt"
	// AuthPasswordFile is a secret the user's own tooling manages.
	AuthPasswordFile AuthMode = "password file"
	// AuthPassword is a password shanty will read and will never write.
	AuthPassword AuthMode = "plaintext password"
)

// AuthModes is every mode, strongest first. ADR-001's preference order is this
// slice and nothing else, so doctor names a better option by reading it rather
// than by carrying a second copy of the ranking that could disagree.
var AuthModes = []AuthMode{AuthAPIKey, AuthToken, AuthPasswordFile, AuthPassword}

// PermissionError is what a readable credentials file gets. It is an error
// rather than a warning on purpose: the audit that started this project found
// the field's best-engineered client shipping a world-readable password for
// want of exactly this check, and a warning is a thing people scroll past.
type PermissionError struct {
	Path string
	Mode os.FileMode
}

func (e *PermissionError) Error() string {
	return fmt.Sprintf(
		"%s is mode %04o, so every account on this machine can read the credential in it.\nFix it with:  chmod 600 %s",
		e.Path, e.Mode, e.Path)
}

// ErrWontWritePassword is returned rather than silently dropping the line. A
// user who wrote a password by hand gets to keep it; shanty declines to be the
// program that put it on disk, and equally declines to be the one that deleted
// it without saying so.
var ErrWontWritePassword = errors.New(
	"shanty does not write plaintext passwords; remove the password line by hand, or set an api_key or token and salt instead")

// Credentials is the secret half. Exactly one mode is used -- Mode picks it --
// but more than one may be present, because a user migrating from a password
// to an API key should not have to get the order right.
type Credentials struct {
	APIKey       string `toml:"api_key,omitempty"`
	Token        string `toml:"token,omitempty"`
	Salt         string `toml:"salt,omitempty"`
	PasswordFile string `toml:"password_file,omitempty"`
	// Password is read if a user wrote one, because refusing to read it only
	// moves them to a client that will. It is never written: see persisted.
	Password string `toml:"password,omitempty"`
}

// persisted is what Save is allowed to write, and the plaintext password is
// not a field on it. "shanty never writes a password" is therefore a property
// of the type rather than a check somebody could forget to run, or delete to
// make a test go green.
type persisted struct {
	APIKey       string `toml:"api_key,omitempty"`
	Token        string `toml:"token,omitempty"`
	Salt         string `toml:"salt,omitempty"`
	PasswordFile string `toml:"password_file,omitempty"`
}

// LoadCredentials reads credentials.toml, refusing to go on if anyone else on
// the machine could have read it first. A missing file is not an error; an
// unprotected one is.
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

// Save writes credentials.toml atomically at 0600, and refuses outright when
// asked to persist a plaintext password.
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

// Mode reports which credential shanty will actually use, following ADR-001's
// order. The second return is false when nothing is configured at all.
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

// ResolvePassword returns the plaintext behind a password-file or password
// credential, for the one caller that has to hash it. The other two modes have
// no password behind them and say so rather than returning an empty string
// that would hash into a valid-looking token.
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
	// The mode check applies to regular files only. §6 invites a pass(1) entry
	// through process substitution, which arrives as a pipe, and a pipe's
	// permissions are not the thing protecting it.
	if info.Mode().IsRegular() {
		if perm := info.Mode().Perm(); perm&^CredentialsMode != 0 {
			return "", &PermissionError{Path: path, Mode: perm}
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("password_file %s: %w", path, err)
	}
	// A trailing newline is what every editor and every secret manager adds,
	// and a password with one on the end authenticates against nothing while
	// looking correct in the file.
	return strings.TrimRight(string(raw), "\r\n"), nil
}
