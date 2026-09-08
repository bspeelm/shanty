package main

import (
	"context"
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/mpv"
)

// loadOrSetUp returns the configuration, running setup first if there is none.
func loadOrSetUp(ctx context.Context, env Env) (config.Config, config.Credentials, error) {
	cfg, err := config.LoadConfig(env.Paths.ConfigFile())
	if err != nil {
		return cfg, config.Credentials{}, err
	}
	creds, err := config.LoadCredentials(env.Paths.CredentialsFile())
	if err != nil {
		// A file that exists but cannot be read is reported, rather than being
		// overwritten by setup.
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

// play runs the interface: it loads the configuration, starts mpv, and hands
// both to the model.
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
	// mpv is stopped when play returns, unless a session has taken it over.
	released := false
	defer func() {
		if released {
			_ = p.Detach()
			return
		}
		_ = p.Close()
	}()

	a := newApp(ctx, client, p)
	a.detach = detacher(env)
	program := tea.NewProgram(a,
		tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(env.Stdout))
	final, err := program.Run()
	if m, ok := final.(app); ok && m.released {
		released = true
		fmt.Fprintln(env.Stdout, "the music is still playing. `shanty` returns to it, `shanty stop` ends it.")
	}
	return err
}
