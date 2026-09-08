package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/subsonic"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
)

const (
	user = "skipper"
	pass = "hornpipe"
)

// scratch builds an Env rooted entirely in a temporary tree, with mpv and its
// version supplied rather than looked for. A doctor test that asked this
// machine whether mpv is installed would pass here and fail in the buildroot.
func scratch(t *testing.T) (Env, *bytes.Buffer) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, "run"))

	paths, err := config.Discover()
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	return Env{
		Paths:    paths,
		Stdout:   out,
		Stderr:   out,
		LookPath: func(string) (string, error) { return "/usr/bin/mpv", nil },
		Command: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("mpv 0.38.0\n built on nothing\n"), nil
		},
	}, out
}

func configure(t *testing.T, env Env, cfg config.Config, creds config.Credentials) {
	t.Helper()
	if err := cfg.Save(env.Paths.ConfigFile()); err != nil {
		t.Fatal(err)
	}
	if err := creds.Save(env.Paths.CredentialsFile()); err != nil {
		t.Fatal(err)
	}
}

func byID(r Report) map[string]Result {
	out := map[string]Result{}
	for _, res := range r.Results {
		out[res.ID] = res
	}
	return out
}

// The closed-world assertion. A check that arrives without a line in checkIDs
// is one nobody decided the meaning of, and a line with no check behind it is
// a promise the report does not keep.
func TestTheDoctorChecksAreAClosedSet(t *testing.T) {
	env, _ := scratch(t)
	srv := fake.New(t, fake.Options{User: user, Password: pass, OpenSubsonic: true})
	configure(t, env, config.Config{Server: srv.URL, Username: user},
		config.Credentials{Token: "t", Salt: "s"})

	got := byID(diagnose(t.Context(), env))

	want := map[string]bool{}
	for _, id := range checkIDs {
		want[id] = true
		if _, ran := got[id]; !ran {
			t.Errorf("checkIDs lists %q, which the report does not contain", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("the report contains %q, which checkIDs does not list; decide what it should say and add it", id)
		}
	}
	if len(checkIDs) < 5 {
		t.Fatalf("checkIDs has %d entries; this assertion has stopped covering anything", len(checkIDs))
	}
}

// §5 as a command rather than a review standard: an error that names a problem
// without naming what to type is half an error.
func TestEveryFailureAndWarningCarriesItsFix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, env Env)
	}{
		{"nothing configured at all", func(*testing.T, Env) {}},
		{"mpv missing", func(t *testing.T, env Env) {}},
		{"a plaintext password", func(t *testing.T, env Env) {
			srv := fake.New(t, fake.Options{User: user, Password: pass})
			hand := filepath.Join(env.Paths.Config, "credentials.toml")
			if err := os.MkdirAll(env.Paths.Config, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(hand, []byte(`password = "`+pass+`"`+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := (config.Config{Server: srv.URL, Username: user}).Save(env.Paths.ConfigFile()); err != nil {
				t.Fatal(err)
			}
		}},
		{"a world-readable credential", func(t *testing.T, env Env) {
			srv := fake.New(t, fake.Options{User: user, Password: pass})
			configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
			if err := os.Chmod(env.Paths.CredentialsFile(), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"a plain-HTTP server", func(t *testing.T, env Env) {
			srv := fake.New(t, fake.Options{User: user, Password: pass})
			configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{Token: "t", Salt: "s"})
		}},
		{"a server that refuses the credential", func(t *testing.T, env Env) {
			srv := fake.New(t, fake.Options{User: user, Password: pass, Malice: fake.Malice{RejectAuth: true}})
			configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "wrong"})
		}},
		{"a server offering a better credential", func(t *testing.T, env Env) {
			srv := fake.New(t, fake.Options{User: user, Password: pass, OpenSubsonic: true})
			configure(t, env, config.Config{Server: srv.URL, Username: user},
				config.Credentials{Token: subsonicToken(pass, "s"), Salt: "s"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := scratch(t)
			if tc.name == "mpv missing" {
				env.LookPath = func(string) (string, error) { return "", errors.New("not found") }
			}
			tc.setup(t, env)

			report := diagnose(t.Context(), env)
			var checked int
			for _, res := range report.Results {
				if res.Severity != Fail && res.Severity != Warn {
					continue
				}
				checked++
				if res.Fix == "" {
					t.Errorf("%s is a %s with no fix: %q", res.ID, res.Severity, res.Summary)
				}
			}
			if checked == 0 {
				t.Skip("nothing failed or warned in this setup")
			}
		})
	}
}

func TestAHealthySetupPasses(t *testing.T) {
	env, out := scratch(t)
	srv := fake.New(t, fake.Options{User: user, Password: pass, OpenSubsonic: true})
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	// The fake accepts this key, so auth passes and auth-mode has nothing to
	// suggest.
	srvKey := fake.New(t, fake.Options{APIKey: "k-1", OpenSubsonic: true})
	configure(t, env, config.Config{Server: srvKey.URL, Username: user}, config.Credentials{APIKey: "k-1"})

	report := diagnose(t.Context(), env)
	for _, res := range report.Results {
		if res.Severity == Fail {
			t.Errorf("%s failed on a healthy setup: %s", res.ID, res.Summary)
		}
	}
	if !report.OK() {
		t.Error("a healthy setup did not report OK")
	}
	_ = out
}

// The credential failure sends the user to the credentials file, not to a
// generic "unauthorized".
func TestARefusedCredentialNamesTheFileToEdit(t *testing.T) {
	env, _ := scratch(t)
	srv := fake.New(t, fake.Options{User: user, Password: pass, Malice: fake.Malice{RejectAuth: true}})
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "wrong"})

	got := byID(diagnose(t.Context(), env))
	if got["auth"].Severity != Fail {
		t.Fatalf("auth = %s, want fail", got["auth"].Severity)
	}
	if !strings.Contains(got["auth"].Fix, "credentials.toml") {
		t.Errorf("the fix does not name the credentials file: %q", got["auth"].Fix)
	}
	// And the server itself answered, so it must not be reported as
	// unreachable: that would send the user to their firewall.
	if got["server"].Severity == Fail {
		t.Errorf("a reachable server that refused us was reported unreachable: %q", got["server"].Summary)
	}
}

// ADR-001's preference order, told to the user rather than only known here.
func TestDoctorNamesABetterCredentialWhenTheServerOffersOne(t *testing.T) {
	env, _ := scratch(t)
	srv := fake.New(t, fake.Options{User: user, Password: pass, OpenSubsonic: true})
	configure(t, env, config.Config{Server: srv.URL, Username: user},
		config.Credentials{Token: subsonicToken(pass, "s"), Salt: "s"})

	got := byID(diagnose(t.Context(), env))
	if got["auth-mode"].Severity != Warn {
		t.Fatalf("auth-mode = %s on a server offering API keys, want warn", got["auth-mode"].Severity)
	}
	if !strings.Contains(got["auth-mode"].Fix, "api_key") {
		t.Errorf("the fix does not say what to set: %q", got["auth-mode"].Fix)
	}
}

// A command that only reports must leave nothing behind -- otherwise
// `sudo shanty doctor` makes a root-owned runtime directory that breaks every
// later run.
func TestDoctorCreatesNothing(t *testing.T) {
	env, _ := scratch(t)
	home := os.Getenv("HOME")

	if err := runDoctor(t.Context(), env, nil); err == nil {
		t.Fatal("doctor passed on an empty setup")
	}

	var found []string
	_ = filepath.WalkDir(home, func(path string, _ os.DirEntry, _ error) error {
		if path != home {
			found = append(found, strings.TrimPrefix(path, home))
		}
		return nil
	})
	sort.Strings(found)
	if len(found) != 0 {
		t.Errorf("doctor left %v behind", found)
	}
}

func TestDoctorJSONIsMachineReadable(t *testing.T) {
	env, out := scratch(t)
	_ = runDoctor(t.Context(), env, []string{"-json"})

	var report Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("--json did not produce JSON: %v\n%s", err, out)
	}
	if len(report.Results) != len(checkIDs) {
		t.Errorf("JSON carried %d results, want %d", len(report.Results), len(checkIDs))
	}
	for _, res := range report.Results {
		if res.ID == "" || res.Severity == "" {
			t.Errorf("a result is missing its id or severity: %+v", res)
		}
	}
}

