package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSave(t *testing.T) {
	base := t.TempDir()
	store := NewLocal(base)

	path, err := store.Save(context.Background(), "file.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("save file: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected content: %s", string(content))
	}
}

func TestLocalSaveFailsForNestedPathWithoutParentDir(t *testing.T) {
	base := t.TempDir()
	store := NewLocal(base)
	if _, err := store.Save(context.Background(), filepath.Join("nested", "a.txt"), []byte("x")); err == nil {
		t.Fatal("expected save to fail for nested path without intermediate directory")
	}
}

func TestLocalSaveFailsWhenBaseIsAFile(t *testing.T) {
	baseFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(baseFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}

	store := NewLocal(baseFile)
	if _, err := store.Save(context.Background(), "a.txt", []byte("x")); err == nil {
		t.Fatal("expected save to fail when base path is file")
	}
}
