package storage

import (
	"io/fs"
	"os"
	"path/filepath"
)

// WriteFile replaces a file only after its complete contents have been written.
// The temporary file stays on the same filesystem; failed renames leave the old file intact.
func WriteFile(path string, data []byte, mode fs.FileMode) error {
	return writeFile(path, data, mode, true)
}

// CreateFile installs a fully written file without overwriting an existing destination.
func CreateFile(path string, data []byte, mode fs.FileMode) error {
	return writeFile(path, data, mode, false)
}

func writeFile(path string, data []byte, mode fs.FileMode, replace bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".gitmoji-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if replace {
		return os.Rename(tmp.Name(), path)
	}
	return os.Link(tmp.Name(), path)
}
