package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRoundTripsAt0644(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	want := Config{Server: "https://music.example.org", Username: "skipper"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != ConfigMode {
		t.Errorf("config mode = %04o, want %04o", got, ConfigMode)
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("round-tripped %+v, want %+v", got, want)
	}
}

func TestAMissingConfigIsAFirstRunNotAFault(t *testing.T) {
	got, err := LoadConfig(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("a missing config errored: %v", err)
	}
	if err := got.Validate(); err == nil {
		t.Error("an empty config validated; nothing would tell a new user what to do")
	}
}

// §5: errors say what happened and what to do. A validation message that names
// the problem without naming the line to type is half an error.
func TestEveryValidationFailureNamesTheFix(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       Config
		contains string
	}{
		{name: "no server", in: Config{}, contains: `server = "https://`},
		{name: "not a URL", in: Config{Server: "://"}, contains: "://"},
		{name: "wrong scheme", in: Config{Server: "ftp://music.example.org"}, contains: "https://"},
		{name: "no host", in: Config{Server: "https://"}, contains: "https://music.example.org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if err == nil {
				t.Fatalf("%+v validated", tc.in)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Errorf("error does not carry %q:\n%s", tc.contains, err)
			}
		})
	}
}

// http is accepted rather than refused: a LAN server and a test container are
// real setups, and refusing them pushes this project's own user onto a client
// that asks fewer questions. It is reported so doctor can say so.
func TestPlainHTTPIsAcceptedAndReported(t *testing.T) {
	lan := Config{Server: "http://nas.local:4533", Username: "skipper"}
	if err := lan.Validate(); err != nil {
		t.Errorf("a plain-HTTP server was refused: %v", err)
	}
	if !lan.Insecure() {
		t.Error("a plain-HTTP server did not report itself insecure, so doctor would stay quiet about it")
	}
	if (Config{Server: "https://music.example.org"}).Insecure() {
		t.Error("an HTTPS server reported itself insecure")
	}
}

func TestBrokenTOMLNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("server = \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want it to name %s", err, path)
	}
}

// The property the atomic write exists for: an interrupted save leaves the
// previous file, not half of the new one. Simulated by writing into a
// directory that cannot be renamed within -- the temp file is created, the
// rename fails, and what was already there must be untouched.
func TestAFailedSaveLeavesThePreviousFileIntact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	first := Config{Server: "https://first.example.org", Username: "skipper"}
	if err := first.Save(path); err != nil {
		t.Fatal(err)
	}

	// A directory in place of the destination file: the rename cannot land.
	blocked := filepath.Join(dir, "blocked.toml")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (Config{Server: "https://second.example.org"}).Save(blocked); err == nil {
		t.Fatal("saving over a directory succeeded")
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Errorf("the untouched file changed to %+v", got)
	}
}

// A failed write leaves no temporary file lying beside the real one. The
// isolation suite diffs the whole write set, so litter here is a test failure
// two packages away with a much less obvious cause.
func TestAFailedSaveLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked.toml")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (Config{Server: "https://example.org"}).Save(blocked); err == nil {
		t.Fatal("saving over a directory succeeded")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".shanty-") {
			t.Errorf("a failed save left %q behind", e.Name())
		}
	}
}

// Two saves at once may not produce a file that is half of each. The unique
// temporary name is what prevents it; a shared "config.toml.tmp" would let one
// writer rename the other's partial file into place.
func TestConcurrentSavesDoNotEatEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	done := make(chan error, 2)
	for _, server := range []string{"https://a.example.org", "https://b.example.org"} {
		go func() { done <- (Config{Server: server, Username: "skipper"}).Save(path) }()
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("the surviving file does not parse: %v", err)
	}
	if got.Server != "https://a.example.org" && got.Server != "https://b.example.org" {
		t.Errorf("the surviving file is neither writer's: %+v", got)
	}
}
