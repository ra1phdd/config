<div align="center">

**English** | [Русский](../ru/SECRETS.md)

</div>

# Secrets

`secret.Secret[T]` is an optional redacted value type. It is imported from `github.com/ra1phdd/config/secret` and has no dependency on the core package.

```go
type Config struct {
	Token secret.Secret[string] `config:"token" env:"APP_TOKEN"`
}

token := cfg.Token.Reveal()
if cfg.Token.IsSet() { /* a source supplied the value */ }
```

Formatting always returns `[REDACTED]`. JSON marshaling emits the same marker. `Reveal` is the explicit boundary at which application code accesses plaintext. Do not log or format the result of `Reveal`.

The package implements `config.ValueSetter`, so the core can decode normalized source values without importing `secret`. It also supports `encoding.TextUnmarshaler` and deep snapshot cloning.

The library does not automatically encrypt, persist, resolve vault references, split security overlays, or rotate secrets. Use an explicit source or application adapter for those operations. Optional secret files should be connected with `config.WithOptionalSource` when absence is valid.
