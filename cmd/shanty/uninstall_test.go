package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesEveryDirectoryAndNothingElse(t *testing.T) {
	env, out := scratch(t)
	home := os.Getenv("HOME")

	// Something shanty made, and something that is not shanty's.
	for _, dir := range env.Paths.All() {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "a-file"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bystander := filepath.Join(home, ".config", "not-shanty")
	if err := os.MkdirAll(bystander, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}

	for _, dir := range env.Paths.All() {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", dir)
		}
		if !strings.Contains(out.String(), dir) {
			t.Errorf("uninstall removed %s without saying so", dir)
		}
	}
	if _, err := os.Stat(bystander); err != nil {
		t.Errorf("uninstall removed something that was not shanty's: %v", err)
	}
}

func TestUninstallOnACleanMachineSaysSo(t *testing.T) {
	env, out := scratch(t)
	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "nothing to remove") {
		t.Errorf("uninstall on a clean machine said:\n%s", out)
	}
}

// A directory shanty deletes is one a symlink could point anywhere. Deleting
// through it would take the target with it.
func TestUninstallRefusesToDeleteThroughASymlink(t *testing.T) {
	env, _ := scratch(t)

	elsewhere := t.TempDir()
	keepsake := filepath.Join(elsewhere, "not-yours")
	if err := os.WriteFile(keepsake, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(env.Paths.Cache), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, env.Paths.Cache); err != nil {
		t.Fatal(err)
	}

	err := runUninstall(t.Context(), env, nil)
	if err == nil {
		t.Fatal("uninstall followed a symlink out of its own tree")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("the refusal does not explain itself: %v", err)
	}
	if _, statErr := os.Stat(keepsake); statErr != nil {
		t.Errorf("the symlink's target was deleted anyway: %v", statErr)
	}
}

// uninstall reads Paths.All rather than its own list, so a fifth directory
// added anywhere cannot be one it leaves behind.
func TestUninstallCoversTheWholeWriteSet(t *testing.T) {
	env, _ := scratch(t)
	if len(env.Paths.All()) != 4 {
		t.Fatalf("the write set has %d directories; §8 names four", len(env.Paths.All()))
	}
}
