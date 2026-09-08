package main

import (
	"os"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
)

// answering builds an Env that replies to the prompts from a script, so the
// whole flow runs without a terminal.
func answering(t *testing.T, typed string, secrets ...string) (Env, *strings.Builder) {
	t.Helper()
	env, out := scratch(t)
	env.Stdin = strings.NewReader(typed)
	env.ReadSecret = func(string) (string, error) {
		if len(secrets) == 0 {
			return "", nil
		}
		s := secrets[0]
		secrets = secrets[1:]
		return s, nil
	}
	shown := &strings.Builder{}
	env.Stdout = shown
	_ = out
	return env, shown
}

// The whole point: a password is taken once and never written. What lands is
// the token the protocol sends, which works against this server and nowhere
// else (ADR-001).
func TestSetupWritesATokenAndNeverThePassword(t *testing.T) {
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	env, shown := answering(t, srv.URL+"\n"+user+"\n", "", pass)

	if err := runSetup(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(env.Paths.CredentialsFile())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), pass) {
		t.Fatalf("the password reached disk:\n%s", raw)
	}
	if !strings.Contains(string(raw), "token") || !strings.Contains(string(raw), "salt") {
		t.Errorf("no token and salt were written:\n%s", raw)
	}
	// And it says so, because what was not written matters as much as what was.
	if !strings.Contains(shown.String(), "password was not saved") {
		t.Errorf("setup does not say the password was not saved:\n%s", shown)
	}

	// The credential it wrote is one the server accepts.
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(env.Paths.ConfigFile())
	if err != nil {
		t.Fatal(err)
	}
	client, err := dial(env, cfg, creds)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ping(t.Context()); err != nil {
		t.Errorf("the credential setup wrote does not work: %v", err)
	}
}

func TestSetupPrefersAnAPIKeyWhenOneIsGiven(t *testing.T) {
	srv := fake.New(t, fake.Options{APIKey: "k-1", OpenSubsonic: true})
	env, _ := answering(t, srv.URL+"\n"+user+"\n", "k-1")

	if err := runSetup(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		t.Fatal(err)
	}
	if mode, _ := creds.Mode(); mode != config.AuthAPIKey {
		t.Errorf("setup stored the %s, want the api key", mode)
	}
}

// A typo is a question asked again, not a config file that looks right and
// does not work.
func TestSetupWritesNothingWhenTheServerRefuses(t *testing.T) {
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	env, _ := answering(t, srv.URL+"\n"+user+"\n", "", "the-wrong-password")

	err := runSetup(t.Context(), env, nil)
	if err == nil {
		t.Fatal("setup accepted a credential the server refused")
	}
	if !strings.Contains(err.Error(), "Nothing was written") {
		t.Errorf("the failure does not say the files were left alone: %v", err)
	}
	for _, path := range []string{env.Paths.ConfigFile(), env.Paths.CredentialsFile()} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("%s was written despite the credential failing", path)
		}
	}
}

func TestSetupRefusesAServerThatIsNotAURL(t *testing.T) {
	env, _ := answering(t, "not a url\n", "")
	if err := runSetup(t.Context(), env, nil); err == nil {
		t.Fatal("setup accepted a server that is not a URL")
	}
}

// Without a terminal there is nowhere safe to type a password, and reading one
// from a pipe leaves it in whatever produced the pipe.
func TestSetupRefusesWithoutATerminal(t *testing.T) {
	env, _ := scratch(t)
	env.ReadSecret = nil
	err := runSetup(t.Context(), env, nil)
	if err == nil {
		t.Fatal("setup ran with no way to read a secret")
	}
	if !strings.Contains(err.Error(), "by hand") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
}

// north-star's one command: `shanty` on a machine with no configuration asks
// rather than sending the user to a document.
func TestAFirstRunAsksInsteadOfRefusing(t *testing.T) {
	srv := fake.New(t, fake.Options{User: user, Password: pass})
	env, shown := answering(t, srv.URL+"\n"+user+"\n", "", pass)

	cfg, creds, err := loadOrSetUp(t.Context(), env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server != srv.URL {
		t.Errorf("the configuration came back as %+v", cfg)
	}
	if mode, ok := creds.Mode(); !ok || mode != config.AuthToken {
		t.Errorf("the credential came back as %q", mode)
	}
	if !strings.Contains(shown.String(), "shanty setup") {
		t.Errorf("a first run did not run setup:\n%s", shown)
	}
}

// A configured machine is not asked again.
func TestAConfiguredRunAsksNothing(t *testing.T) {
	srv := fake.New(t, fake.Options{APIKey: "k-1"})
	env, shown := answering(t, "", "")
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})

	if _, _, err := loadOrSetUp(t.Context(), env); err != nil {
		t.Fatal(err)
	}
	if shown.Len() != 0 {
		t.Errorf("a configured run printed a prompt:\n%s", shown)
	}
}
