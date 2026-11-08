package env

import (
	"context"
	"os"
	"testing"

	"github.com/ra1phdd/config/v2"
)

func TestProcessLoadsTaggedEnvironment(t *testing.T) {
	t.Setenv("SERVICE_NAME", "demo")
	type target struct {
		Name string `config:"name" env:"SERVICE_NAME"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(Process()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "demo" {
		t.Fatalf("name = %q", cfg.Name)
	}
}

func TestFileLoadsTaggedEnvironment(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/.env"
	if err := os.WriteFile(path, []byte("SERVICE_NAME=demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type target struct {
		Name string `config:"name" env:"SERVICE_NAME"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(File(path)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "demo" {
		t.Fatalf("name = %q", cfg.Name)
	}
}

func TestLoadForRejectsNilTarget(t *testing.T) {
	t.Parallel()
	source := Process()
	targetSource, ok := source.(config.TargetSource)
	if !ok {
		t.Fatal("environment source does not implement config.TargetSource")
	}
	if _, err := targetSource.LoadFor(context.Background(), nil); err == nil {
		t.Fatal("LoadFor() accepted a nil target type")
	}
}
