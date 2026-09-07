package main

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/mpv"
)

// play is `shanty` with no arguments: the whole program.
//
// It refuses to start rather than starting broken. A music player that opens
// to an empty screen and an error somewhere in it has spent the user's
// attention to tell them what doctor would have said in one line.
func play(ctx context.Context, env Env) error {
	cfg, err := config.LoadConfig(env.Paths.ConfigFile())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("%w\n\nEdit %s, then run `shanty doctor`", err, env.Paths.ConfigFile())
	}
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		return err
	}
	if _, ok := creds.Mode(); !ok {
		return fmt.Errorf("no credential is configured in %s\n\nRun `shanty doctor` to see what to put there",
			env.Paths.CredentialsFile())
	}

	client, err := dial(env, cfg, creds)
	if err != nil {
		return err
	}
	if _, err := env.LookPath("mpv"); err != nil {
		return errors.New("mpv is not on PATH, and shanty does not decode audio itself\n\nRun `shanty doctor` for the install command")
	}
	if err := env.Paths.EnsureRuntime(); err != nil {
		return err
	}

	p, err := mpv.Start(ctx, mpv.Options{Socket: env.Paths.Socket()})
	if err != nil {
		return err
	}
	// The player outlives the program only if something here panics, which the
	// panic budget says nothing does.
	defer func() { _ = p.Close() }()

	program := tea.NewProgram(newApp(ctx, client, p),
		tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(env.Stdout))
	_, err = program.Run()
	return err
}
