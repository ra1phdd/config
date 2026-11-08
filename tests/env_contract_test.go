package config_test

import (
	"context"
	"os"
	"testing"

	"github.com/ra1phdd/config/v2"
	"github.com/ra1phdd/config/v2/source/env"
)

func TestEnvironmentFileContract(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/.env"
	if err := os.WriteFile(path, []byte("APP_NAME=service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type target struct {
		Name string `config:"name" env:"APP_NAME"`
	}
	if _, err := config.Load[target](context.Background(), config.WithSource(env.File(path))); err != nil {
		t.Fatal(err)
	}
}

func TestProcessPrefersUppercaseConfigPathThenFallsBackToEnvTag(t *testing.T) {
	type target struct {
		Logging string `config:"logging" env:"APP_LOGGING_FALLBACK_7F31"`
	}
	t.Setenv("LOGGING", "canonical")
	t.Setenv("APP_LOGGING_FALLBACK_7F31", "fallback")
	cfg, err := config.Load[target](context.Background(), config.WithSource(env.Process()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Logging != "canonical" {
		t.Fatalf("Logging = %q, want canonical config key", cfg.Logging)
	}
}

func TestProcessFallsBackToExplicitEnvTag(t *testing.T) {
	type target struct {
		Logging string `config:"logging" env:"APP_LOGGING_FALLBACK_91C2"`
	}
	t.Setenv("APP_LOGGING_FALLBACK_91C2", "fallback")
	cfg, err := config.Load[target](context.Background(), config.WithSource(env.Process()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Logging != "fallback" {
		t.Fatalf("Logging = %q, want fallback env key", cfg.Logging)
	}
}

func TestProcessBuildsUppercaseNestedConfigPath(t *testing.T) {
	type target struct {
		HTTP struct {
			Address string `config:"address"`
		} `config:"http"`
	}
	t.Setenv("HTTP_ADDRESS", ":9191")
	cfg, err := config.Load[target](context.Background(), config.WithSource(env.Process()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Address != ":9191" {
		t.Fatalf("HTTP.Address = %q", cfg.HTTP.Address)
	}
}

func TestProcessResolvesSiblingEnvFallbacksIndependently(t *testing.T) {
	type target struct {
		HTTP struct {
			Address string `config:"address" env:"SERVICE_ADDRESS_5A20"`
			Port    int    `config:"port" env:"SERVICE_PORT_5A20"`
		} `config:"http"`
	}
	t.Setenv("SERVICE_ADDRESS_5A20", ":8181")
	t.Setenv("SERVICE_PORT_5A20", "8182")
	cfg, err := config.Load[target](context.Background(), config.WithSource(env.Process()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.Address != ":8181" || cfg.HTTP.Port != 8182 {
		t.Fatalf("HTTP = %+v", cfg.HTTP)
	}
}

func TestEnvironmentFileAcceptsConfigKeyBeforeEnvFallback(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/env.conf"
	if err := os.WriteFile(path, []byte("LOGGING=canonical\nAPP_LOGGING_FALLBACK_2AD0=fallback\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	type target struct {
		Logging string `config:"logging" env:"APP_LOGGING_FALLBACK_2AD0"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(env.File(path)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Logging != "canonical" {
		t.Fatalf("Logging = %q, want canonical config key", cfg.Logging)
	}
}
