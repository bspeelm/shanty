package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bspeelm/shanty/internal/config"
	"github.com/bspeelm/shanty/internal/subsonic/fake"
)

// The isolation suite is §8 as a test: four directories, and nothing else,
// ever. It runs the real commands against a scratch $HOME and diffs the tree.
//
// What it does not cover, stated so nobody reads it as covering more than it
// does: the two sockets. Creating them needs mpv, which is not on every machine
// this is built on. That they land in a 0700 directory under Runtime is
// internal/mpv's TestTheSocketDirectoryIs0700 and TestChildProcessHygiene, and
// internal/control's tests; the integration job is where they meet.

// decoys are the things §8 promises shanty never touches. They are put in the
// scratch home before anything runs, and read back afterwards -- an assertion
// on the write set alone would pass a program that overwrote a file that was
// already there.
var decoys = map[string]string{
	".bashrc":                 "# the user's shell, not shanty's\n",
	".config/mpv/mpv.conf":    "# the user's mpv, which shanty launches with --no-config\n",
	".config/shanty-adjacent": "# a name that starts the same way\n",
	".local/state/other/x":    "# another program's state\n",
}

type tree map[string]string

// snapshot records every file under root with its contents and mode.
func snapshot(t *testing.T, root string) tree {
	t.Helper()
	out := tree{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if d.IsDir() {
			out[rel+"/"] = info.Mode().Perm().String()
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[rel] = info.Mode().Perm().String() + " " + string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func added(before, after tree) []string {
	var out []string
	for path := range after {
		if _, existed := before[path]; !existed {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func changed(before, after tree) []string {
	var out []string
	for path, was := range before {
		if now, still := after[path]; !still || now != was {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// isolated builds a scratch home with the decoys in it, and an Env pointed
// entirely inside it.
func isolated(t *testing.T) (Env, string) {
	t.Helper()
	env, _ := scratch(t)
	home := os.Getenv("HOME")

	for name, body := range decoys {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return env, home
}

// permitted reports whether a path inside the scratch home is one of the four
// directories, or inside one.
func permitted(t *testing.T, env Env, home, rel string) bool {
	t.Helper()
	abs := filepath.Join(home, strings.TrimSuffix(rel, "/"))
	for _, dir := range env.Paths.All() {
		if abs == dir {
			return true
		}
		if r, err := filepath.Rel(dir, abs); err == nil && !strings.HasPrefix(r, "..") {
			return true
		}
		// The parents of a permitted directory are made on the way to it.
		if r, err := filepath.Rel(abs, dir); err == nil && !strings.HasPrefix(r, "..") {
			return true
		}
	}
	return false
}

// A first run writes only inside the four directories. Every command is driven,
// including the ones that should write nothing at all.
func TestIsolationFirstRunWritesOnlyItsOwnDirectories(t *testing.T) {
	env, home := isolated(t)
	srv := fake.New(t, fake.Options{APIKey: "k-1", OpenSubsonic: true})

	before := snapshot(t, home)

	// Everything a first run does: look at the setup, be configured, look
	// again, then be asked what it is.
	if err := runDoctor(t.Context(), env, nil); err == nil {
		t.Fatal("doctor passed on an empty setup")
	}
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	if err := runDoctor(t.Context(), env, []string{"-json"}); err != nil {
		t.Fatalf("doctor failed on a configured setup: %v", err)
	}
	for _, cmd := range []func(context.Context, Env, []string) error{runVersion, runHelp} {
		if err := cmd(t.Context(), env, nil); err != nil {
			t.Fatal(err)
		}
	}

	after := snapshot(t, home)
	for _, rel := range added(before, after) {
		if !permitted(t, env, home, rel) {
			t.Errorf("a first run wrote %s, which is outside the four directories §8 permits", rel)
		}
	}
}

// The decoys are read back byte for byte. §8 says not ~/.config/mpv, not the
// user's shell files, nothing -- and a write-set assertion alone would pass a
// program that overwrote a file already there.
func TestIsolationTouchesNothingOfAnybodyElses(t *testing.T) {
	env, home := isolated(t)
	srv := fake.New(t, fake.Options{APIKey: "k-1"})

	before := snapshot(t, home)
	_ = runDoctor(t.Context(), env, nil)
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	_ = runDoctor(t.Context(), env, nil)

	after := snapshot(t, home)
	for _, rel := range changed(before, after) {
		t.Errorf("something that was already there changed: %s", rel)
	}
	for name := range decoys {
		if after[name] != before[name] {
			t.Errorf("%s was modified", name)
		}
	}
}

// The write set is exactly what §8 names, and the credentials file is 0600
// inside a directory nobody else can list.
func TestIsolationTheWriteSetIsTheOneInTheContract(t *testing.T) {
	env, home := isolated(t)
	srv := fake.New(t, fake.Options{APIKey: "k-1"})

	before := snapshot(t, home)
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	after := snapshot(t, home)

	want := map[string]bool{
		".config/shanty/":                 true,
		".config/shanty/config.toml":      true,
		".config/shanty/credentials.toml": true,
	}
	for _, rel := range added(before, after) {
		if !want[rel] {
			t.Errorf("configuring wrote %s, which the contract does not name", rel)
		}
		delete(want, rel)
	}
	for rel := range want {
		t.Errorf("the contract names %s and it was not written", rel)
	}

	if mode := after[".config/shanty/credentials.toml"]; !strings.HasPrefix(mode, "-rw-------") {
		t.Errorf("the credentials file is %q", strings.SplitN(mode, " ", 2)[0])
	}
	if mode := after[".config/shanty/"]; !strings.HasPrefix(mode, "-rwx------") {
		t.Errorf("the config directory is %q", mode)
	}
}

// Removable without residue: after uninstall the scratch home is byte for byte
// what it was before shanty ran.
func TestIsolationUninstallLeavesNothingBehind(t *testing.T) {
	env, home := isolated(t)
	srv := fake.New(t, fake.Options{APIKey: "k-1"})

	before := snapshot(t, home)

	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	if err := env.Paths.EnsureRuntime(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range env.Paths.All() {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}

	// What may legitimately remain: the standard XDG directories shanty had
	// to create on the way to its own. ~/.cache is not shanty's to delete --
	// another program may be about to use it -- so the promise is that
	// nothing of shanty's remains, and that anything that does is an empty
	// directory that was never ours.
	after := snapshot(t, home)
	for _, rel := range added(before, after) {
		if !strings.HasSuffix(rel, "/") {
			t.Errorf("uninstall left the file %s behind", rel)
			continue
		}
		if strings.Contains(rel, "shanty") {
			t.Errorf("uninstall left %s behind, which is shanty's own", rel)
			continue
		}
		if !empty(t, filepath.Join(home, rel)) {
			t.Errorf("uninstall left %s behind and it is not empty", rel)
		}
	}
	if gone := changed(before, after); len(gone) != 0 {
		t.Errorf("uninstall removed or altered %v, which was not shanty's", gone)
	}
}

// empty reports a directory with nothing in it.
func empty(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries) == 0
}

// TestIsolationOnlyTheInstallerWritesOutsideTheFourDirectories covers the one
// exception in the contract. A shell reads completions from a directory of its
// own, so `shanty completions install` writes there and nothing else does.
func TestIsolationOnlyTheInstallerWritesOutsideTheFourDirectories(t *testing.T) {
	env, home := isolated(t)
	env.LookPath = func(string) (string, error) { return "/usr/bin/anything", nil }
	srv := fake.New(t, fake.Options{APIKey: "k-1"})

	// Everything a person runs in the ordinary way.
	before := snapshot(t, home)
	configure(t, env, config.Config{Server: srv.URL, Username: user}, config.Credentials{APIKey: "k-1"})
	if err := runDoctor(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}
	for _, rel := range added(before, snapshot(t, home)) {
		if strings.Contains(rel, "completion") {
			t.Errorf("an ordinary command wrote %s; only the installer should", rel)
		}
	}

	// The installer, which is the exception.
	before = snapshot(t, home)
	if err := runCompletions(t.Context(), env, []string{"install"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, path := range env.Paths.Completions() {
		rel, err := filepath.Rel(home, path)
		if err != nil {
			t.Fatal(err)
		}
		want[rel] = true
	}
	for _, rel := range added(before, snapshot(t, home)) {
		if strings.HasSuffix(rel, "/") {
			continue // the directories a shell reads from
		}
		if !want[rel] {
			t.Errorf("installing completion wrote %s, which the contract does not name", rel)
		}
		delete(want, rel)
	}
	for rel := range want {
		t.Errorf("the contract names %s and it was not written", rel)
	}

	// And uninstall takes every file back.
	if err := runUninstall(t.Context(), env, nil); err != nil {
		t.Fatal(err)
	}
	for shell, path := range env.Paths.Completions() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("the %s completion survived uninstall", shell)
		}
	}
}
