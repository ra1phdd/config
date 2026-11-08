<div align="center">

**English** | [Русский](../ru/DECODING.md)

</div>

# Decoding

`config.Load` applies defaults, loads sources, merges normalized data, decodes into a fresh value, checks required fields, and then runs validators.

`config` tags select field names:

```go
type HTTP struct {
	Address string        `config:"address,required" env:"HTTP_ADDRESS"`
	Timeout time.Duration `config:"timeout"`
}
```

An omitted `config` name uses the lower-cased first letter of the Go field name. `config:"-"` skips a field. The `required` option rejects missing or nil values; an explicitly supplied zero value is valid.

JSON and YAML sources first use the `config` tag, then fall back to the matching `json` or `yaml` tag, and finally to the default config field name. If multiple names for a field are present, the `config` name wins over the format tag, which wins over the default name. A `json:"-"` or `yaml:"-"` field is ignored by that format unless it has an explicit `config` tag. For environment sources, the canonical variable name is the complete config path uppercased and joined with underscores (`http.address` becomes `HTTP_ADDRESS`). An explicit `env` tag is a fallback alias, checked only when the canonical name is absent. If both variables exist, the canonical config-derived name wins. An `env` tag on a nested struct keeps that struct as one environment-valued field instead of recursing.

Sources merge recursively for maps and objects. Scalars, arrays, and slices replace the previous value. Missing keys preserve the previous layer. `null` clears pointers, maps, slices, and interfaces; it is invalid for non-nullable scalar fields. Empty strings, false, zero, empty maps, and empty slices are explicit values.

The decoder supports pointers, nested structs, maps, slices, arrays, named scalar types, `time.Duration`, URLs, IP addresses, `encoding.TextUnmarshaler`, format-specific decoders, and loader-local exact-type decoders:

```go
config.WithDecoder[MyType](func(input any) (MyType, error) {
	// Convert one normalized source value.
})
```

For tag-based struct validation, import the optional `validator` package and register a reusable go-playground engine:

```go
engine := validator.New()
config.Load[AppConfig](ctx,
	config.WithSource(source),
	validator.With[AppConfig](engine),
)
```

The adapter uses the official [`go-playground/validator`](https://github.com/go-playground/validator), applies `validate:"..."` tags, supports custom validations, and receives the loader context through `StructCtx`. The core keeps its required-field and strict-source policies independent from this optional integration.

Optional value packages can implement `config.ValueSetter`. The core calls that contract without importing the package that owns the value type.

Strict validation is enabled per source with `source.Strict` or a format package's `StrictFile`/`StrictBytes`. Required checks and validators run only after all sources have loaded and decoding has completed.

Independent failures are returned as `config.Errors`. A `config.FieldError` identifies the source, canonical field path, operation, and wrapped cause without including secret values.
