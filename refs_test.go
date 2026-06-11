package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

type fileRefConfig struct {
	Name string            `json:"name" yaml:"name"`
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty" config:"file=data.json"`
}

func TestLoadAndSaveGenericFileRefs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dataPath := filepath.Join(dir, "data.json")

	if err := os.WriteFile(configPath, []byte(`{"name":"app","data":"file://data.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dataPath, []byte(`{"hello":"world"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	loader, err := NewLoader(WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &fileRefConfig{}
	if err := loader.LoadInto(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Data["hello"] != "world" {
		t.Fatalf("data = %#v, want loaded sidecar value", cfg.Data)
	}

	cfg.Data["saved"] = "again"
	if err := loader.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var savedMain map[string]any
	readJSONFile(t, configPath, &savedMain)
	if savedMain["data"] != "file://data.json" {
		t.Fatalf("data ref = %#v, want file://data.json", savedMain["data"])
	}

	var savedData map[string]string
	readJSONFile(t, dataPath, &savedData)
	if savedData["saved"] != "again" {
		t.Fatalf("saved sidecar = %#v, want updated value", savedData)
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

type yamlFileRefConfig struct {
	Name string            `json:"name" yaml:"name"`
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty" config:"file=data.yaml"`
}

func TestLoadAndSaveGenericYAMLFileRefs(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	securityPath := filepath.Join(dir, ".security.yml")
	dataPath := filepath.Join(dir, "data.yaml")

	if err := os.WriteFile(configPath, []byte("name: app\ndata: file://data.yaml\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(dataPath, []byte("hello: world\n"), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	loader, err := NewLoader(WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}

	cfg := &yamlFileRefConfig{}
	err = loader.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if cfg.Data["hello"] != "world" {
		t.Fatalf("data = %#v, want loaded sidecar value", cfg.Data)
	}

	cfg.Data["saved"] = "again"
	err = loader.Save(cfg)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(configData) != "name: app\ndata: file://data.yaml\n" {
		t.Fatalf("config.yaml = %q, want file reference preserved", string(configData))
	}

	var savedData map[string]string
	readYAMLFile(t, dataPath, &savedData)
	if savedData["saved"] != "again" {
		t.Fatalf("saved sidecar = %#v, want updated value", savedData)
	}
}

func TestSaveGenericWritesDefaultFileRefWhenConfigDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dataPath := filepath.Join(dir, "data.json")

	cfg := &fileRefConfig{
		Name: "app",
		Data: map[string]string{"hello": "world"},
	}

	err := SaveGeneric(cfg, WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("SaveGeneric() error = %v", err)
	}

	var savedMain map[string]any
	readJSONFile(t, configPath, &savedMain)
	if savedMain["data"] != "file://data.json" {
		t.Fatalf("data ref = %#v, want file://data.json", savedMain["data"])
	}

	var savedData map[string]string
	readJSONFile(t, dataPath, &savedData)
	if savedData["hello"] != "world" {
		t.Fatalf("saved sidecar = %#v, want written default file ref target", savedData)
	}
}

func TestLoadGenericFileRefUsesZeroValueForEmptySidecar(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	securityPath := filepath.Join(dir, ".security.yml")
	dataPath := filepath.Join(dir, "data.json")

	if err := os.WriteFile(configPath, []byte(`{"name":"app","data":"file://data.json"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(dataPath, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	loader, err := NewLoader(WithConfigPath(configPath), WithSecurityPath(securityPath), WithEnvironment(false))
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}

	cfg := &fileRefConfig{}
	err = loader.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}
	if cfg.Data != nil {
		t.Fatalf("Data = %#v, want nil zero value", cfg.Data)
	}
}

func TestResolveFileReferencePathRejectsInvalidReference(t *testing.T) {
	_, err := resolveFileReferencePath(filepath.Join(t.TempDir(), "config.json"), "file://", DefaultPathResolver())
	if err == nil {
		t.Fatal("resolveFileReferencePath() error = nil, want invalid reference error")
	}
}

func readYAMLFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}
