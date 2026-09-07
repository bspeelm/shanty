package config

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const secret = "correct-horse-battery-staple"

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile applies the umask, which on a developer's machine quietly
	// turns 0666 into 0644 and would make this suite pass for the wrong
	// reason.
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// This is §6 as a test, and it refuses rather than warns. Every mode a second
// account could read from is a mode shanty will not start on.
func TestCredentialsPermissions(t *testing.T) {
	for _, tc := range []struct {
		mode   os.FileMode
		accept bool
	}{
		{mode: 0o600, accept: true},
		{mode: 0o400, accept: true},  // what an agenix secret arrives as
		{mode: 0o640, accept: false}, // the group can read it
		{mode: 0o604, accept: false}, // everyone can read it
		{mode: 0o644, accept: false},
		{mode: 0o666, accept: false},
		{mode: 0o700, accept: false}, // executable is not a credential's business
	} {
		t.Run(tc.mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials.toml")
			write(t, path, "api_key = \"k-1\"\n", tc.mode)

			got, err := LoadCredentials(path)
			if tc.accept {
				if err != nil {
					t.Fatalf("mode %04o was refused: %v", tc.mode, err)
				}
				if got.APIKey != "k-1" {
					t.Errorf("api key = %q, want k-1", got.APIKey)
				}
				return
			}

			var perr *PermissionError
			if !errors.As(err, &perr) {
				t.Fatalf("mode %04o was accepted; err = %v", tc.mode, err)
			}
			// §5: every failure line carries the command that fixes it.
			if !strings.Contains(err.Error(), "chmod 600 "+path) {
				t.Errorf("the refusal does not say what to type:\n%s", err)
			}
		})
	}
}

// A missing credentials file is a first run, not a fault. Refusing here would
// mean shanty could never tell a new user what to do next.
func TestAMissingCredentialsFileIsNotAnError(t *testing.T) {
	got, err := LoadCredentials(filepath.Join(t.TempDir(), "credentials.toml"))
	if err != nil {
		t.Fatalf("a missing file errored: %v", err)
	}
	if _, ok := got.Mode(); ok {
		t.Error("a missing file produced a usable credential")
	}
}

// §5's second named gate. The plaintext password is not a field on the type
// Save marshals, so this holds by construction -- and the test exists so that
// reintroducing the field has to fail something.
func TestNoPlaintextPasswordPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials.toml")

	t.Run("saving one is refused, not silently dropped", func(t *testing.T) {
		err := Credentials{Password: secret}.Save(path)
		if !errors.Is(err, ErrWontWritePassword) {
			t.Fatalf("Save(password) err = %v, want ErrWontWritePassword", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Error("a refused save left a file behind")
		}
	})

	t.Run("the secret never reaches disk beside another credential", func(t *testing.T) {
		// The migration a user actually performs: a hand-written password is
		// in memory, an API key has just been obtained, and the API key is
		// saved. Nothing about that flow may carry the password along.
		loaded := Credentials{Password: secret}
		loaded.Password = ""
		loaded.APIKey = "k-1"
		if err := loaded.Save(path); err != nil {
			t.Fatal(err)
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) {
			t.Errorf("the password reached disk:\n%s", raw)
		}
		if strings.Contains(string(raw), "password") {
			t.Errorf("a password key reached disk:\n%s", raw)
		}
	})

	t.Run("but a hand-written one is still read", func(t *testing.T) {
		hand := filepath.Join(dir, "hand.toml")
		write(t, hand, "password = \""+secret+"\"\n", 0o600)

		got, err := LoadCredentials(hand)
		if err != nil {
			t.Fatal(err)
		}
		// Refusing to read it would only move the user to a client that will.
		if got.Password != secret {
			t.Errorf("password = %q, want it read as written", got.Password)
		}
		if mode, _ := got.Mode(); mode != AuthPassword {
			t.Errorf("mode = %q, want %q", mode, AuthPassword)
		}
	})
}

func TestModeFollowsThePreferenceOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   Credentials
		want AuthMode
	}{
		{name: "nothing", in: Credentials{}, want: ""},
		{name: "only a password", in: Credentials{Password: secret}, want: AuthPassword},
		{name: "only a password file", in: Credentials{PasswordFile: "/run/secret"}, want: AuthPasswordFile},
		{name: "only a token", in: Credentials{Token: "t", Salt: "s"}, want: AuthToken},
		{name: "only an api key", in: Credentials{APIKey: "k"}, want: AuthAPIKey},
		{
			name: "a token without its salt is not a token",
			in:   Credentials{Token: "t", Password: secret},
			want: AuthPassword,
		},
		{
			name: "an api key outranks everything beneath it",
			in:   Credentials{APIKey: "k", Token: "t", Salt: "s", PasswordFile: "/run/s", Password: secret},
			want: AuthAPIKey,
		},
		{
			name: "a token outranks a password the user has not removed yet",
			in:   Credentials{Token: "t", Salt: "s", Password: secret},
			want: AuthToken,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.in.Mode()
			if got != tc.want {
				t.Errorf("Mode() = %q, want %q", got, tc.want)
			}
			if ok != (tc.want != "") {
				t.Errorf("Mode() ok = %v, want %v", ok, tc.want != "")
			}
		})
	}
}

// AuthModes is a closed-world set: doctor reads it to name a better option, so
// a mode that exists but is missing from the ranking would be one doctor never
// suggests. Asserted in both directions against the source itself, because the
// two can only drift apart in one file.
func TestEveryAuthModeIsRanked(t *testing.T) {
	declared := map[AuthMode]string{}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "credentials.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if id, ok := spec.Type.(*ast.Ident); !ok || id.Name != "AuthMode" {
			return true
		}
		for i, name := range spec.Names {
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			declared[AuthMode(strings.Trim(lit.Value, `"`))] = name.Name
		}
		return true
	})

	if len(declared) < 2 {
		t.Fatalf("the source parser found %d AuthMode constants; it has stopped matching", len(declared))
	}

	ranked := map[AuthMode]bool{}
	for _, m := range AuthModes {
		ranked[m] = true
		if _, found := declared[m]; !found {
			t.Errorf("AuthModes ranks %q, which is not a declared AuthMode constant", m)
		}
	}
	for m, name := range declared {
		if !ranked[m] {
			t.Errorf("%s (%q) is declared but not in AuthModes; decide where it ranks against the others", name, m)
		}
	}
}

func TestResolvePassword(t *testing.T) {
	dir := t.TempDir()

	t.Run("a password file is read and its trailing newline dropped", func(t *testing.T) {
		path := filepath.Join(dir, "secret")
		write(t, path, secret+"\n", 0o600)

		got, err := Credentials{PasswordFile: path}.ResolvePassword()
		if err != nil {
			t.Fatal(err)
		}
		// Every editor and every secret manager adds this newline, and a
		// password carrying one authenticates against nothing while looking
		// perfectly correct in the file.
		if got != secret {
			t.Errorf("resolved %q, want %q", got, secret)
		}
	})

	t.Run("a readable password file is refused like our own", func(t *testing.T) {
		path := filepath.Join(dir, "loose")
		write(t, path, secret, 0o644)

		_, err := Credentials{PasswordFile: path}.ResolvePassword()
		var perr *PermissionError
		if !errors.As(err, &perr) {
			t.Fatalf("err = %v, want a PermissionError", err)
		}
	})

	t.Run("a missing password file says which one", func(t *testing.T) {
		path := filepath.Join(dir, "absent")
		_, err := Credentials{PasswordFile: path}.ResolvePassword()
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("err = %v, want it to name %s", err, path)
		}
	})

	t.Run("an api key has no password behind it", func(t *testing.T) {
		got, err := Credentials{APIKey: "k-1"}.ResolvePassword()
		if err == nil {
			t.Fatalf("resolved %q from an api key; an empty string here hashes into a valid-looking token", got)
		}
	})

	t.Run("nothing configured says so", func(t *testing.T) {
		if _, err := (Credentials{}).ResolvePassword(); err == nil {
			t.Error("an empty credential resolved a password")
		}
	})
}

func TestSaveWritesCredentialsAt0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "credentials.toml")
	if err := (Credentials{APIKey: "k-1"}).Save(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != CredentialsMode {
		t.Errorf("credentials mode = %04o, want %04o", got, CredentialsMode)
	}

	// The directory Save created holds a credential, so it may not be one
	// another account can list.
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dir.Mode().Perm(); got&^0o700 != 0 {
		t.Errorf("credentials directory mode = %04o, want no bits outside 0700", got)
	}

	// And it round-trips, which is the only reason to write it at all.
	back, err := LoadCredentials(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.APIKey != "k-1" {
		t.Errorf("api key round-tripped as %q", back.APIKey)
	}
}