// subsonicToken keeps the test's credential honest without importing the
// client's hashing into every case.
func subsonicToken(password, salt string) string { return subsonic.Token(password, salt) }

// A fix line that names a command the machine cannot run is not a fix line.
// On an rpm-ostree root, `dnf install` cannot write /usr, and a flatpak mpv
// cannot see the socket shanty creates because its sandbox remaps
// XDG_RUNTIME_DIR.
func TestTheMpvFixNamesACommandThatWorksOnThisKindOfMachine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		immutable bool
		want      string
		notWant   string
	}{
		{name: "an ordinary root", immutable: false, want: "dnf install mpv", notWant: "rpm-ostree"},
		{name: "an rpm-ostree root", immutable: true, want: "rpm-ostree install mpv", notWant: "dnf install mpv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, _ := scratch(t)
			env.LookPath = func(string) (string, error) { return "", errors.New("not found") }
			env.ImmutableHost = tc.immutable

			got := byID(diagnose(t.Context(), env))["mpv"]
			if got.Severity != Fail {
				t.Fatalf("mpv missing reported %s", got.Severity)
			}
			if !strings.Contains(got.Fix, tc.want) {
				t.Errorf("the fix does not name %q:\n%s", tc.want, got.Fix)
			}
			if strings.Contains(got.Fix, tc.notWant) {
				t.Errorf("the fix names %q, which does not work here:\n%s", tc.notWant, got.Fix)
			}
		})
	}
}

