package config

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const notHere = `"[NOT_HERE]"`

type SecureStrings []*SecureString

func (s *SecureStrings) Values() []string {
	if s == nil {
		return nil
	}
	keys := make([]string, len(*s))
	for i, k := range *s {
		keys[i] = k.String()
	}
	return unique(keys)
}

func SimpleSecureStrings(val ...string) SecureStrings {
	val = unique(val)
	vv := make(SecureStrings, len(val))
	for i, s := range val {
		vv[i] = NewSecureString(s)
	}
	return vv
}

func (s SecureStrings) MarshalJSON() ([]byte, error) {
	return []byte(notHere), nil
}

func (s *SecureStrings) UnmarshalJSON(value []byte) error {
	if string(value) == notHere {
		return nil
	}
	var v []*SecureString
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	*s = v
	return nil
}

type SecureString struct {
	resolved string
	raw      string
}

func NewSecureString(value string) *SecureString {
	s := &SecureString{}
	_ = s.fromRaw(value)
	return s
}

func (s *SecureString) String() string {
	if s == nil {
		return ""
	}
	return s.resolved
}

func (s *SecureString) Set(value string) *SecureString {
	s.resolved = value
	s.raw = ""
	return s
}

func (s *SecureString) IsZero() bool {
	if callerFromYAML() {
		return true
	}
	return s.resolved == ""
}

func (s *SecureString) MarshalJSON() ([]byte, error) {
	return []byte(notHere), nil
}

func (s *SecureString) UnmarshalJSON(value []byte) error {
	if string(value) == notHere {
		return nil
	}
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	return s.fromRaw(v)
}

func (s *SecureString) MarshalYAML() (any, error) {
	if strings.HasPrefix(s.raw, EncScheme) || strings.HasPrefix(s.raw, FileScheme) {
		return s.raw, nil
	}
	if strings.HasPrefix(s.resolved, EncScheme) || strings.HasPrefix(s.resolved, FileScheme) {
		return s.resolved, nil
	}
	if passphrase := PassphraseProvider(); passphrase != "" {
		encrypted, err := Encrypt(passphrase, "", s.resolved)
		if err != nil {
			return nil, err
		}
		return encrypted, nil
	}
	return s.resolved, nil
}

func (s *SecureString) UnmarshalYAML(value *yaml.Node) error {
	return s.fromRaw(value.Value)
}

func (s *SecureString) UnmarshalText(text []byte) error {
	return s.fromRaw(string(text))
}

func (s *SecureString) fromRaw(v string) error {
	s.raw = v
	vv, err := resolveKey(v)
	if err != nil {
		return err
	}
	s.resolved = vv
	return nil
}

var (
	secResolverMu sync.RWMutex
	secResolver   *CredentialResolver
)

func setCredentialResolver(path string, allowSymlinks bool) {
	secResolverMu.Lock()
	defer secResolverMu.Unlock()
	secResolver = NewCredentialResolver(path, allowSymlinks)
}

func resolveKey(v string) (string, error) {
	secResolverMu.RLock()
	resolver := secResolver
	secResolverMu.RUnlock()
	if resolver == nil {
		resolver = NewCredentialResolver("", false)
	}
	if strings.HasPrefix(v, EncScheme) || strings.HasPrefix(v, FileScheme) {
		return resolver.Resolve(v)
	}
	return v, nil
}

func callerFromYAML() bool {
	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return false
	}
	return !strings.Contains(filepath.Dir(file), "yaml.v")
}

func unique[T comparable](input []T) []T {
	m := make(map[T]struct{}, len(input))
	var result []T
	for _, v := range input {
		if _, ok := m[v]; !ok {
			m[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
