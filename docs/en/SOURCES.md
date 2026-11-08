<div align="center">

**English** | [Русский](../ru/SOURCES.md)

</div>

# Sources

Configuration sources implement `config.Source` and are applied in declaration order. Later sources override earlier values.

The format-neutral package provides the generic adapters:

```go
source.Values("defaults", map[string]any{"port": 8080})
source.Bytes("plugin", data, parser)
source.File("settings", path, parser)
source.Strict(source)
```

Format packages own parsing and can be imported independently:

```go
json.File("config")
yaml.Bytes(data)
env.Process()
env.File(".env.local")
```

The path extension is never inspected. `source.File` reads bytes and delegates parsing to the supplied `config.Parser`. A custom parser only needs:

```go
type Parser interface {
	Parse([]byte) (config.Layer, error)
}
```

Sources may return `config.ErrNotFound`; use `config.WithOptionalSource` when that absence is allowed. `config.Layer.Strict` requests unknown-field validation. Source errors expose `FieldError.Source`, `FieldError.Path`, `FieldError.Op`, and preserve the underlying error for `errors.Is` and `errors.As`.

Environment sources do not mutate `os.Environ`. `env.Process` reads explicit `env` tags from the process environment. `env.File` parses KEY=value files using the same target-aware tags. Tests should use `t.Setenv` or a temporary environment file.

Use `config.WithSource` for required layers and `config.WithOptionalSource` for optional files. Keep source ordering explicit in the composition root.
