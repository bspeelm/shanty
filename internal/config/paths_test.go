package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratchHome points every directory-resolving variable at a temporary tree,
// so a test can ask what shanty would do without depending on the machine it
// runs on -- and so that a bug in Discover cannot write to the developer's own
// config directory while the suite is proving it does not.
func scratchHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	return home
}

func TestDiscoverUsesTheXDGDefaults(t *testing.T) {
	home := scratchHome(t)

	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ got, want string }{
		{p.Config, filepath.Join(home, ".config", "shanty")},
		{p.State, filepath.Join(home, ".local", "state", "shanty")},
		{p.Cache, filepath.Join(home, ".cache", "shanty")},
	} {
		if tc.got != tc.want {
			t.Errorf("resolved %q, want %q", tc.got, tc.want)
		}
	}
}

func TestDiscoverHonoursTheXDGVariables(t *testing.T) {
	scratchHome(t)
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "c"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "s"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "k"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "r"))

	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ got, want string }{
		{p.Config, filepath.Join(root, "c", "shanty")},
		{p.State, filepath.Join(root, "s", "shanty")},
		{p.Cache, filepath.Join(root, "k", "shanty")},
		{p.Runtime, filepath.Join(root, "r", "shanty")},
	} {
		if tc.got != tc.want {
			t.Errorf("resolved %q, want %q", tc.got, tc.want)
		}
	}
}

// The XDG specification says a relative value is ignored. Resolving one
// against the working directory would scatter shanty's state into whatever
// directory it happened to be started from -- which is both wrong and, for the
// credentials file, a way to write a secret somewhere nobody expects.
func TestARelativeXDGVariableIsRefused(t *testing.T) {
	for _, env := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
		t.Run(env, func(t *testing.T) {
			scratchHome(t)
			t.Setenv(env, "relative/path")
			if _, err := Discover(); err == nil {
				t.Errorf("a relative %s was accepted", env)
			}
		})
	}
}

// §7: never a shared /tmp. The socket name is predictable, so a directory
// another account can write to is one where somebody else's socket can be
// waiting under our name by the time we connect.
func TestTheRuntimeFallbackIsNeverASharedTmp(t *testing.T) {
	home := scratchHome(t)

	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(p.Runtime, home) {
		t.Errorf("with no XDG_RUNTIME_DIR the socket went to %q, outside the user's own tree", p.Runtime)
	}
	// Asserted against the documented fallback rather than against "/tmp",
	// because a test whose own HOME is a temporary directory would otherwise
	// fail on the right behaviour.
	if rel, err := filepath.Rel(p.Cache, p.Runtime); err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("with no XDG_RUNTIME_DIR the socket went to %q, which is not under the cache directory %q", p.Runtime, p.Cache)
	}
	if p.Runtime == os.TempDir() || filepath.Dir(p.Runtime) == os.TempDir() {
		t.Errorf("the socket fell back to the shared temp directory: %q", p.Socket())
	}
}

func TestEnsureRuntimeIs0700(t *testing.T) {
	scratchHome(t)
	root := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", root)

	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}

	// A directory left behind by an older version, or by anything else that
	// got there first, is tightened rather than trusted.
	if err := os.MkdirAll(p.Runtime, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.EnsureRuntime(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(p.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("runtime directory mode = %04o, want 0700", got)
	}
}

// Discover reports where things go; it does not put anything there. A command
// that only reports -- doctor, or --help -- must not leave a directory behind
// as the price of having run.
func TestDiscoverCreatesNothing(t *testing.T) {
	home := scratchHome(t)

	if _, err := Discover(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("Discover created %v", names)
	}
}

// All is the write set the isolation suite and uninstall both read. A fifth
// directory resolved anywhere in this package and left out of All would be one
// neither of them ever looks at.
func TestAllIsEveryDirectoryPathsResolves(t *testing.T) {
	scratchHome(t)
	p, err := Discover()
	if err != nil {
		t.Fatal(err)
	}

	inAll := map[string]bool{}
	for _, d := range p.All() {
		inAll[d] = true
	}
	for name, dir := range map[string]string{
		"Config": p.Config, "State": p.State, "Cache": p.Cache, "Runtime": p.Runtime,
	} {
		if !inAll[dir] {
			t.Errorf("Paths.%s resolves to %q, which All() does not list", name, dir)
		}
	}
	if len(p.All()) != 4 {
		t.Errorf("All() lists %d directories; §8 names four, so this is a change to the filesystem contract", len(p.All()))
	}

	// Every file the package names must live inside one of them.
	for name, file := range map[string]string{
		"ConfigFile": p.ConfigFile(), "CredentialsFile": p.CredentialsFile(), "Socket": p.Socket(),
	} {
		var contained bool
		for _, dir := range p.All() {
			if rel, err := filepath.Rel(dir, file); err == nil && !strings.HasPrefix(rel, "..") {
				contained = true
			}
		}
		if !contained {
			t.Errorf("%s resolves to %q, outside every directory in All()", name, file)
		}
	}
}
