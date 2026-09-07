package config

import (
	"os"
	"path/filepath"
)

// writeAtomic writes data to path at mode, or leaves what was already there
// untouched. A half-written credentials file authenticates against nothing, at
// the moment the user least wants to debug their music player.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	// 0700: this directory holds the credentials file, and one others can list
	// is one whose contents they know to come back for.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// A unique name, not path+".tmp": two shantys started at once would share
	// that one name, and one could rename the other's half-written file into
	// place, defeating the atomicity this exists for.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".shanty-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Unchecked deliberately: after a successful rename there is nothing left
	// to remove, and after a failure the intact original is what matters.
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Set before the rename, so nothing ever observes the file at the wrong
	// permissions -- not even for the instant between two syscalls.
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
