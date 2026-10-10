package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/Snah-s/commit_tool/internal/storage"
)

func readHook(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: message must be a regular file, not a symlink", path)
	}
	return os.ReadFile(path)
}

func replaceHook(path string, original, message []byte) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0222 == 0 {
		return fmt.Errorf("%s: message is not a writable regular file", path)
	}
	current, err := readHook(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, original) {
		return fmt.Errorf("%s: message changed while editing; original was not replaced", path)
	}
	return storage.WriteFile(path, message, info.Mode().Perm())
}
