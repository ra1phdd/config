package yaml

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ra1phdd/config"
	yamlv3 "gopkg.in/yaml.v3"
)

type yamlConfigValue struct {
	Value string
}

func (v *yamlConfigValue) UnmarshalYAML(node *yamlv3.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return fmt.Errorf("decode YAML scalar: %w", err)
	}
	v.Value = "yaml:" + value
	return nil
}

func TestBytesLoadsNestedYAML(t *testing.T) {
	t.Parallel()
	layer, err := Bytes([]byte("http:\n  address: :8080\n")).Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	http, ok := layer.Data["http"].(map[string]any)
	if !ok || http["address"] != ":8080" {
		t.Fatalf("layer = %#v", layer.Data)
	}
}

func TestYAMLTagFallbackAndConfigTagPrecedence(t *testing.T) {
	t.Parallel()
	type target struct {
		Fallback string `yaml:"log_level"`
		Explicit string `config:"logging" yaml:"yaml_logging"`
		Priority string `config:"preferred" yaml:"legacy"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(Bytes([]byte("log_level: debug\nyaml_logging: trace\npreferred: config\nlegacy: format\n"))))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Fallback != "debug" || cfg.Explicit != "trace" || cfg.Priority != "config" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestYAMLTagsNormalizeNestedCollectionsAndRespectSkip(t *testing.T) {
	t.Parallel()
	type nested struct {
		Value   string `yaml:"wire_value"`
		Ignored string `yaml:"-"`
	}
	type target struct {
		Nested nested   `yaml:"nested_block"`
		Items  []nested `yaml:"records"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(Bytes([]byte(
		"nested_block:\n  wire_value: one\n  Ignored: hidden\nrecords:\n  - wire_value: two\n",
	))))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Nested.Value != "one" || cfg.Nested.Ignored != "" || len(cfg.Items) != 1 || cfg.Items[0].Value != "two" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestFileErrorsCarrySourceAndPath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.conf")
	_, err := File(path, "settings").Load(context.Background())
	var fieldErr *config.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Source != "settings" || fieldErr.Path != path || !errors.Is(err, config.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}

	bad := filepath.Join(t.TempDir(), "bad.conf")
	if err := os.WriteFile(bad, []byte("value: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = File(bad, "bad").Load(context.Background())
	if !errors.As(err, &fieldErr) || fieldErr.Source != "bad" || fieldErr.Path != bad || fieldErr.Op != "parse" || !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("parse error = %v", err)
	}
}

func TestRejectsTrailingYAMLDocument(t *testing.T) {
	t.Parallel()
	_, err := Bytes([]byte("name: one\n---\nname: two\n")).Load(context.Background())
	var fieldErr *config.FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Op != "parse" || !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func TestStrictBytesRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `config:"name"`
	}
	_, err := config.Load[target](context.Background(), config.WithSource(
		StrictBytes([]byte("name: one\nunknown: true\n")),
	))
	if err == nil {
		t.Fatal("strict YAML source accepted an unknown field")
	}
}

func TestBytesUsesYAMLSpecificUnmarshaler(t *testing.T) {
	t.Parallel()
	type yamlConfig struct {
		Value yamlConfigValue `config:"value"`
	}
	cfg, err := config.Load[yamlConfig](context.Background(), config.WithSource(
		Bytes([]byte("value: custom\n")),
	))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Value.Value != "yaml:custom" {
		t.Fatalf("custom YAML value = %q", cfg.Value.Value)
	}
}
