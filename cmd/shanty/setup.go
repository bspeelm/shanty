package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/subsonic"
)

// runSetup asks for a server, a username and a credential, checks them
// against the server, and writes both configuration files. It writes nothing
// if the server rejects the credential.
func runSetup(ctx context.Context, env Env, _ []string) error {
	if env.ReadSecret == nil || env.Stdin == nil {
		return errors.New("setup needs a terminal; write the two files by hand instead (see the README)")
	}

	fmt.Fprintf(env.Stdout, "shanty setup — two files, in %s\n\n", env.Paths.Config)

	// One reader for the whole conversation. A fresh bufio.Reader per question
	// buffers everything after the first line and then throws it away, which
	// a terminal hides -- the next line has not been typed yet -- and a pipe
	// does not.
	in := bufio.NewReader(env.Stdin)

	server, err := ask(env, in, "Server URL", "https://music.example.org")
	if err != nil {
		return err
	}
	cfg := config.Config{Server: strings.TrimRight(server, "/")}
	if err := cfg.Validate(); err != nil {
		return err
	}
	if cfg.Insecure() {
		fmt.Fprintln(env.Stdout, "\n  note: that is plain HTTP, so the credential crosses the network in clear.")
	}

	if cfg.Username, err = ask(env, in, "Username", ""); err != nil {
		return err
	}

	creds, err := askCredential(env, cfg)
	if err != nil {
		return err
	}

	// Checked before anything is written.
	fmt.Fprint(env.Stdout, "\nchecking… ")
	client, err := dial(env, cfg, creds)
	if err != nil {
		return err
	}
	if err := client.Ping(ctx); err != nil {
		fmt.Fprintln(env.Stdout, "no.")
		return fmt.Errorf("%w\n\nNothing was written. Run `shanty setup` again", err)
	}
	fmt.Fprintln(env.Stdout, "the server accepted it.")

	if err := cfg.Save(env.Paths.ConfigFile()); err != nil {
		return err
	}
	if err := creds.Save(env.Paths.CredentialsFile()); err != nil {
		return err
	}

	mode, _ := creds.Mode()
	fmt.Fprintf(env.Stdout, "\nwrote %s\n", env.Paths.ConfigFile())
	fmt.Fprintf(env.Stdout, "wrote %s (0600, holding the %s)\n", env.Paths.CredentialsFile(), mode)
	if mode == config.AuthToken {
		fmt.Fprintln(env.Stdout, "\nYour password was not saved. shanty stored the hash the Subsonic\nprotocol sends instead, which works against this server and nowhere else.")
		if better, err := client.SupportsAPIKeys(ctx); err == nil && better {
			fmt.Fprintln(env.Stdout, "\nThis server also offers API keys, which are revoked in one click.\nMake one in its web interface and `shanty setup` again to use it.")
		}
	}
	fmt.Fprintln(env.Stdout, "\nnext: shanty")
	return nil
}

// askCredential asks for an API key, then for a password if none was given.
func askCredential(env Env, cfg config.Config) (config.Credentials, error) {
	key, err := env.ReadSecret("API key (press enter to use a password instead): ")
	if err != nil {
		return config.Credentials{}, err
	}
	if key = strings.TrimSpace(key); key != "" {
		return config.Credentials{APIKey: key}, nil
	}

	password, err := env.ReadSecret("Password: ")
	if err != nil {
		return config.Credentials{}, err
	}
	if password == "" {
		return config.Credentials{}, errors.New("no credential was given")
	}

	// The salt is generated once and stored, because a stored token has to keep
	// the salt it was made with.
	salt := subsonic.Salt()
	return config.Credentials{Token: subsonic.Token(password, salt), Salt: salt}, nil
}

func ask(env Env, in *bufio.Reader, prompt, example string) (string, error) {
	if example != "" {
		fmt.Fprintf(env.Stdout, "%s [%s]: ", prompt, example)
	} else {
		fmt.Fprintf(env.Stdout, "%s: ", prompt)
	}
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(prompt))
	}
	return answer, nil
}
