package config

import (
	"errors"
	"path/filepath"
	"testing"
)

type parserNameConfig struct {
	Name string `json:"name" env:"CONFIG_V2_TEST_NAME"`
}

func TestDecodeConfigFileIntoRejectsUnsupportedExtension(t *testing.T) {
	var cfg parserNameConfig

	err := decodeConfigFileInto("config.toml", []byte("name = 'app'"), &cfg, false)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("decodeConfigFileInto() error = %v, want ErrInvalidConfig", err)
	}
}

func TestEncodeConfigFileRejectsDotEnvExtension(t *testing.T) {
	_, err := encodeConfigFile("config.env", parserNameConfig{Name: "app"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("encodeConfigFile() error = %v, want ErrInvalidConfig", err)
	}
}

func TestDecodeJSONIntoRejectsMultipleDocuments(t *testing.T) {
	var cfg parserNameConfig

	err := decodeJSONInto([]byte(`{"name":"first"} {"name":"second"}`), &cfg, false)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("decodeJSONInto() error = %v, want ErrInvalidConfig", err)
	}
}

func TestPathResolverResolvesMarkersAndRelativePaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	cwd := filepath.Join(t.TempDir(), "cwd")
	tmp := filepath.Join(t.TempDir(), "tmp")
	baseDir := filepath.Join(t.TempDir(), "base")

	resolver := PathResolver{
		Getwd:       func() (string, error) { return cwd, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		TempDir:     func() string { return tmp },
	}

	homePath := resolver.ResolveAgainst("{HOME}/config.json", "")
	if homePath != filepath.Join(home, "config.json") {
		t.Fatalf("ResolveAgainst({HOME}) = %q, want %q", homePath, filepath.Join(home, "config.json"))
	}

	relativePath := resolver.ResolveAgainst("config.json", baseDir)
	if relativePath != filepath.Join(baseDir, "config.json") {
		t.Fatalf("ResolveAgainst(relative) = %q, want %q", relativePath, filepath.Join(baseDir, "config.json"))
	}

	tmpPath := resolver.ResolveAgainst("{TMP}/cache.json", "")
	if tmpPath != filepath.Join(tmp, "cache.json") {
		t.Fatalf("ResolveAgainst({TMP}) = %q, want %q", tmpPath, filepath.Join(tmp, "cache.json"))
	}
}

func TestNewLoaderUsesCustomResolverAndEnvNames(t *testing.T) {
	dir := t.TempDir()
	envConfigPath := filepath.Join(dir, "env-config.json")
	envSecurityPath := filepath.Join(dir, "env-security.yml")

	resolver := PathResolver{
		Getenv: func(name string) string {
			switch name {
			case "CONFIG_V2_CUSTOM_CONFIG":
				return envConfigPath
			case "CONFIG_V2_CUSTOM_SECURITY":
				return envSecurityPath
			default:
				return ""
			}
		},
	}

	loader, err := NewLoader(
		WithPathResolver(resolver),
		WithConfigPathEnv("CONFIG_V2_CUSTOM_CONFIG"),
		WithSecurityPathEnv("CONFIG_V2_CUSTOM_SECURITY"),
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

func TestReadRegularFileRejectsDirectories(t *testing.T) {
	dir := t.TempDir()

	_, err := readRegularFile(dir, true)
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("readRegularFile() error = %v, want ErrUnsafePath", err)
	}
}

func TestWritePrivateFileRejectsDirectoryTargets(t *testing.T) {
	dir := t.TempDir()

	err := writePrivateFile(dir, []byte("content"), true)
	if !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("writePrivateFile() error = %v, want ErrUnsafePath", err)
	}
}

func TestLoadYAMLOverlayIgnoresMissingFile(t *testing.T) {
	cfg := &sampleConfig{}
	err := loadYAMLOverlay(cfg, filepath.Join(t.TempDir(), "missing.yml"), true)
	if err != nil {
		t.Fatalf("loadYAMLOverlay() error = %v", err)
	}
}

func TestSaveYAMLOverlayRejectsNilConfig(t *testing.T) {
	err := saveYAMLOverlay(filepath.Join(t.TempDir(), ".security.yml"), nil, true)
	if !errors.Is(err, ErrNilConfig) {
		t.Fatalf("saveYAMLOverlay() error = %v, want ErrNilConfig", err)
	}
}
