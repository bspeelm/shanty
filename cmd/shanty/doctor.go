package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/mpv"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// Severity is how serious a check result is.
type Severity string

const (
	Fail Severity = "fail"
	Warn Severity = "warn"
	Pass Severity = "pass"
	Skip Severity = "skip"
)

// Result is one check’s outcome. Every Fail and Warn carries a Fix.
type Result struct {
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Detail   string   `json:"detail,omitempty"`
	Fix      string   `json:"fix,omitempty"`
}

// Report is every check, in the order they were run: the local setup first,
// then the server.
type Report struct {
	Results []Result `json:"results"`
}

// OK reports whether every check passed. Warnings do not fail a report.
func (r Report) OK() bool {
	for _, res := range r.Results {
		if res.Severity == Fail {
			return false
		}
	}
	return true
}

// checkIDs is every check the report contains. A test holds this list and the
// report to each other.
var checkIDs = []string{
	"mpv", "mpv-version", "runtime-dir", "session", "config", "credentials",
	"server", "auth", "auth-mode",
}

func runDoctor(ctx context.Context, env Env, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}

	report := diagnose(ctx, env)
	if *asJSON {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		printReport(env, report)
	}
	if !report.OK() {
		return errors.New("doctor found something broken")
	}
	return nil
}

func printReport(env Env, r Report) {
	marks := map[Severity]string{Fail: "✗", Warn: "!", Pass: "✓", Skip: "–"}
	for _, res := range r.Results {
		fmt.Fprintf(env.Stdout, "%s %-14s %s\n", marks[res.Severity], res.ID, res.Summary)
		if res.Detail != "" {
			fmt.Fprintf(env.Stdout, "  %s\n", res.Detail)
		}
		if res.Fix != "" {
			fmt.Fprintf(env.Stdout, "  fix: %s\n", res.Fix)
		}
	}
}

// diagnose runs every check. Checks that an earlier failure has made
// unanswerable are skipped rather than failed.
func diagnose(ctx context.Context, env Env) Report {
	var out []Result
	add := func(r Result) Result { out = append(out, r); return r }

	mpv := add(checkMpv(env))
	add(checkMpvVersion(ctx, env, mpv))
	add(checkRuntimeDir(env))
	add(checkSession(env))

	cfg, cfgResult := checkConfig(env)
	add(cfgResult)
	creds, credResult := checkCredentials(env)
	add(credResult)

	if cfgResult.Severity == Fail || credResult.Severity == Fail {
		out = append(out,
			skipped("server", "not checked; the configuration is not usable yet"),
			skipped("auth", "not checked; the configuration is not usable yet"),
			skipped("auth-mode", "not checked; the configuration is not usable yet"))
		return Report{Results: out}
	}

	client, err := dial(env, cfg, creds)
	if err != nil {
		out = append(out,
			Result{ID: "server", Severity: Fail, Summary: "cannot build a client for this server",
				Detail: err.Error(), Fix: "check the server line in " + env.Paths.ConfigFile()},
			skipped("auth", "not checked; there is no client to ask with"),
			skipped("auth-mode", "not checked; there is no client to ask with"))
		return Report{Results: out}
	}

	server := add(checkServer(ctx, env, cfg, client))
	if server.Severity == Fail {
		out = append(out,
			skipped("auth", "not checked; the server did not answer"),
			skipped("auth-mode", "not checked; the server did not answer"))
		return Report{Results: out}
	}
	add(checkAuth(ctx, env, client))
	add(checkAuthMode(ctx, creds, client))

	return Report{Results: out}
}

func skipped(id, why string) Result {
	return Result{ID: id, Severity: Skip, Summary: why}
}

func checkMpv(env Env) Result {
	path, err := env.LookPath("mpv")
	if err != nil {
		fix := "install it: apt install mpv · dnf install mpv · brew install mpv · pacman -S mpv"
		if env.ImmutableHost {
			// dnf cannot write /usr here, and a flatpak mpv is worse than
			// none: its sandbox remaps XDG_RUNTIME_DIR, so the socket shanty
			// creates is not visible at that path inside it.
			fix = "rpm-ostree install mpv, then reboot. A flatpak mpv will not work: shanty and mpv share a unix socket, so they must share a mount namespace."
		}
		return Result{ID: "mpv", Severity: Fail,
			Summary: "mpv is not on PATH",
			Detail:  "shanty does not decode audio itself; mpv does the playing (ADR-011)",
			Fix:     fix}
	}
	return Result{ID: "mpv", Severity: Pass, Summary: "mpv is at " + path}
}