// TestTheDoctorHoldsMpvToAMinimumVersion covers the number and the reason for
// it: shanty always passes --prefetch-playlist, which arrived in 0.24.0.
func TestTheDoctorHoldsMpvToAMinimumVersion(t *testing.T) {
	for _, tc := range []struct {
		banner   string
		severity Severity
		says     string
	}{
		{"mpv v0.41.0 Copyright", Pass, "0.41.0"},
		{"mpv v0.24.0 Copyright", Pass, "0.24.0"},
		{"mpv v0.23.0 Copyright", Fail, "too old"},
		{"mpv v0.17.0 Copyright", Fail, "too old"},
		{"mpv, but it will not say which", Warn, "did not say which version"},
	} {
		t.Run(tc.banner, func(t *testing.T) {
			env, _ := scratch(t)
			env.Command = func(context.Context, string, ...string) ([]byte, error) {
				return []byte(tc.banner + "\n"), nil
			}

			got := checkMpvVersion(t.Context(), env, Result{ID: "mpv", Severity: Pass})

			if got.Severity != tc.severity {
				t.Errorf("severity %v, want %v (%s)", got.Severity, tc.severity, got.Summary)
			}
			if !strings.Contains(got.Summary+got.Detail+got.Fix, tc.says) {
				t.Errorf("the report reads %q / %q / %q, want it to mention %q",
					got.Summary, got.Detail, got.Fix, tc.says)
			}
			if tc.severity != Pass && got.Fix == "" {
				t.Error("a problem with nothing to do about it")
			}
			if tc.severity == Fail && !strings.Contains(got.Detail+got.Fix, "0.24.0") {
				t.Errorf("a version too old does not name the minimum: %q / %q", got.Detail, got.Fix)
			}
		})
	}
}
