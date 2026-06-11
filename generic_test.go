package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testEnvPort   = "CONFIG_V2_TEST_PORT"
	testEnvName   = "CONFIG_V2_TEST_NAME"
	testEnvSecret = "CONFIG_V2_TEST_SECRET"
)

type sampleConfig struct {
	Port   int          `json:"port" yaml:"port" env:"CONFIG_V2_TEST_PORT"`
	Name   string       `json:"name" yaml:"name" env:"CONFIG_V2_TEST_NAME"`
	Secret SecureString `json:"secret,omitzero" yaml:"secret,omitempty" env:"CONFIG_V2_TEST_SECRET"`
}

type interfaceFieldConfig struct {
	Secret SecureString   `yaml:"secret,omitempty"`
	Extra  map[string]any `yaml:"extra,omitempty"`
}

type botOverlayConfig struct {
	Bots []botOverlayBot `json:"bots" yaml:"bots"`
}

type botOverlayBot struct {
	Name   string       `json:"name" yaml:"name"`
	APIKey SecureString `json:"api_key,omitzero" yaml:"api_key,omitempty"`
}

func (c *sampleConfig) Validate() error {
	if c.Port < 0 {
		return errors.New("port cannot be negative")
	}
	return nil
}

func sampleDefaults() *sampleConfig {
	return &sampleConfig{
		Port:   1000,
		Name:   "default",
		Secret: *NewSecureString("default-secret"),
	}
}

func cleanupSampleEnv(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = os.Unsetenv(testEnvPort)
		_ = os.Unsetenv(testEnvName)
		_ = os.Unsetenv(testEnvSecret)
	})
}

func TestLoadGenericMergesDefaultsJSONSecurityDotEnvAndProcessEnv(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dotEnvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(configPath, []byte(`{"port":2000,"name":"json","secret":"json-secret"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(securityPath, []byte("secret: security-secret\nname: ignored-security-name\n"), 0o600); err != nil {
		t.Fatalf("write security: %v", err)
	}
	dotenv := strings.Join([]string{
		testEnvPort + "=3000",
		testEnvName + "=dotenv",
		testEnvSecret + "=dotenv-secret",
		"",
	}, "\n")
	if err := os.WriteFile(dotEnvPath, []byte(dotenv), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv(testEnvPort, "4000")

	cfg, err := LoadGeneric(sampleDefaults, WithConfigPath(configPath), WithSecurityPath(securityPath), WithDotEnv(dotEnvPath))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}

	if cfg.Port != 4000 {
		t.Fatalf("Port = %d, want process env override 4000", cfg.Port)
	}
	if cfg.Name != "dotenv" {
		t.Fatalf("Name = %q, want dotenv", cfg.Name)
	}
	if got := cfg.Secret.String(); got != "dotenv-secret" {
		t.Fatalf("Secret = %q, want dotenv-secret", got)
	}
}

func TestLoadGenericCanSkipDotEnv(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	dotEnvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(configPath, []byte(`{"port":2000,"name":"json"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(dotEnvPath, []byte(testEnvName+"=dotenv\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	cfg, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(filepath.Join(dir, ".security.yml")),
		WithDotEnv(dotEnvPath),
		WithDotEnvEnabled(false),
	)
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Name != "json" {
		t.Fatalf("Name = %q, want json", cfg.Name)
	}
}

func TestSaveGenericKeepsSecureStringOutOfJSON(t *testing.T) {
	cleanupSampleEnv(t)
	t.Setenv(PassphraseEnvVar, "")
	t.Setenv(SSHKeyPathEnvVar, "")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	cfg := &sampleConfig{
		Port:   8080,
		Name:   "app",
		Secret: *NewSecureString("save-secret"),
	}

	if err := SaveGeneric(cfg, WithConfigPath(configPath), WithSecurityPath(securityPath)); err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(configData), "save-secret") {
		t.Fatalf("config.json contains secret: %s", string(configData))
	}

	securityData, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("read security: %v", err)
	}
	securityText := string(securityData)
	if !strings.Contains(securityText, "save-secret") {
		t.Fatalf("security file does not contain secret: %s", securityText)
	}
	if strings.Contains(securityText, "8080") || strings.Contains(securityText, "app") {
		t.Fatalf("security file contains non-secret config: %s", securityText)
	}
}

func TestSaveGenericDoesNotEscapeHTMLInJSON(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	cfg := &sampleConfig{
		Port: 8080,
		Name: "<b>name</b>",
	}

	if err := SaveGeneric(cfg, WithConfigPath(configPath), WithSecurityPath(securityPath)); err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	configText := string(configData)
	if strings.Contains(configText, `\u003c`) || strings.Contains(configText, `\u003e`) {
		t.Fatalf("config.json contains escaped html: %s", configText)
	}
	if !strings.Contains(configText, `<b>name</b>`) {
		t.Fatalf("config.json does not contain raw html: %s", configText)
	}
}

func TestYAMLSecurityOverlayIgnoresInterfaceMapFields(t *testing.T) {
	dir := t.TempDir()
	securityPath := filepath.Join(dir, ".security.yml")
	if err := saveYAMLOverlay(securityPath, &interfaceFieldConfig{
		Secret: *NewSecureString("save-secret"),
		Extra:  map[string]any{"ignored": "value"},
	}, true); err != nil {
		t.Fatalf("saveYAMLOverlay() error = %v", err)
	}
	securityData, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("read security: %v", err)
	}
	securityText := string(securityData)
	if !strings.Contains(securityText, "save-secret") {
		t.Fatalf("security file does not contain secret: %s", securityText)
	}
	if strings.Contains(securityText, "ignored") || strings.Contains(securityText, "value") {
		t.Fatalf("security file contains non-secret interface map data: %s", securityText)
	}

	if err := os.WriteFile(securityPath, []byte("secret: security-secret\nextra:\n  ignored: value\n"), 0o600); err != nil {
		t.Fatalf("write security: %v", err)
	}

	var cfg interfaceFieldConfig
	if err := decodeYAMLOverlay(securityPath, &cfg, true); err != nil {
		t.Fatalf("decodeYAMLOverlay() error = %v", err)
	}

	if got := cfg.Secret.String(); got != "security-secret" {
		t.Fatalf("Secret = %q, want security-secret", got)
	}
	if cfg.Extra != nil {
		t.Fatalf("Extra = %#v, want nil", cfg.Extra)
	}
}

func TestLoadGenericRunsValidator(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"port":-1}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadGeneric(sampleDefaults, WithConfigPath(configPath), WithSecurityPath(filepath.Join(dir, ".security.yml")))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("LoadGeneric() error = %v, want ErrInvalidConfig", err)
	}
}