// checkMpvVersion reports mpv’s version and whether it is old enough to be
// missing a flag shanty always passes.
func checkMpvVersion(ctx context.Context, env Env, found Result) Result {
	if found.Severity == Fail {
		return skipped("mpv-version", "not checked; mpv was not found")
	}
	out, err := env.Command(ctx, "mpv", "--version")
	if err != nil {
		return Result{ID: "mpv-version", Severity: Warn,
			Summary: "mpv would not report its version",
			Detail:  err.Error(),
			Fix:     "run `mpv --version` by hand and see what it says"}
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	version, known := mpv.Version(string(out))
	if !known {
		return Result{ID: "mpv-version", Severity: Warn,
			Summary: "mpv did not say which version it is",
			Detail:  line,
			Fix:     "shanty needs " + mpv.Minimum + " or newer; check with `mpv --version`"}
	}
	if mpv.OlderThan(version, mpv.Minimum) {
		return Result{ID: "mpv-version", Severity: Fail,
			Summary: "mpv " + version + " is too old",
			Detail:  "shanty starts mpv with --prefetch-playlist, which arrived in " + mpv.Minimum,
			Fix:     "upgrade mpv to " + mpv.Minimum + " or newer"}
	}
	return Result{ID: "mpv-version", Severity: Pass, Summary: line}
}

// checkRuntimeDir reports on the runtime directory without creating it.
func checkRuntimeDir(env Env) Result {
	info, err := os.Stat(env.Paths.Runtime)
	if errors.Is(err, os.ErrNotExist) {
		return Result{ID: "runtime-dir", Severity: Pass,
			Summary: env.Paths.Runtime + " will be made at 0700 when a track first plays"}
	}
	if err != nil {
		return Result{ID: "runtime-dir", Severity: Fail, Summary: err.Error(),
			Fix: "remove or fix " + env.Paths.Runtime}
	}
	if !info.IsDir() {
		return Result{ID: "runtime-dir", Severity: Fail,
			Summary: env.Paths.Runtime + " is not a directory",
			Fix:     "rm " + env.Paths.Runtime}
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		return Result{ID: "runtime-dir", Severity: Fail,
			Summary: fmt.Sprintf("the runtime directory is mode %04o", perm),
			Detail:  "the mpv socket lives here, and it reaches a player holding a credential",
			Fix:     "chmod 700 " + env.Paths.Runtime}
	}
	// A socket path over the kernel's limit fails to bind with a message that
	// says nothing about length, so it is named here before anything opens one.
	for _, path := range []string{env.Paths.Socket(), env.Paths.ControlSocket()} {
		if over, tooLong := config.TooLongForASocket(path); tooLong {
			return Result{ID: "runtime-dir", Severity: Fail,
				Summary: fmt.Sprintf("the path for a socket here is %d over the %d a socket may have", over, config.SocketLimit),
				Detail:  path,
				Fix:     "set XDG_RUNTIME_DIR to something shorter"}
		}
	}
	return Result{ID: "runtime-dir", Severity: Pass, Summary: env.Paths.Runtime + " is 0700"}
}

// checkSession reports on a session playing in the background. A process
// nobody can see is the one thing here that cannot report itself, so this is
// where it is named.
func checkSession(env Env) Result {
	session := playing(env)
	player := listening(env.Paths.Socket())

	switch {
	case session && player:
		return Result{ID: "session", Severity: Pass,
			Summary: "a session is playing in the background",
			Detail:  "`shanty` returns the interface to it, `shanty stop` ends it"}

	case session && !player:
		return Result{ID: "session", Severity: Warn,
			Summary: "a session is running and its player has stopped",
			Detail:  "it has nothing left to play and nothing to say so",
			Fix:     "shanty stop"}

	case !session && player:
		return Result{ID: "session", Severity: Warn,
			Summary: "an mpv is running that no session owns",
			Detail:  "it was left by a session that was killed, and it holds a stream URL",
			Fix:     "shanty stop"}
	}
	return Result{ID: "session", Severity: Pass, Summary: "nothing is playing in the background"}
}

func checkConfig(env Env) (config.Config, Result) {
	cfg, err := config.LoadConfig(env.Paths.ConfigFile())
	if err != nil {
		return cfg, Result{ID: "config", Severity: Fail,
			Summary: "the configuration will not parse",
			Detail:  err.Error(),
			Fix:     "edit " + env.Paths.ConfigFile()}
	}
	if err := cfg.Validate(); err != nil {
		return cfg, Result{ID: "config", Severity: Fail,
			Summary: "the configuration is not usable",
			Detail:  err.Error(),
			Fix:     "edit " + env.Paths.ConfigFile()}
	}
	if cfg.Insecure() {
		return cfg, Result{ID: "config", Severity: Warn,
			Summary: "the server is plain HTTP",
			Detail:  "the credential and everything played crosses the network in clear",
			Fix:     "use https:// in " + env.Paths.ConfigFile() + " if the server offers it"}
	}
	return cfg, Result{ID: "config", Severity: Pass, Summary: cfg.Server}
}

func checkCredentials(env Env) (config.Credentials, Result) {
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		var perr *config.PermissionError
		if errors.As(err, &perr) {
			// The message already contains the chmod command; it is split so the command
			// appears on the fix line.
			summary, fix, _ := strings.Cut(perr.Error(), "\n")
			return creds, Result{ID: "credentials", Severity: Fail,
				Summary: "the credentials file is readable by other accounts",
				Detail:  summary, Fix: strings.TrimPrefix(fix, "Fix it with:  ")}
		}
		return creds, Result{ID: "credentials", Severity: Fail,
			Summary: "the credentials will not load", Detail: err.Error(),
			Fix: "edit " + env.Paths.CredentialsFile()}
	}
	mode, ok := creds.Mode()
	if !ok {
		return creds, Result{ID: "credentials", Severity: Fail,
			Summary: "no credential is configured",
			Fix:     "put an api_key, or a token and salt, in " + env.Paths.CredentialsFile()}
	}
	if mode == config.AuthPassword {
		return creds, Result{ID: "credentials", Severity: Warn,
			Summary: "the credential is a plaintext password",
			Detail:  "a stolen config yields whatever else that password opens (ADR-001)",
			Fix:     "replace it with an api_key, or with password_file pointing at a secret you manage"}
	}
	return creds, Result{ID: "credentials", Severity: Pass, Summary: "using the " + string(mode)}
}

