package main

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/mpv"
)

// loadOrSetUp reads the configuration, or asks for one and writes it.
//
// north-star measures a first run in minutes and calls it one command. Sending
// a new user away to write two TOML files by hand is not one command, and the
// file they would most obviously write by hand holds a plaintext password --
// the credential ADR-001 ranks last. Doing it here means shanty derives a
// token instead and the password never reaches disk.
func loadOrSetUp(ctx context.Context, env Env) (config.Config, config.Credentials, error) {
	cfg, err := config.LoadConfig(env.Paths.ConfigFile())
	if err != nil {
		return cfg, config.Credentials{}, err
	}
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		// A file that exists and cannot be read is a problem to report, not a
		// reason to start asking questions over the top of it.
		return cfg, creds, err
	}

	_, haveCredential := creds.Mode()
	if cfg.Validate() == nil && haveCredential {
		return cfg, creds, nil
	}
	if env.ReadSecret == nil {
		return cfg, creds, fmt.Errorf("shanty is not configured, and there is no terminal to ask at\n\nRun `shanty setup`, or write %s and %s by hand",
			env.Paths.ConfigFile(), env.Paths.CredentialsFile())
	}

	if err := runSetup(ctx, env, nil); err != nil {
		return cfg, creds, err
	}
	if cfg, err = config.LoadConfig(env.Paths.ConfigFile()); err != nil {
		return cfg, creds, err
	}
	creds, err = config.LoadCredentials(env.Paths.CredentialsFile())
	return cfg, creds, err
}

// play is `shanty` with no arguments: the whole program.
//
// It refuses to start rather than starting broken. A music player that opens
// to an empty screen and an error somewhere in it has spent the user's
// attention to tell them what doctor would have said in one line.
func play(ctx context.Context, env Env) error {
	cfg, creds, err := loadOrSetUp(ctx, env)
	if err != nil {
		return err
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
