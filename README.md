# config v2

Typed, deterministic configuration loading for Go services. v2 has no compatibility layer with v1; the v1 implementation and tests live in [`legacy/`](legacy/).

## Features

- Optional JSON, YAML, and process-environment/file sources.
- JSON sources use the Go 1.27 `encoding/json/v2` implementation.
- Parsers are independent of the core and can be replaced with a custom `config.Parser`.
- Fresh typed snapshots with defaults, recursive map merge, scalar/slice replacement.
- Custom text types, durations, URLs, IPs, maps, slices, pointers, and redacted `Secret[T]`.
- Optional file reload with debounce and last-good snapshot retention.
- No mutation of `os.Environ`.

## Install

```bash
go get github.com/ra1phdd/config
```

## Quick start

```go
package main

import (
	"context"
	"time"

	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/secret"
	"github.com/ra1phdd/config/source/env"
	"github.com/ra1phdd/config/source/json"
)

type AppConfig struct {
	Address string        `config:"address,required" env:"APP_ADDRESS"`
	Timeout time.Duration `config:"timeout" env:"APP_TIMEOUT"`
	Token   secret.Secret[string] `config:"token" env:"APP_TOKEN"`
}

func main() {
	cfg, err := config.Load[AppConfig](context.Background(),
		config.WithDefaults(func() AppConfig { return AppConfig{Address: ":8080", Timeout: 5 * time.Second} }),
		config.WithSource(json.File("config")),
		config.WithSource(env.Process()),
	)
	if err != nil { panic(err) }
	_ = cfg
}
```

Environment names come from the uppercase `config` path by default (`http.address` becomes `HTTP_ADDRESS`). An explicit `env` tag is checked as a fallback alias. Import only the source packages the application uses. Each parser owns its format handling; the core imports none of them. Custom parsers implement `config.Parser` and can be passed to `source.File` or `source.Bytes` from the format-neutral `source` package. File formats are selected by the source constructor, not the path extension.

For `validate:"..."` struct tags, use the optional `validator` adapter:

```go
engine := validator.New()
config.Load[AppConfig](ctx,
	config.WithSource(source),
	validator.With[AppConfig](engine),
)
```

The adapter uses `go-playground/validator`, including custom validations and `StructCtx` context support. Required-field and strict-source checks remain part of the core loading policy.

For file reloads use the optional `reload` package. It publishes only valid snapshots and keeps the last valid value after an invalid edit:

```go
import (
	"context"
	"time"

	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/reload"
)

func watch(ctx context.Context, loader *config.Loader[AppConfig]) error {
	r, err := reload.New(loader,
		reload.WithWatchFile("config"),
		reload.WithDebounce(100*time.Millisecond),
	)
	if err != nil {
		return err
	}
	updates, err := r.Start(ctx)
	if err != nil {
		return err
	}
	defer r.Close()
	for update := range updates {
		if update.Err != nil {
			continue
		}
		_ = update.Config // apply the snapshot in the application layer
	}
	return nil
}
```

Call `r.Reload(ctx)` when a non-file source such as process environment or a remote adapter changes.

`Start` may be called once. `Close` is idempotent and permanently stops the reloader; calls to `Start` or `Reload` after `Close` return `reload.ErrClosed`. Manual `Reload` returns its `Update` directly and does not send a second value through the file-event channel. Applications can provide another change detector with `reload.WithTrigger`.

Snapshots deep-copy exported reference fields. Types with unexported mutable state should implement `config.SnapshotCloner` to provide an independent copy.

## Documentation

- [Getting examples](docs/en/EXAMPLES.md)
- [Sources](docs/en/SOURCES.md)
- [Decoding](docs/en/DECODING.md)
- [Reload](docs/en/RELOAD.md)
- [Secrets](docs/en/SECRETS.md)
- [Architecture](docs/en/ARCHITECTURE.md)

Russian versions are available in [`docs/ru/`](docs/ru/).