func checkServer(ctx context.Context, env Env, cfg config.Config, client *subsonic.Client) Result {
	if err := client.Ping(ctx); err != nil {
		var serr *subsonic.Error
		if errors.As(err, &serr) && serr.Unauthorized() {
			// The server answered. A rejected credential is the auth check’s result, not
			// this one’s.
			return Result{ID: "server", Severity: Pass, Summary: cfg.Server + " answered"}
		}
		return Result{ID: "server", Severity: Fail,
			Summary: "the server did not answer",
			Detail:  err.Error(),
			Fix:     "check that " + cfg.Server + " is reachable from here"}
	}
	return Result{ID: "server", Severity: Pass, Summary: cfg.Server + " answered"}
}

func checkAuth(ctx context.Context, env Env, client *subsonic.Client) Result {
	if err := client.Ping(ctx); err != nil {
		return Result{ID: "auth", Severity: Fail,
			Summary: "the server refused the credential",
			Detail:  err.Error(),
			Fix:     "check the credential in " + env.Paths.CredentialsFile() + " and the username in " + env.Paths.ConfigFile()}
	}
	return Result{ID: "auth", Severity: Pass, Summary: "the credential works"}
}

// checkAuthMode reports whether the server supports a stronger credential
// than the one in use.
func checkAuthMode(ctx context.Context, creds config.Credentials, client *subsonic.Client) Result {
	mode, _ := creds.Mode()
	if mode == config.AuthAPIKey {
		return Result{ID: "auth-mode", Severity: Pass, Summary: "already using the strongest credential"}
	}
	better, err := client.SupportsAPIKeys(ctx)
	if err != nil {
		return Result{ID: "auth-mode", Severity: Skip, Summary: "the server did not say what it supports"}
	}
	if !better {
		return Result{ID: "auth-mode", Severity: Pass,
			Summary: "this server does not offer API keys, so " + string(mode) + " is the best available"}
	}
	return Result{ID: "auth-mode", Severity: Warn,
		Summary: "this server offers API keys and shanty is not using one",
		Detail:  "an API key is revoked server-side in one click; a token is not (ADR-001)",
		Fix:     "create one in the server's web interface and put it in credentials.toml as api_key"}
}

// dial returns a client authenticated the same way playback will be.
func dial(env Env, cfg config.Config, creds config.Credentials) (*subsonic.Client, error) {
	auth, err := authenticator(cfg, creds)
	if err != nil {
		return nil, err
	}
	return subsonic.New(cfg.Server, auth, subsonic.Options{
		UserAgent: "shanty/" + Version,
	})
}

func authenticator(cfg config.Config, creds config.Credentials) (subsonic.Authenticator, error) {
	mode, ok := creds.Mode()
	if !ok {
		return nil, errors.New("no credential is configured")
	}
	switch mode {
	case config.AuthAPIKey:
		return subsonic.APIKeyAuth(creds.APIKey), nil
	case config.AuthToken:
		return subsonic.TokenAuth(cfg.Username, creds.Token, creds.Salt), nil
	default:
		password, err := creds.ResolvePassword()
		if err != nil {
			return nil, err
		}
		return subsonic.PasswordAuth(cfg.Username, password), nil
	}
}
