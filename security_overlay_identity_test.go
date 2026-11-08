package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type mergeLiveSync struct {
	SetupURI        SecureString `json:"setup_uri,omitzero" yaml:"setup_uri,omitempty"`
	SetupPassphrase SecureString `json:"setup_passphrase,omitzero" yaml:"setup_passphrase,omitempty"`
}

type mergeProfile struct {
	Name             string        `json:"name" yaml:"name"`
	SystemPromptPath string        `json:"system_prompt_path" yaml:"system_prompt_path"`
	LiveSync         mergeLiveSync `json:"livesync" yaml:"livesync"`
}

type mergeOwner struct {
	TelegramID     string         `json:"telegram_id" yaml:"telegram_id"`
	DefaultProfile string         `json:"default_profile" yaml:"default_profile"`
	Profiles       []mergeProfile `json:"profiles" yaml:"profiles"`
}

func TestSecurityOverlayMergesProfileWithoutReplacingData(t *testing.T) {
	for _, ext := range []string{".json", ".yaml"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config"+ext)
			securityPath := filepath.Join(dir, ".security.yml")
			var main []byte
			if ext == ".json" {
				main = []byte(`{"telegram_id":"42","default_profile":"main","profiles":[{"name":"main","system_prompt_path":"main.txt"},{"name":"other","system_prompt_path":"other.txt"}]}`)
			} else {
				main = []byte("telegram_id: \"42\"\ndefault_profile: main\nprofiles:\n  - name: main\n    system_prompt_path: main.txt\n  - name: other\n    system_prompt_path: other.txt\n")
			}
			security := []byte("profiles:\n  - name: main\n    livesync:\n      setup_uri: secret-uri\n      setup_passphrase: secret-pass\n")
			if err := os.WriteFile(configPath, main, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(securityPath, security, 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := LoadGeneric(func() *mergeOwner { return &mergeOwner{} }, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
			if err != nil {
				t.Fatalf("LoadGeneric() error = %v", err)
			}
			if len(got.Profiles) != 2 || got.Profiles[0].SystemPromptPath != "main.txt" || got.Profiles[1].SystemPromptPath != "other.txt" {
				t.Fatalf("profiles were replaced: %#v", got.Profiles)
			}
			if got.Profiles[0].LiveSync.SetupURI.String() != "secret-uri" || got.Profiles[0].LiveSync.SetupPassphrase.String() != "secret-pass" {
				t.Fatalf("secrets not merged: %#v", got.Profiles[0])
			}
		})
	}
}

type pointerMergeOwner struct {
	Profiles []*mergeProfile `yaml:"profiles"`
}

type keyedAccount struct {
	Provider string       `yaml:"provider" securityMerge:"key"`
	Region   string       `yaml:"region" securityMerge:"key"`
	Label    string       `yaml:"label"`
	Token    SecureString `yaml:"token,omitempty"`
}

type keyedAccounts struct {
	Accounts []keyedAccount `yaml:"accounts"`
}

func TestSecurityOverlayExplicitCompositeKey(t *testing.T) {
	got := keyedAccounts{Accounts: []keyedAccount{
		{Provider: "telegram", Region: "eu", Label: "keep-eu"},
		{Provider: "telegram", Region: "asia", Label: "keep-asia"},
	}}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - provider: telegram\n    region: asia\n    token: asia-secret\n"), &got)
	if err != nil {
		t.Fatalf("merge error = %v", err)
	}
	if len(got.Accounts) != 2 || got.Accounts[1].Label != "keep-asia" || got.Accounts[1].Token.String() != "asia-secret" {
		t.Fatalf("accounts = %#v", got.Accounts)
	}
}

func TestSecurityOverlayExplicitCompositeKeyRequiresEveryField(t *testing.T) {
	got := keyedAccounts{Accounts: []keyedAccount{{Provider: "telegram", Region: "eu"}}}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - provider: telegram\n    token: secret\n"), &got)
	if !errors.Is(err, ErrInvalidMergeKey) {
		t.Fatalf("error = %v, want ErrInvalidMergeKey", err)
	}
}

type automaticAccount struct {
	Name  string       `yaml:"name"`
	ID    string       `yaml:"id"`
	Label string       `yaml:"label"`
	Token SecureString `yaml:"token,omitempty"`
}

type automaticAccounts struct {
	Accounts []automaticAccount `yaml:"accounts"`
}

func TestSecurityOverlayRejectsConflictingAutomaticSelectors(t *testing.T) {
	got := automaticAccounts{Accounts: []automaticAccount{{Name: "first", ID: "1"}, {Name: "second", ID: "2"}}}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - name: first\n    id: \"2\"\n    token: secret\n"), &got)
	if !errors.Is(err, ErrAmbiguousMerge) {
		t.Fatalf("error = %v, want ErrAmbiguousMerge", err)
	}
	if got.Accounts[0].Token.String() != "" || got.Accounts[1].Token.String() != "" {
		t.Fatal("collection mutated after conflict")
	}
}

