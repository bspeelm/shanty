package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// runUninstall removes the four directories §8 permits and prints what it
// removed. It reads Paths.All rather than carrying its own list, so a fifth
// directory added anywhere cannot be one uninstall leaves behind.
func runUninstall(_ context.Context, env Env, _ []string) error {
	var removed, absent []string
	for _, dir := range env.Paths.All() {
		switch _, err := os.Stat(dir); {
		case errors.Is(err, os.ErrNotExist):
			absent = append(absent, dir)
			continue
		case err != nil:
			return err
		}
		// Refusing rather than deleting: a directory shanty would remove is
		// one a symlink could point anywhere, and the four paths are
		// well-known enough for that to be worth a check rather than a
		// paragraph in the release notes.
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

// refuseSymlink stops a delete from following a link out of shanty's own tree.
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
