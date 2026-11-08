<div align="center">

[English](../en/EXAMPLES.md) | **Русский**

</div>

# Примеры

## JSON и ENV

```go
loader, err := config.New[AppConfig](
	config.WithSource(json.File("config.json")),
	config.WithSource(env.Process()),
)
cfg, err := loader.Load(ctx)
```

## Детерминированный тест

```go
cfg, err := config.Load[AppConfig](ctx,
	config.WithSource(source.Values("defaults", map[string]any{"port": 8080})),
	// Перед загрузкой задайте APP_PORT через test framework.
	config.WithSource(env.Process()),
)
```

## Пользовательский parser

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

## Reload с custom trigger

```go
r, err := reload.New(loader, reload.WithTrigger(trigger))
updates, err := r.Start(ctx)
defer r.Close()
for update := range updates {
	if update.Err == nil { apply(update.Config) }
}
```
