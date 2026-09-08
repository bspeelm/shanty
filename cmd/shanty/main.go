// Command shanty is a terminal client for Navidrome and other Subsonic
// servers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"

	"github.com/bspeelm/shanty/internal/config"
)

// Version is stamped by the release build. A development binary says so
// rather than claiming a number nobody tagged.
var Version = "dev"

// Env is everything the commands need from the world, in one struct so a test
// can hand them a different world without a server, a player or a $HOME.
type Env struct {
	Paths  config.Paths
	Stdout io.Writer
	Stderr io.Writer

	// LookPath and Command are how mpv is found and asked its version. They
	// are fields so doctor can be tested where mpv is not installed, which is
	// every machine this was written on.
	LookPath func(string) (string, error)
	Command  func(ctx context.Context, name string, args ...string) ([]byte, error)

	// ImmutableHost says the root filesystem is managed by rpm-ostree, where
	// `dnf install` cannot write. A fix line that names the wrong command is
	// not a fix line.
	ImmutableHost bool
}

// command is one subcommand. The set is closed: docs_test holds it against the
// README in both directions, so a command that exists is documented and a
// command that is documented exists.
type command struct {
	name    string
	summary string
	run     func(ctx context.Context, env Env, args []string) error
}

// A function rather than a variable: help lists the commands, so a slice would
// refer to a function that refers back to the slice.
func commands() []command {
	return []command{
		{"doctor", "check the setup and say what to fix", runDoctor},
		{"uninstall", "remove every directory shanty made, and say which", runUninstall},
		{"version", "print the version", runVersion},
		{"help", "print this", runHelp},
	}
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
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		LookPath:      exec.LookPath,
		ImmutableHost: immutableHost(),
		Command: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}

	if err := run(ctx, env, os.Args[1:]); err != nil {
		// A cancelled context is the user pressing ctrl-c, which is not a
		// failure and should not print like one.
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
	for _, c := range commands() {
		if c.name == args[0] {
			return c.run(ctx, env, args[1:])
		}
	}
	return fmt.Errorf("no such command: %s\nRun `shanty help` for the list", args[0])
}

// immutableHost reports an rpm-ostree root. Inside a toolbox this is false and
// correctly so: the container has a writable /usr and dnf is the right answer
// there -- which is also the only place mpv can live, because shanty and mpv
// have to share a mount namespace to share a socket.
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
