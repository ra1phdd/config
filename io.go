package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func readRegularFile(path string, allowSymlinks bool) ([]byte, error) {
	info, err := safeStat(path, allowSymlinks)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a directory", ErrUnsafePath, path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, path)
	}

	return os.ReadFile(path)
}

func writePrivateFile(path string, data []byte, allowSymlinks bool) error {
	if err := ensureWritableTarget(path, allowSymlinks); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func safeStat(path string, allowSymlinks bool) (os.FileInfo, error) {
	if allowSymlinks {
		return os.Stat(path)
	}

	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: refusing symlink %s", ErrUnsafePath, path)
	}
	return info, nil
}

func ensureWritableTarget(path string, allowSymlinks bool) error {
	info, err := safeStat(path, allowSymlinks)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%w: %s is a directory", ErrUnsafePath, path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafePath, path)
	}
	return nil
}
