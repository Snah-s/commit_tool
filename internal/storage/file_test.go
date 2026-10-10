package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplacementAndFailedWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "file.json")
	if err := WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CreateFile(path, []byte("overwrite"), 0600); err == nil {
		t.Fatal("CreateFile overwrote an existing file")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "old" {
		t.Fatal("failed creation changed original")
	}
	if err := WriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "new" {
		t.Fatal("replacement failed")
	}
	target := filepath.Join(root, "directory")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(target, "keep")
	if err := os.WriteFile(keep, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(target, []byte("bad"), 0600); err == nil {
		t.Fatal("replaced a directory")
	}
	data, _ = os.ReadFile(keep)
	if string(data) != "safe" {
		t.Fatal("failed replacement changed destination")
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".gitmoji-*"))
	if len(matches) != 0 {
		t.Fatalf("leaked temporary files: %v", matches)
	}
}