func TestYAMLSecurityOverlayMergesSliceItemsByName(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	securityPath := filepath.Join(dir, ".security.yml")

	configData := strings.TrimSpace(`
bots:
  - name: main
  - name: music
`)
	securityData := strings.TrimSpace(`
bots:
  - name: main
    api_key: main-secret
  - name: music
    api_key: music-secret
`)

	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(securityPath, []byte(securityData), 0o600); err != nil {
		t.Fatalf("write security: %v", err)
	}

	cfg, err := LoadGeneric(func() *botOverlayConfig { return &botOverlayConfig{} }, WithConfigPath(configPath), WithSecurityPath(securityPath))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if len(cfg.Bots) != 2 {
		t.Fatalf("len(Bots) = %d, want 2", len(cfg.Bots))
	}
	if cfg.Bots[0].Name != "main" || cfg.Bots[0].APIKey.String() != "main-secret" {
		t.Fatalf("bot[0] = %+v", cfg.Bots[0])
	}
	if cfg.Bots[1].Name != "music" || cfg.Bots[1].APIKey.String() != "music-secret" {
		t.Fatalf("bot[1] = %+v", cfg.Bots[1])
	}
}

func TestSaveGenericKeepsMergeKeyInSecuritySliceOverlay(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	securityPath := filepath.Join(dir, ".security.yml")
	cfg := &botOverlayConfig{
		Bots: []botOverlayBot{
			{Name: "main", APIKey: *NewSecureString("main-secret")},
			{Name: "music", APIKey: *NewSecureString("music-secret")},
		},
	}

	if err := SaveGeneric(cfg, WithConfigPath(configPath), WithSecurityPath(securityPath)); err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}

	securityData, err := os.ReadFile(securityPath)
	if err != nil {
		t.Fatalf("read security: %v", err)
	}
	securityText := string(securityData)
	if !strings.Contains(securityText, "name: main") || !strings.Contains(securityText, "name: music") {
		t.Fatalf("security overlay missing merge keys: %s", securityText)
	}
	if !strings.Contains(securityText, "main-secret") || !strings.Contains(securityText, "music-secret") {
		t.Fatalf("security overlay missing secrets: %s", securityText)
	}
}

func TestLoadGenericParsesYAMLConfigPath(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("port: 9090\nname: yaml\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadGeneric(sampleDefaults, WithConfigPath(configPath), WithSecurityPath(filepath.Join(dir, ".security.yml")))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Port != 9090 || cfg.Name != "yaml" {
		t.Fatalf("cfg = %+v, want yaml values", cfg)
	}
}

func TestLoadGenericParsesEnvConfigPath(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(configPath, []byte(testEnvPort+"=7777\n"+testEnvName+"=envfile\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadGeneric(sampleDefaults, WithConfigPath(configPath), WithSecurityPath(filepath.Join(dir, ".security.yml")))
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Port != 7777 || cfg.Name != "envfile" {
		t.Fatalf("cfg = %+v, want env values", cfg)
	}
}

func TestNewLoaderUsesExplicitPaths(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "custom.json")
	securityPath := filepath.Join(dir, "custom-security.yml")

	loader, err := NewLoader(WithConfigPath(configPath), WithSecurityPath(securityPath))
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}
	if loader.ConfigPath() != configPath {
		t.Fatalf("ConfigPath() = %q, want %q", loader.ConfigPath(), configPath)
	}
	if loader.SecurityPath() != securityPath {
		t.Fatalf("SecurityPath() = %q, want %q", loader.SecurityPath(), securityPath)
	}
}

