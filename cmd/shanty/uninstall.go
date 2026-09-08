package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// runUninstall deletes the directories in Paths.All and prints which were
// removed and which were absent.
func runUninstall(_ context.Context, env Env, _ []string) error {
	// The runtime directory holds the socket a session is reached through.
	// Deleting it under a session that is playing would leave a player nothing
	// could stop.
	if playing(env) {
		return errors.New("a session is playing\n\nRun `shanty stop` first, then `shanty uninstall`")
	}

	var removed, absent []string
	for _, dir := range env.Paths.All() {
		switch _, err := os.Stat(dir); {
		case errors.Is(err, os.ErrNotExist):
			absent = append(absent, dir)
			continue
		case err != nil:
			return err
		}
		// A directory that is a symbolic link is reported rather than deleted.
		if err := refuseSymlink(dir); err != nil {
			return err
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		removed = append(removed, dir)
	}

	for _, dir := range removed {
		fmt.Fprintln(env.Stdout, "removed", dir)
	}
	for _, dir := range absent {
		fmt.Fprintln(env.Stdout, "not there", dir)
	}
	if len(removed) == 0 {
		fmt.Fprintln(env.Stdout, "\nnothing to remove; shanty had left nothing behind")
		return nil
	}
	fmt.Fprintln(env.Stdout, "\nThat is everything shanty writes (§8). Your music is on the server.")
	return nil
}

// refuseSymlink returns an error if dir is a symbolic link.
func refuseSymlink(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := filepath.EvalSymlinks(dir)
		return fmt.Errorf("%s is a symlink to %s; remove it by hand rather than letting shanty delete through it", dir, target)
	}
	return nil
}
