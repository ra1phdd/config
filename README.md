<div align="center">
<h1>config</h1>
<p>A Go library for loading application config from JSON, YAML, .env, and environment variables with secure secret overlays, file references, and encrypted credentials.</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://goreportcard.com/badge/github.com/ra1phdd/config" alt="Go report">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

**English** | [Russian](README.ru.md)
</div>

## Features

- Loads config into typed Go structs.
- Supports `.json`, `.yaml`, `.yml`, and `.env` config files.
- Merges defaults, main config, security overlay, `.env`, and process environment.
- Keeps `SecureString` values out of the main config file when saving.
- Stores secrets in a separate YAML security overlay.
- Supports `file://` references for secret values and sidecar config files.
- Supports `enc://` encrypted secret values.
- Rejects symlinks by default for safer file access.
- Uses atomic writes with `0600` permissions for saved files.

## Install

```bash
go get github.com/ra1phdd/config
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/ra1phdd/config"
)

type AppConfig struct {
	Port   int                 `json:"port" yaml:"port" env:"APP_PORT"`
	Name   string              `json:"name" yaml:"name" env:"APP_NAME"`
	Token  config.SecureString `json:"token,omitzero" yaml:"token,omitempty" env:"APP_TOKEN"`
	Labels map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty" config:"file=labels.yaml"`
}

func defaults() *AppConfig {
	return &AppConfig{
		Port: 8080,
		Name: "app",
	}
}

func main() {
	cfg, err := config.LoadGeneric(
		defaults,
		config.WithConfigPath("config.yaml"),
		config.WithSecurityPath(".security.yml"),
		config.WithDotEnv(".env"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(cfg.Port, cfg.Name, cfg.Token.String())

	err = config.SaveGeneric(
		cfg,
		config.WithConfigPath("config.yaml"),
		config.WithSecurityPath(".security.yml"),
	)
	if err != nil {
		panic(err)
	}
}
```

## Merge Order

Values are applied in this order:

1. Defaults passed to `LoadGeneric`
2. Main config file
3. Security overlay YAML
4. Optional `.env` files
5. Process environment variables

By default, existing process environment variables still win over `.env`. Use `WithDotEnvOverride(true)` if you want `.env` to overwrite already-set environment values before parsing.

## Files

Main config file:

- Supported for loading: `.env`, `.json`, `.yaml`, `.yml`
- Supported for saving: `.json`, `.yaml`, `.yml`

Security file:

- YAML overlay only
- Missing file is allowed
- Contains only security-related values such as `SecureString`

Paths can be passed explicitly or through environment variables:

- `APP_CONFIG`
- `APP_SECURITY`

## Secure Values

Use `config.SecureString` for secrets.

When saving:

- The main config file gets a placeholder instead of the secret value.
- The real secret is written to the security overlay.
- If `CONFIG_KEY_PASSPHRASE` is set and an SSH private key is available, the secret is saved as `enc://...`.

When loading, `SecureString` can resolve:

- Plain text values
- `file://relative/or/absolute/path`
- `enc://base64...`

Encryption-related environment variables:

- `CONFIG_KEY_PASSPHRASE`
- `CONFIG_SSH_KEY_PATH`

If `CONFIG_SSH_KEY_PATH` is not set, the library looks for a default SSH key in `~/.ssh`.

### Merging secrets into struct slices

An item in `.security.yml` is matched against the main configuration using ordinary scalar fields present in the overlay (`name`, `id`, `type`, and so on). A unique match updates protected branches only, preserving all other fields and slice elements. No match appends a new element. Conflicting or ambiguous selectors return an error.

Identity fields can be declared explicitly; multiple fields form a composite key:

```go
type AccountConfig struct {
	Provider string `json:"provider" yaml:"provider" securityMerge:"key"`
	Region   string `json:"region" yaml:"region" securityMerge:"key"`
	Token    config.SecureString `json:"token" yaml:"token,omitempty"`
}
```

Every explicit composite-key component is required in the corresponding `.security.yml` item. Value and pointer slices are supported. An unmatched item is appended instead of being written to an arbitrary position; a fixed array cannot grow and returns an error.

## File References For Structured Data

You can move part of the config into a sidecar file with a struct tag:

```go
type AppConfig struct {
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty" config:"file=data.json"`
}
```

With this tag:

- Loading `data: file://data.json` reads and decodes `data.json` into `Data`.
- Saving keeps the `file://...` reference in the main config file.
- The sidecar file is written automatically.

This works for JSON and YAML sidecar files.

## Options

Common options:

- `WithConfigPath(path)`
- `WithSecurityPath(path)`
- `WithDotEnv(paths...)`
- `WithEnvironment(enabled)`
- `WithDotEnvEnabled(enabled)`
- `WithDotEnvOverride(enabled)`
- `WithStrictJSON(enabled)`
- `WithValidation(enabled)`
- `WithMissingConfigAllowed(allowed)`
- `WithEmptyConfigAllowed(allowed)`
- `WithSymlinksAllowed(allowed)`
- `WithPathResolver(resolver)`

If your config struct implements:

```go
type ConfigValidator interface {
	Validate() error
}
```

validation runs automatically on load and save unless disabled.

## Path Resolution

The built-in path resolver supports markers in paths:

- `{HOME}`: current user's home directory
- `{PWD}`: current working directory
- `{CWD}`: alias for the current working directory
- `{TMP}`: system temporary directory
- `{TEMP}`: alias for the system temporary directory

## Notes

- Config and security paths are required. If neither explicit paths nor path environment variables are set, loader construction panics.
- Symlinks are rejected unless `WithSymlinksAllowed(true)` is used.
- Saved files are written atomically.