func TestNewLoaderPrefersEnvPathsOverConfiguredPaths(t *testing.T) {
	dir := t.TempDir()
	envConfigPath := filepath.Join(dir, "env-config.json")
	envSecurityPath := filepath.Join(dir, "env-security.yml")

	t.Setenv(EnvConfig, envConfigPath)
	t.Setenv(EnvSecurity, envSecurityPath)

	loader, err := NewLoader(
		WithConfigPath(filepath.Join(dir, "configured.json")),
		WithSecurityPath(filepath.Join(dir, "configured-security.yml")),
	)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}
	if loader.ConfigPath() != envConfigPath {
		t.Fatalf("ConfigPath() = %q, want %q", loader.ConfigPath(), envConfigPath)
	}
	if loader.SecurityPath() != envSecurityPath {
		t.Fatalf("SecurityPath() = %q, want %q", loader.SecurityPath(), envSecurityPath)
	}
}

func TestNewLoaderPanicsWithoutPaths(t *testing.T) {
	t.Setenv(EnvConfig, "")
	t.Setenv(EnvSecurity, "")

	defer func() {
		if recover() == nil {
			t.Fatal("NewLoader() did not panic without paths")
		}
	}()

	_, _ = NewLoader()
}

func TestSaveGenericRejectsNilConfig(t *testing.T) {
	var cfg *sampleConfig

	err := SaveGeneric(cfg)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("SaveGeneric() error = %v, want ErrNilConfig", err)
	}
}

func TestLoaderLoadIntoRejectsNilTarget(t *testing.T) {
	dir := t.TempDir()
	loader, err := NewLoader(
		WithConfigPath(filepath.Join(dir, "config.json")),
		WithSecurityPath(filepath.Join(dir, ".security.yml")),
	)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}

	err = loader.LoadInto(nil)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("LoadInto() error = %v, want ErrNilConfig", err)
	}
}

func TestLoadGenericRejectsMissingConfigWhenDisallowed(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "missing.json")
	securityPath := filepath.Join(dir, ".security.yml")

	_, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithMissingConfigAllowed(false),
	)
	if err == nil || !strings.Contains(err.Error(), "read config:") || !strings.Contains(err.Error(), filepath.Base(configPath)) {
		t.Fatalf("LoadGeneric() error = %v, want missing-config read error", err)
	}
}

func TestLoadGenericRejectsEmptyConfigWhenDisallowed(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	if err := os.WriteFile(configPath, []byte(" \n\t"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithEmptyConfigAllowed(false),
	)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("LoadGeneric() error = %v, want ErrInvalidConfig", err)
	}
}

func TestLoadGenericRejectsUnknownJSONFieldsInStrictMode(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	if err := os.WriteFile(configPath, []byte(`{"port":8080,"unknown":true}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithStrictJSON(true),
	)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("LoadGeneric() error = %v, want unknown field error", err)
	}
}

func TestLoadGenericSkipsValidationWhenDisabled(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	if err := os.WriteFile(configPath, []byte(`{"port":-1}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithEnvironment(false),
		WithValidation(false),
	)
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Port != -1 {
		t.Fatalf("Port = %d, want -1", cfg.Port)
	}
}

func TestLoadGenericCanDisableEnvironmentOverrides(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dotEnvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(configPath, []byte(`{"name":"json"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(dotEnvPath, []byte(testEnvName+"=dotenv\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv(testEnvName, "process")

	cfg, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithDotEnv(dotEnvPath),
		WithEnvironment(false),
	)
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Name != "json" {
		t.Fatalf("Name = %q, want json", cfg.Name)
	}
}

func TestLoadGenericDotEnvOverrideCanReplaceProcessEnv(t *testing.T) {
	cleanupSampleEnv(t)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dotEnvPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(configPath, []byte(`{"name":"json"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(dotEnvPath, []byte(testEnvName+"=dotenv\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv(testEnvName, "process")

	cfg, err := LoadGeneric(
		sampleDefaults,
		WithConfigPath(configPath),
		WithSecurityPath(securityPath),
		WithDotEnv(dotEnvPath),
		WithDotEnvOverride(true),
	)
	if err != nil {
		t.Fatalf("LoadGeneric() error = %v", err)
	}
	if cfg.Name != "dotenv" {
		t.Fatalf("Name = %q, want dotenv", cfg.Name)
	}
}

func TestSaveGenericRejectsInvalidConfigDuringValidation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	cfg := &sampleConfig{Port: -1}

	err := SaveGeneric(cfg, WithConfigPath(configPath), WithSecurityPath(securityPath))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("SaveGeneric() error = %v, want ErrInvalidConfig", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file stat error = %v, want not exists", statErr)
	}
}
