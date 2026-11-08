<div align="center">

[English](../en/ARCHITECTURE.md) | **Русский**

</div>

# Архитектура

Core владеет контрактами и orchestration. Он не импортирует format parsers, reload или secret implementations.

```text
config
  -> internal/schema
  -> internal/clone
  -> internal/path
  -> internal/merge
  -> internal/validate

source/json, source/yaml, source/env -> contracts config
reload    -> contracts config
secret    -> config.ValueSetter
```

Публичный core содержит `Loader`, `Load`, options, `Source`, `Layer`, parser contracts, decoder contracts, validators и diagnostics. Пакет `source` содержит neutral file/bytes/value adapters. `source/json`, `source/yaml` и `source/env` владеют format-specific поведением. `env` владеет и process environment, и KEY=value-файлами. `reload` владеет lifecycle, triggers, polling, debounce и last-good publication. `secret` владеет redacted values.

Внутренние пакеты разделены по ответственности:

- `schema` кэширует struct metadata и tag options;
- `clone` выполняет cycle-preserving defensive copies;
- `path` работает с canonical field paths и source origins;
- `merge` normalizes и объединяет source maps;
- `validate` реализует strict, required и presence policies.

Composition остаётся в приложении: оно выбирает sources, порядок, optionality, reload strategy и действия после обновления snapshot.
