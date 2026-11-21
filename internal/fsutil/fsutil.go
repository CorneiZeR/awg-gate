// Package fsutil writes the files awg-gate keeps - keys, configurations,
// gate.yaml - so that a reader never sees half of one and a failure never
// leaves a stray copy of a secret behind.
package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadFile reads a file named by the operator (a flag, an environment variable).
func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Clean(path))
}

// WriteAtomic replaces path with data through a temporary file in the same
// directory and a rename, so the file is always either the old or the new one.
func WriteAtomic(path string, data []byte, mode fs.FileMode) (err error) {
	path = filepath.Clean(path)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, discard(tmp))
		}
	}()
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// WriteNew creates path with data and refuses to replace an existing file.
func WriteNew(path string, data []byte, mode fs.FileMode) (err error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, discard(f))
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Close()
}

// WithTempFile writes data to a 0600 file in dir, hands its path to use and
// removes it afterwards, whatever use returns.
func WithTempFile(dir string, data []byte, use func(path string) error) (err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, discard(f)) }()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return use(f.Name())
}

// discard closes and removes a file that is being given up on. Closing an
// already closed file and removing a missing one are not errors here.
func discard(f *os.File) error {
	closeErr := f.Close()
	if errors.Is(closeErr, os.ErrClosed) {
		closeErr = nil
	}
	removeErr := os.Remove(f.Name())
	if errors.Is(removeErr, fs.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
}
