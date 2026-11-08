package config_test

import (
	"context"
	"testing"

	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/source/env"
	configjson "github.com/ra1phdd/config/source/json"
	configyaml "github.com/ra1phdd/config/source/yaml"
)

const (
	benchmarkEnvName  = "APP_NAME"
	benchmarkModeSafe = "safe"
)

type benchmarkConfig struct {
	Name    string `config:"name" env:"APP_NAME"`
	Region  string `config:"region" env:"APP_REGION"`
	Enabled bool   `config:"enabled" env:"APP_ENABLED"`
	Count   int    `config:"count" env:"APP_COUNT"`
	Timeout string `config:"timeout" env:"APP_TIMEOUT"`
	Mode    string `config:"mode" env:"APP_MODE"`
	Host    string `config:"host" env:"APP_HOST"`
	Port    int    `config:"port" env:"APP_PORT"`
	Token   string `config:"token" env:"APP_TOKEN"`
	Zone    string `config:"zone" env:"APP_ZONE"`
}

func BenchmarkLoadEnvSmall(b *testing.B) {
	values := map[string]string{
		benchmarkEnvName: "service", "APP_REGION": "us-east", "APP_ENABLED": "true", "APP_COUNT": "3",
		"APP_TIMEOUT": "5s", "APP_MODE": benchmarkModeSafe, "APP_HOST": "127.0.0.1", "APP_PORT": "8080",
		"APP_TOKEN": "redacted", "APP_ZONE": "a",
	}
	for key, value := range values {
		b.Setenv(key, value)
	}
	loader, err := config.New[benchmarkConfig](config.WithSource(env.Process()))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := loader.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadYAMLSmall(b *testing.B) {
	loader, err := config.New[benchmarkConfig](config.WithSource(configyaml.Bytes([]byte("name: service\nregion: us-east\nenabled: true\ncount: 3\ntimeout: 5s\nmode: safe\nhost: 127.0.0.1\nport: 8080\ntoken: redacted\nzone: a\n"))))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := loader.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadJSONSmall(b *testing.B) {
	loader, err := config.New[benchmarkConfig](config.WithSource(configjson.Bytes([]byte(`{"name":"service","region":"us-east","enabled":true,"count":3,"timeout":"5s","mode":"safe","host":"127.0.0.1","port":8080,"token":"redacted","zone":"a"}`))))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := loader.Load(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

//nolint:paralleltest // testing.AllocsPerRun cannot run in parallel.
func TestLoadJSONSmallAllocationBudget(t *testing.T) {
	loader, err := config.New[benchmarkConfig](config.WithSource(configjson.Bytes([]byte(`{"name":"service","region":"us-east","enabled":true,"count":3,"timeout":"5s","mode":"safe","host":"127.0.0.1","port":8080,"token":"redacted","zone":"a"}`))))
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := loader.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if allocs >= 70 {
		t.Fatalf("Load allocated %.0f objects per run; want fewer than 70", allocs)
	}
}

func TestLoadEnvSmallAllocationBudget(t *testing.T) { //nolint:paralleltest // t.Setenv cannot be used with t.Parallel.
	values := map[string]string{
		"APP_NAME": "service", "APP_REGION": "us-east", "APP_ENABLED": "true", "APP_COUNT": "3",
		"APP_TIMEOUT": "5s", "APP_MODE": "safe", "APP_HOST": "127.0.0.1", "APP_PORT": "8080",
		"APP_TOKEN": "redacted", "APP_ZONE": "a",
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	loader, err := config.New[benchmarkConfig](config.WithSource(env.Process()))
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := loader.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if allocs >= 42 {
		t.Fatalf("Load allocated %.0f objects per run; want fewer than 42", allocs)
	}
}

//nolint:paralleltest // testing.AllocsPerRun cannot run in parallel.
func TestLoadYAMLSmallAllocationBudget(t *testing.T) {
	loader, err := config.New[benchmarkConfig](config.WithSource(configyaml.Bytes([]byte("name: service\nregion: us-east\nenabled: true\ncount: 3\ntimeout: 5s\nmode: safe\nhost: 127.0.0.1\nport: 8080\ntoken: redacted\nzone: a\n"))))
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, err := loader.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if allocs >= 210 {
		t.Fatalf("Load allocated %.0f objects per run; want fewer than 210", allocs)
	}
}
