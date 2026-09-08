package config

import (
	"os"
	"path/filepath"
)

// writeAtomic writes data to path with the given permissions, through a
// temporary file in the same directory. On any failure the existing file at
// path is left unchanged.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	// The directory is created 0700.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// CreateTemp gives each writer its own name, so two processes writing at once
	// cannot rename each other’s partial files into place.
	f, err := os.CreateTemp(dir, filepath.Base(path)+".shanty-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// Nothing remains to remove after a successful rename, and after a failure the
	// original file is what matters.
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Permissions are set before the rename, so the file never exists at path with
	// the wrong ones.
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
