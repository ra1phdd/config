package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var PassphraseProvider = func() string {
	return os.Getenv(PassphraseEnvVar)
}

var SSHKeyPathProvider = func() string {
	return os.Getenv(SSHKeyPathEnvVar)
}

var ErrPassphraseRequired = errors.New("credential: enc:// passphrase required")

var ErrDecryptionFailed = errors.New("credential: enc:// decryption failed (wrong passphrase or SSH key?)")

const (
	hkdfInfo = "config-credential-v1"
	saltLen  = 16
	nonceLen = 12
	keyLen   = 32
)

type CredentialResolver struct {
	baseDir       string
	allowSymlinks bool
	resolver      PathResolver
}

func NewCredentialResolver(baseDir string, allowSymlinks bool) *CredentialResolver {
	return &CredentialResolver{
		baseDir:       baseDir,
		allowSymlinks: allowSymlinks,
		resolver:      DefaultPathResolver(),
	}
}

func (r *CredentialResolver) Resolve(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}

	if after, ok := strings.CutPrefix(raw, FileScheme); ok {
		path := strings.TrimSpace(after)
		if path == "" {
			return "", errors.New("credential: file:// reference has no filename")
		}

		resolved := r.resolver.ResolveAgainst(path, r.baseDir)
		data, err := readRegularFile(resolved, r.allowSymlinks)
		if err != nil {
			return "", fmt.Errorf("credential: failed to read credential file %q: %w", resolved, err)
		}

		value := strings.TrimSpace(string(data))
		if value == "" {
			return "", fmt.Errorf("credential: credential file %q is empty", resolved)
		}
		return value, nil
	}

	if strings.HasPrefix(raw, EncScheme) {
		return resolveEncrypted(raw)
	}

	return raw, nil
}

func resolveEncrypted(raw string) (string, error) {
	passphrase := PassphraseProvider()
	if passphrase == "" {
		return "", ErrPassphraseRequired
	}

	sshKeyPath := pickSSHKeyPath("")
	b64 := strings.TrimPrefix(raw, EncScheme)
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("credential: enc:// invalid base64: %w", err)
	}
	if len(blob) < saltLen+nonceLen+1 {
		return "", errors.New("credential: enc:// payload too short")
	}

	salt := blob[:saltLen]
	nonce := blob[saltLen : saltLen+nonceLen]
	ciphertext := blob[saltLen+nonceLen:]

	key, err := deriveKey(passphrase, sshKeyPath, salt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("credential: enc:// cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("credential: enc:// gcm init: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}
	return string(plaintext), nil
}

func Encrypt(passphrase, sshKeyPath, plaintext string) (string, error) {
	if passphrase == "" {
		return "", errors.New("credential: passphrase must not be empty")
	}
	sshKeyPath = pickSSHKeyPath(sshKeyPath)

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("credential: failed to generate salt: %w", err)
	}

	key, err := deriveKey(passphrase, sshKeyPath, salt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("credential: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("credential: gcm init: %w", err)
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("credential: failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	blob := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ciphertext...)
	return EncScheme + base64.StdEncoding.EncodeToString(blob), nil
}

func deriveKey(passphrase, sshKeyPath string, salt []byte) ([]byte, error) {
	if sshKeyPath == "" {
		return nil, errors.New("credential: SSH private key is required but not found (set CONFIG_SSH_KEY_PATH or place a key in ~/.ssh)")
	}
	sshBytes, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("credential: cannot read SSH key %q: %w", sshKeyPath, err)
	}
	sshHash := sha256.Sum256(sshBytes)
	mac := hmac.New(sha256.New, sshHash[:])
	_, _ = mac.Write([]byte(passphrase))
	ikm := mac.Sum(nil)

	key, err := hkdf.Key(sha256.New, ikm, salt, hkdfInfo, keyLen)
	if err != nil {
		return nil, fmt.Errorf("credential: HKDF expand failed: %w", err)
	}
	return key, nil
}

func pickSSHKeyPath(override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	if path := strings.TrimSpace(SSHKeyPathProvider()); path != "" {
		return path
	}
	return findDefaultSSHKey()
}

func findDefaultSSHKey() string {
	paths, err := defaultSSHKeyCandidates()
	if err != nil {
		return ""
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func DefaultSSHKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("credential: cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "id_ed25519"), nil
}

func defaultSSHKeyCandidates() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("credential: cannot determine home directory: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	return []string{
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_rsa"),
	}, nil
}