func TestSecurityOverlayAppendsUnmatchedElement(t *testing.T) {
	got := automaticAccounts{Accounts: []automaticAccount{{Name: "first", ID: "1", Label: "keep"}}}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - name: second\n    id: \"2\"\n    token: secret\n"), &got)
	if err != nil {
		t.Fatalf("merge error = %v", err)
	}
	if len(got.Accounts) != 2 || got.Accounts[0].Label != "keep" || got.Accounts[1].Name != "second" || got.Accounts[1].ID != "2" || got.Accounts[1].Token.String() != "secret" {
		t.Fatalf("accounts = %#v", got.Accounts)
	}
}

func TestSecurityOverlayRejectsDuplicateAutomaticSelector(t *testing.T) {
	got := automaticAccounts{Accounts: []automaticAccount{{Name: "same"}, {Name: "same"}}}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - name: same\n    token: secret\n"), &got)
	if !errors.Is(err, ErrAmbiguousMerge) {
		t.Fatalf("error = %v, want ErrAmbiguousMerge", err)
	}
}

type invalidKeyItem struct {
	ID    string       `yaml:"id" securityMerge:"primary"`
	Token SecureString `yaml:"token,omitempty"`
}
type invalidKeyItems struct {
	Items []invalidKeyItem `yaml:"items"`
}

func TestSecurityOverlayRejectsInvalidMergeTag(t *testing.T) {
	var got invalidKeyItems
	err := mergeOverlayYAMLForTest([]byte("items:\n  - id: one\n    token: secret\n"), &got)
	if !errors.Is(err, ErrInvalidMergeKey) {
		t.Fatalf("error = %v, want ErrInvalidMergeKey", err)
	}
}

type arrayAccounts struct {
	Accounts [1]automaticAccount `yaml:"accounts"`
}

func TestSecurityOverlayMergesFixedArrayMatchAndRejectsAppend(t *testing.T) {
	got := arrayAccounts{Accounts: [1]automaticAccount{{Name: "first", Label: "keep"}}}
	if err := mergeOverlayYAMLForTest([]byte("accounts:\n  - name: first\n    token: secret\n"), &got); err != nil {
		t.Fatalf("matching merge error = %v", err)
	}
	if got.Accounts[0].Label != "keep" || got.Accounts[0].Token.String() != "secret" {
		t.Fatalf("account = %#v", got.Accounts[0])
	}
	err := mergeOverlayYAMLForTest([]byte("accounts:\n  - name: second\n    token: other\n"), &got)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("append error = %v, want ErrInvalidConfig", err)
	}
	if got.Accounts[0].Name != "first" || got.Accounts[0].Token.String() != "secret" {
		t.Fatalf("array overwritten: %#v", got.Accounts)
	}
}

type pointerKeyedAccounts struct {
	Accounts []*keyedAccount `json:"accounts" yaml:"accounts"`
}

func TestSecurityOverlayPointerCompositeKeySaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	want := &pointerKeyedAccounts{Accounts: []*keyedAccount{{Provider: "telegram", Region: "eu", Label: "keep", Token: *NewSecureString("secret")}}}
	if err := SaveGeneric(want, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false)); err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}
	got, err := LoadGeneric(func() *pointerKeyedAccounts { return &pointerKeyedAccounts{} }, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0] == nil || got.Accounts[0].Provider != "telegram" || got.Accounts[0].Region != "eu" || got.Accounts[0].Label != "keep" || got.Accounts[0].Token.String() != "secret" {
		t.Fatalf("accounts = %#v", got.Accounts)
	}
}

func TestSecurityOverlayMergesPointerProfile(t *testing.T) {
	var got pointerMergeOwner
	got.Profiles = []*mergeProfile{{Name: "main", SystemPromptPath: "keep.txt"}}
	err := mergeOverlayYAMLForTest([]byte("profiles:\n  - name: main\n    livesync:\n      setup_uri: secret-uri\n"), &got)
	if err != nil {
		t.Fatalf("merge error = %v", err)
	}
	if len(got.Profiles) != 1 || got.Profiles[0] == nil || got.Profiles[0].SystemPromptPath != "keep.txt" || got.Profiles[0].LiveSync.SetupURI.String() != "secret-uri" {
		t.Fatalf("profiles = %#v", got.Profiles)
	}
}

func mergeOverlayYAMLForTest(data []byte, target any) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	filtered, ok := filterSecurityOverlayNode(node.Content[0], target)
	if !ok {
		return errors.New("overlay was filtered out")
	}
	return mergeYAMLOverlayNodeIntoTarget(filtered, target)
}
