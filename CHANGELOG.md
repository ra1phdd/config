# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Environment sources now prefer uppercase config paths and use explicit `env` tags as fallback aliases.
- JSON and YAML sources now fall back to their format-specific struct tags after `config` tags.

- Moved JSON, YAML, and environment parsing into independently imported source packages; the core now accepts custom parsers through a public contract.
- Moved reload implementation and lifecycle errors into the optional `reload` package, with pluggable change triggers.
- Moved format-neutral source construction into `source`, shared reflection metadata into `internal/schema`, and cycle-preserving cloning into `internal/clone`.
- Moved redacted `Secret[T]` values into the optional `secret` package behind the core `ValueSetter` contract.
- Moved path operations, normalized map merging, and strict/required/presence validation into independent internal packages.

### Added

- Added the optional `validator` package integrating `go-playground/validator` through `validator.New`, `validator.With[T]`, and `validator.For[T]`.
- Rebuilt the configuration runtime as `config v2` with explicit JSON, YAML, environment-file, and process-environment sources, typed decoding, aggregated diagnostics, optional secrets, and file-backed hot reload.
- Moved the v1 implementation and tests under `legacy/` with isolated dependencies.
- Added explicit `Reloader.Reload(ctx)` for process-environment and other non-file sources.
