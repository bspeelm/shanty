// Command shanty is a terminal client for Subsonic and Navidrome servers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/charmbracelet/x/term"

	"github.com/bspeelm/shanty/internal/config"
)

// Version is set by the release build. A development build reports "dev".
var Version = "dev"

// Env is what the commands need from the environment, gathered so that tests
// can supply their own.
type Env struct {
	Paths  config.Paths
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	// ReadSecret prompts for a value without echoing it. Nil means there is no
	// terminal to prompt at.
	ReadSecret func(prompt string) (string, error)

	// Executable is the path of the running binary, which a session is started
	// from.
	Executable func() (string, error)

	// LookPath and Command locate mpv and ask its version.
	LookPath func(string) (string, error)
	Command  func(ctx context.Context, name string, args ...string) ([]byte, error)

	// ImmutableHost reports that the root filesystem is managed by rpm-ostree,
	// where dnf cannot install packages.
	ImmutableHost bool
}

// command is one subcommand. The set is closed: docs_test checks it against
// the README in both directions.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env Env, args []string) error
}

// A function rather than a variable, so that help can list the commands.
func commands() []command {
	out := []command{
		{"setup", "ask for a server and a credential, and write both files", runSetup},
		{"doctor", "check the setup and say what to fix", runDoctor},
		{"uninstall", "remove every directory shanty made, and say which", runUninstall},
		{"version", "print the version", runVersion},
		{"completions", "install shell completion, or write the script for one shell", runCompletions},
		{"help", "print this", runHelp},
	}
	for _, c := range commanding {
		out = append(out, command{c.name, c.summary, commandSession(c.name, c.verb)})
	}
	return out
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	paths, err := config.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	env := Env{
		Paths:         paths,
		Stdin:         os.Stdin,
		ReadSecret:    readSecret,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Executable:    os.Executable,
		LookPath:      exec.LookPath,
		ImmutableHost: immutableHost(),
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}

	if err := run(ctx, env, os.Args[1:]); err != nil {
		// A cancelled context is an interrupt, not a failure to report.
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(env.Stderr, err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, env Env, args []string) error {
	if len(args) == 0 {
		return play(ctx, env)
	}
	// A session is started by :headless, not typed, so it is not in the table
	// and does not appear in help.
	if args[0] == sessionArg {
		return runSession(ctx, env)
	}
	// The conventional spellings reach the same place as the command.
	switch args[0] {
	case "-h", "--help", "-help":
		return runHelp(ctx, env, nil)
	case "--version":
		return runVersion(ctx, env, nil)
	}
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(ctx, env, args[1:])
		}
	}
	return fmt.Errorf("no such command: %s\nRun `shanty help` for the list", args[0])
}

// readSecret prompts for a value on the terminal without echoing it. It fails
// if standard input is not a terminal.
func readSecret(prompt string) (string, error) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return "", errors.New("a credential can only be typed at a terminal")
	}
	fmt.Fprint(os.Stdout, prompt)
	raw, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stdout)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// immutableHost reports whether the root filesystem is managed by rpm-ostree.
func immutableHost() bool {
	_, err := os.Stat("/run/ostree-booted")
	return err == nil
}

func runVersion(_ context.Context, env Env, _ []string) error {
	_, err := fmt.Fprintln(env.Stdout, "shanty", Version)
	return err
}

func runHelp(_ context.Context, env Env, _ []string) error {
	fmt.Fprint(env.Stdout, "shanty — a music player for your own server\n\n")
	for _, c := range commands() {
		fmt.Fprintf(env.Stdout, "  shanty %-10s %s\n", c.name, c.summary)
	}
	return nil
}
