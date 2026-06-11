package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadGenericEncryptsSecretInSecurityOverlay(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	sshKeyPath := filepath.Join(dir, "id_ed25519")
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	if err := os.WriteFile(sshKeyPath, []byte("ssh-private-key"), 0o600); err != nil {
		t.Fatalf("write ssh key: %v", err)
	}

	oldPassphraseProvider := PassphraseProvider
	oldSSHKeyPathProvider := SSHKeyPathProvider
	PassphraseProvider = func() string { return "passphrase" }
	SSHKeyPathProvider = func() string { return sshKeyPath }
	t.Cleanup(func() {
		PassphraseProvider = oldPassphraseProvider
		SSHKeyPathProvider = oldSSHKeyPathProvider
	})

	saved := &sampleConfig{
		Port:   8080,
		Name:   "app",
		Secret: *NewSecureString("plain-secret"),
	}

	err := SaveGeneric(saved, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}

	securityData, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("read security: %v", err)
	}
	securityText := string(securityData)
	if strings.Contains(securityText, "plain-secret") {
		t.Fatalf("security overlay contains plaintext secret: %s", securityText)
	}
	if !strings.Contains(securityText, EncScheme) {
		t.Fatalf("security overlay does not contain encrypted secret: %s", securityText)
	}

	loaded, err := LoadGeneric(sampleDefaults, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if loaded.Secret.String() != "plain-secret" {
		t.Fatalf("loaded secret = %q, want plain-secret", loaded.Secret.String())
	}
}

func TestCredentialResolverRejectsEmptyFileReference(t *testing.T) {
	resolver := NewCredentialResolver(t.TempDir(), false)

	_, err := resolver.Resolve("file://   ")
	if err == nil {
		t.Fatal("Resolve() error = nil, want file reference error")
	}
}

func TestCredentialResolverFailsDecryptionWithWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	sshKeyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(sshKeyPath, []byte("ssh-private-key"), 0o600); err != nil {
		t.Fatalf("write ssh key: %v", err)
	}

	enc, err := Encrypt("right-passphrase", sshKeyPath, "plain-secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	oldPassphraseProvider := PassphraseProvider
	oldSSHKeyPathProvider := SSHKeyPathProvider
	PassphraseProvider = func() string { return "wrong-passphrase" }
	SSHKeyPathProvider = func() string { return sshKeyPath }
	t.Cleanup(func() {
		PassphraseProvider = oldPassphraseProvider
		SSHKeyPathProvider = oldSSHKeyPathProvider
	})

	resolver := NewCredentialResolver(dir, false)
	_, err = resolver.Resolve(enc)
	if !errors.Is(err, ErrDecryptionFailed) {
		t.Fatalf("Resolve() error = %v, want ErrDecryptionFailed", err)
	}
}
