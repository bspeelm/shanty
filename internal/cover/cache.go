// Package cover keeps cover art on disk and turns it into something a terminal
// can show.
//
// Nothing here is reached from internal/tui. The interface is handed art that
// is already bytes or already an escape sequence, which is what keeps rule 1
// true while cover art is a file on disk and a network request behind it.
package cover

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

// Cache is a directory of cover images.
//
// It has no eviction. The directory holds derived data, deleting it costs one
// download each, and `shanty uninstall` removes it with the other three.
type Cache struct{ dir string }

// NewCache returns a cache in a directory. The directory is created by the
// first write, not by this.
func NewCache(dir string) Cache { return Cache{dir: dir} }

// Dir is where the cache keeps its files.
func (c Cache) Dir() string { return c.dir }

// name is the file an image is kept under.
//
// The server chooses the identifier and it is opaque: it may hold path
// separators, may be `..`, and may be long enough that no filesystem will take
// it. It is hashed rather than escaped, so there is no value a server can
// return that names a file outside the directory, and no filename that depends
// on the server behaving.
//
// The size is part of the key because the same image at two sizes is two
// different files.
func name(id string, size int) string {
	sum := sha256.Sum256([]byte(strconv.Itoa(size) + "\x00" + id))
	return hex.EncodeToString(sum[:])
}

// Get returns the image kept for an identifier, and whether there was one.
//
// A cache that cannot be read is a miss rather than an error. The caller's
// answer to both is the same: ask the server.
func (c Cache) Get(id string, size int) ([]byte, bool) {
	if c.dir == "" || id == "" {
		return nil, false
	}
	data, err := os.ReadFile(filepath.Join(c.dir, name(id, size)))
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

// Put keeps an image, creating the directory if it is not there.
func (c Cache) Put(id string, size int, data []byte) error {
	if c.dir == "" {
		return errors.New("the cache has no directory")
	}
	if id == "" {
		return errors.New("no cover art id was given")
	}
	if len(data) == 0 {
		return errors.New("there are no bytes to keep")
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return err
	}

	// Written beside the file and moved over it, so that a process stopped
	// part way through leaves no half image for the next run to read.
	full := filepath.Join(c.dir, name(id, size))
	tmp, err := os.CreateTemp(c.dir, "art-")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), full); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
