package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGenericRejectsSymlinkedConfigWhenDisallowed(t *testing.T) {
	symlinkPath, securityPath := writeSymlinkedConfigOrSkip(t)

	_, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(symlinkPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithSymlinksAllowed(false),
	)
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("LoadGeneric() error = %v, want ErrUnsafePath", err)
	}
}

func TestLoadGenericAllowsSymlinkedConfigWhenEnabled(t *testing.T) {
	symlinkPath, securityPath := writeSymlinkedConfigOrSkip(t)

	cfg, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(symlinkPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithSymlinksAllowed(true),
	)
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfg.Port)
	}
}

func writeSymlinkedConfigOrSkip(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	actualPath := filepath.Join(dir, "actual.json")
	symlinkPath := filepath.Join(dir, "config-link.json")
	securityPath := filepath.Join(dir, ".security.yml")

	if err := os.WriteFile(actualPath, []byte(`{"port":8080}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.Symlink(actualPath, symlinkPath); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	return symlinkPath, securityPath
}
