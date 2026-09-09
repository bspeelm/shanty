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

// TestCompletionsGoWhereEachShellLooks covers the files the installer writes
// beside the binary. They are not shanty's own data: a shell reads completions
// from a directory of its own, so a file kept with the four directories would
// never be read.
func TestCompletionsGoWhereEachShellLooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}

	paths, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	got := paths.Completions()

	want := map[string]string{
		"bash": filepath.Join(home, ".local", "share", "bash-completion", "completions", "shanty"),
		"fish": filepath.Join(home, ".config", "fish", "completions", "shanty.fish"),
	}
	for shell, path := range want {
		if got[shell] != path {
			t.Errorf("%s completion goes to %q, want %q", shell, got[shell], path)
		}
	}
	if len(got) != len(want) {
		t.Errorf("there are %d completion files and %d were checked", len(got), len(want))
	}

	// None of them is inside a directory shanty writes to, because uninstall
	// removes those wholesale and these belong to the shell.
	for shell, path := range got {
		for _, dir := range paths.All() {
			if strings.HasPrefix(path, dir+string(filepath.Separator)) {
				t.Errorf("the %s completion is inside %s", shell, dir)
			}
		}
	}
}

// TestCompletionsFollowTheEnvironment covers a machine that puts its XDG
// directories somewhere other than the default.
func TestCompletionsFollowTheEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "dat"))

	paths, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	got := paths.Completions()

	if want := filepath.Join(home, "dat", "bash-completion", "completions", "shanty"); got["bash"] != want {
		t.Errorf("bash completion goes to %q, want %q", got["bash"], want)
	}
	if want := filepath.Join(home, "cfg", "fish", "completions", "shanty.fish"); got["fish"] != want {
		t.Errorf("fish completion goes to %q, want %q", got["fish"], want)
	}
}

// TestAPathTooLongForASocketIsNamedAsSuch covers the limit the kernel enforces
// without explaining. A socket path over it fails to bind with "invalid
// argument", which says nothing about length.
func TestAPathTooLongForASocketIsNamedAsSuch(t *testing.T) {
	for _, tc := range []struct {
		length int
		over   int
		long   bool
	}{
		{10, 0, false},
		{SocketLimit - 1, 0, false},
		{SocketLimit, 0, false},
		{SocketLimit + 1, 1, true},
		{SocketLimit + 40, 40, true},
	} {
		over, long := TooLongForASocket(strings.Repeat("a", tc.length))
		if long != tc.long {
			t.Errorf("a path of %d characters: too long = %v, want %v", tc.length, long, tc.long)
		}
		if over != tc.over {
			t.Errorf("a path of %d characters is %d over, want %d", tc.length, over, tc.over)
		}
	}
}

// TestTheLimitIsTheShorterOfTheTwoKernels covers the number. Linux allows 108
// bytes and macOS 104, both including the end of the string, so the shorter is
// the one to hold to.
func TestTheLimitIsTheShorterOfTheTwoKernels(t *testing.T) {
	if SocketLimit != 103 {
		t.Errorf("the limit is %d; macOS allows 104 bytes including the terminator", SocketLimit)
	}
}

// TestCoversAreInsideTheCacheDirectory holds §8: cover art is the first thing
// written to the cache, and it has to be written inside one of the four
// directories rather than beside them.
func TestCoversAreInsideTheCacheDirectory(t *testing.T) {
	p := Paths{Config: "/c", State: "/s", Cache: "/k", Runtime: "/r"}

	covers := p.Covers()
	if !strings.HasPrefix(covers, p.Cache+string(filepath.Separator)) {
		t.Errorf("covers go to %q, which is not inside the cache directory %q", covers, p.Cache)
	}
	// Removing the four directories removes the covers with them, which is
	// what lets `shanty uninstall` say it left nothing.
	var covered bool
	for _, dir := range p.All() {
		if strings.HasPrefix(covers, dir+string(filepath.Separator)) {
			covered = true
		}
	}
	if !covered {
		t.Errorf("%q is in none of the four directories, so uninstall would leave it", covers)
	}
}
