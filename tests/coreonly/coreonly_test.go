package coreonly

import (
	"context"
	"testing"

	"github.com/ra1phdd/config"
	genericSource "github.com/ra1phdd/config/source"
)

type parser struct{}

func (parser) Parse(data []byte) (config.Layer, error) {
	return config.Layer{Data: map[string]any{"name": string(data)}}, nil
}

func TestCustomParserUsesCoreWithoutBuiltInExtensions(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `config:"name"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(
		genericSource.Bytes([]byte("from-plugin"), parser{}, "custom"),
	))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "from-plugin" {
		t.Fatalf("Name = %q, want from-plugin", cfg.Name)
	}
}

func TestStrictSourceKeepsNilSourceInvalid(t *testing.T) {
	t.Parallel()
	type target struct{}
	if _, err := config.New[target](config.WithSource(genericSource.Strict(nil))); err == nil {
		t.Fatal("New() accepted a strict wrapper around a nil source")
	}
}
