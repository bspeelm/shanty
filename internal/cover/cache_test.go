package cover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnImageComesBackAsItWentIn(t *testing.T) {
	c := NewCache(t.TempDir())
	want := []byte("\x89PNG\r\n\x1a\n and then some bytes")

	if err := c.Put("mf-al-1", 300, want); err != nil {
		t.Fatal(err)
	}
	got, found := c.Get("mf-al-1", 300)
	if !found {
		t.Fatal("what was just written was not found")
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNothingKeptIsAMissRatherThanAnError(t *testing.T) {
	c := NewCache(t.TempDir())
	if _, found := c.Get("mf-al-1", 300); found {
		t.Error("an empty cache reported a hit")
	}
}

// TestADirectoryThatCannotBeReadIsAMiss covers the case that decides whether
// this can be called without a guard at every site: a cache that is broken has
// to behave like a cache that is empty.
func TestADirectoryThatCannotBeReadIsAMiss(t *testing.T) {
	for _, c := range []Cache{NewCache(""), NewCache("/does/not/exist/at/all")} {
		if _, found := c.Get("mf-al-1", 300); found {
			t.Errorf("%q reported a hit", c.Dir())
		}
	}
}

// TestTheSameImageAtTwoSizesIsTwoFiles covers the key. A thumbnail served for
// a full-size request is worse than a miss, because nothing later notices.
func TestTheSameImageAtTwoSizesIsTwoFiles(t *testing.T) {
	c := NewCache(t.TempDir())
	if err := c.Put("mf-al-1", 100, []byte("small")); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("mf-al-1", 600, []byte("large")); err != nil {
		t.Fatal(err)
	}
	small, _ := c.Get("mf-al-1", 100)
	large, _ := c.Get("mf-al-1", 600)
	if string(small) != "small" || string(large) != "large" {
		t.Errorf("the two sizes gave %q and %q", small, large)
	}
}

// TestAnIdentifierThatIsAPathStaysInTheDirectory is the one the threat model
// asks for. The identifier is the server's, and it is the first server value
// that could become a filename.
func TestAnIdentifierThatIsAPathStaysInTheDirectory(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	c := NewCache(filepath.Join(dir, "art"))

	hostile := []string{
		"../../outside/escaped",
		"/etc/shanty-should-not-write-here",
		"..",
		".",
		"a/b/c",
		strings.Repeat("n", 4096),
	}
	for _, id := range hostile {
		if err := c.Put(id, 300, []byte("payload")); err != nil {
			t.Errorf("Put(%q) failed, and a hostile id should be kept safely rather than refused: %v", id, err)
			continue
		}
		got, found := c.Get(id, 300)
		if !found || string(got) != "payload" {
			t.Errorf("Put(%q) then Get did not round trip", id)
		}
	}

	// Every file written is directly in the cache directory, named by nothing
	// the server chose.
	entries, err := os.ReadDir(c.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(hostile) {
		t.Errorf("%d files were written for %d identifiers", len(entries), len(hostile))
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("%s is a directory; the cache is one level deep", e.Name())
		}
		if len(e.Name()) != 64 || strings.ContainsAny(e.Name(), "/.") {
			t.Errorf("%q is not a hash of the identifier", e.Name())
		}
	}

	// And nothing reached the directory beside it.
	if got, err := os.ReadDir(outside); err != nil || len(got) != 0 {
		t.Errorf("the cache wrote outside its own directory: %v %v", got, err)
	}
}

// TestKeepingAnImageTwiceReplacesItAndLeavesNothingBeside covers what the
// temporary file does that a test can see: it is renamed over the image and
// never left in the directory.
//
// The write is also atomic, and that part is not covered here. The difference
// between a rename and a truncating write shows only when a process dies part
// way through one, which needs a seam to fail on that this does not have.
func TestKeepingAnImageTwiceReplacesItAndLeavesNothingBeside(t *testing.T) {
	c := NewCache(t.TempDir())
	if err := c.Put("mf-al-1", 300, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := c.Put("mf-al-1", 300, []byte("second")); err != nil {
		t.Fatal(err)
	}

	got, found := c.Get("mf-al-1", 300)
	if !found || string(got) != "second" {
		t.Errorf("the second image did not replace the first: %q", got)
	}
	entries, err := os.ReadDir(c.Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "art-") {
			t.Errorf("%s was left behind", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("%d files after keeping one image twice", len(entries))
	}
}

// TestTheImagesAreNotReadableByOtherAccounts covers §8: what is in the cache
// says what somebody listens to.
func TestTheImagesAreNotReadableByOtherAccounts(t *testing.T) {
	c := NewCache(filepath.Join(t.TempDir(), "art"))
	if err := c.Put("mf-al-1", 300, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	dir, err := os.Stat(c.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Errorf("the cache directory is %v", dir.Mode().Perm())
	}
	file, err := os.Stat(filepath.Join(c.Dir(), name("mf-al-1", 300)))
	if err != nil {
		t.Fatal(err)
	}
	if file.Mode().Perm() != 0o600 {
		t.Errorf("the image is %v", file.Mode().Perm())
	}
}
