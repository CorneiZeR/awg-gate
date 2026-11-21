package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicReplacesAndSetsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	st, _ := os.Stat(path)
	if string(got) != "new" || st.Mode().Perm() != 0o600 {
		t.Fatalf("content %q mode %o", got, st.Mode().Perm())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmp-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestWriteNewRefusesToReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := WriteNew(path, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteNew(path, []byte("b"), 0o600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second WriteNew: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "a" {
		t.Fatalf("content replaced: %q", got)
	}
}

func TestWithTempFileRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	var seen string
	useErr := errors.New("use failed")
	err := WithTempFile(dir, []byte("secret"), func(p string) error {
		seen = p
		if got, _ := os.ReadFile(p); string(got) != "secret" {
			t.Errorf("content %q", got)
		}
		return useErr
	})
	if !errors.Is(err, useErr) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(seen); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the temporary file survived: %v", err)
	}
}
