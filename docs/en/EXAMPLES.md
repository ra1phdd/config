<div align="center">

**English** | [Русский](../ru/EXAMPLES.md)

</div>

# Examples

## JSON plus environment

```go
loader, err := config.New[AppConfig](
	config.WithSource(json.File("config.json")),
	config.WithSource(env.Process()),
)
cfg, err := loader.Load(ctx)
```

## Deterministic tests

```go
cfg, err := config.Load[AppConfig](ctx,
	config.WithSource(source.Values("defaults", map[string]any{"port": 8080})),
	// Set APP_PORT with the test framework before loading.
	config.WithSource(env.Process()),
)
```

## Custom parser

```go
type lineParser struct{}

func (lineParser) Parse(data []byte) (config.Layer, error) {
	return config.Layer{Data: map[string]any{"name": string(data)}}, nil
}

src := source.Bytes("custom", []byte("service"), lineParser{})
cfg, err := config.Load[struct{ Name string `config:"name"` }](ctx,
	config.WithSource(src),
)
```

## Optional file

```go
loader, err := config.New[AppConfig](
	config.WithSource(yaml.File("config")),
	config.WithOptionalSource(yaml.File(".security")),
)
```

## Reload with a custom trigger

```go
r, err := reload.New(loader, reload.WithTrigger(trigger))
updates, err := r.Start(ctx)
defer r.Close()
for update := range updates {
	if update.Err == nil { apply(update.Config) }
}
```
