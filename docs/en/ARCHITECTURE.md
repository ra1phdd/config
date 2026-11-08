<div align="center">

**English** | [Русский](../ru/ARCHITECTURE.md)

</div>

# Architecture

The core owns contracts and orchestration. It does not import format parsers, reload, or secret implementations.

```text
config
  -> internal/schema
  -> internal/clone
  -> internal/path
  -> internal/merge
  -> internal/validate

source/json, source/yaml, source/env -> config contracts
reload    -> config contracts
secret    -> config.ValueSetter
```

The public core contains `Loader`, `Load`, options, `Source`, `Layer`, parser contracts, decoder contracts, validators, and public diagnostics. `source` contains format-neutral file/bytes/value adapters. `source/json`, `source/yaml`, and `source/env` own format-specific behavior. `env` owns both process environment and KEY=value file parsing. `reload` owns lifecycle, triggers, file polling, debounce, and last-good publication. `secret` owns redacted values.

Internal packages have focused responsibilities:

- `schema` caches struct metadata and tag options;
- `clone` performs cycle-preserving defensive copies;
- `path` handles canonical field paths and source-origin lookup;
- `merge` normalizes and combines source maps;
- `validate` implements strict, required, and presence policies.

Composition belongs to the application. The application chooses sources, their order, optionality, reload strategy, and what dependent resources should be rebuilt after an update.
