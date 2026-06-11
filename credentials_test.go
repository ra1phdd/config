package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialResolverResolvesRelativeFileRef(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "api.key")
	if err := os.WriteFile(keyPath, []byte(" secret \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	resolver := NewCredentialResolver(dir, false)
	value, err := resolver.Resolve(FileScheme + "api.key")
	if err != nil {
		t.Fatal(err)
	}
	if value != "secret" {
		t.Fatalf("value = %q, want secret", value)
	}
}

func TestCredentialResolverResolvesEncryptedValue(t *testing.T) {
	dir := t.TempDir()
	sshKeyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(sshKeyPath, []byte("ssh-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(PassphraseEnvVar, "passphrase")
	t.Setenv(SSHKeyPathEnvVar, sshKeyPath)

	enc, err := Encrypt("passphrase", sshKeyPath, "plain-secret")
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewCredentialResolver(dir, false)
	value, err := resolver.Resolve(enc)
	if err != nil {
		t.Fatal(err)
	}
	if value != "plain-secret" {
		t.Fatalf("value = %q, want plain-secret", value)
	}
}

func TestCredentialResolverRequiresPassphraseForEncryptedValue(t *testing.T) {
	dir := t.TempDir()
	sshKeyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(sshKeyPath, []byte("ssh-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	enc, err := Encrypt("passphrase", sshKeyPath, "plain-secret")
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv(PassphraseEnvVar, "")
	t.Setenv(SSHKeyPathEnvVar, sshKeyPath)

	resolver := NewCredentialResolver(dir, false)
	_, err = resolver.Resolve(enc)
	if !errors.Is(err, ErrPassphraseRequired) {
		t.Fatalf("Resolve() error = %v, want ErrPassphraseRequired", err)
	}
}
