package config

import (
	"os"
	"path/filepath"
)

// writeAtomic writes data to path at mode, or leaves what was already there
// untouched. A half-written credentials file is not a corrupt file: it is a
// file that authenticates against nothing, at the moment the user least wants
// to debug their music player.
//
// Lifted from bothy's stageExecutable, including the reason for the unique
// name.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	// 0700 rather than 0755: this directory holds the credentials file, and a
	// directory others can list is a directory whose contents they know to
	// come back for.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// A unique temporary name, not path+".tmp": two shantys started at once
	// would share that one name, and one could rename the other's
	// half-written file into place, defeating the atomicity this exists for.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".shanty-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Best-effort, and deliberately unchecked: after a successful rename there
	// is nothing here to remove, and after a failure the intact original file
	// is what matters, not the litter beside it.
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// CreateTemp makes the file 0600, which is right for credentials and wrong
	// for settings. Either way the mode is set before the rename, so nothing
	// ever observes the file at the wrong permissions -- not even for the
	// instant between two syscalls.
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
