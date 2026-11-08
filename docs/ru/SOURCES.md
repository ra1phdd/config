<div align="center">

[English](../en/SOURCES.md) | **Русский**

</div>

# Источники

Источники реализуют `config.Source` и применяются в порядке объявления. Поздние источники переопределяют ранние.

Нейтральный пакет `source` предоставляет generic adapters:

```go
source.Values("defaults", map[string]any{"port": 8080})
source.Bytes("plugin", data, parser)
source.File("settings", path, parser)
source.Strict(source)
```

Разбор форматов находится в независимых пакетах:

```go
json.File("config")
yaml.Bytes(data)
env.Process()
env.File(".env.local")
```

Расширение пути не анализируется. `source.File` читает bytes и передаёт их в `config.Parser`:

```go
type Parser interface {
	Parse([]byte) (config.Layer, error)
}
```

Источник может вернуть `config.ErrNotFound`; используйте `config.WithOptionalSource`, если отсутствие допустимо. `config.Layer.Strict` включает проверку неизвестных полей. Ошибки источников содержат `FieldError.Source`, `Path`, `Op` и сохраняют причину для `errors.Is`/`errors.As`.

ENV-источники не изменяют `os.Environ`. `env.Process` читает явные `env` tags из окружения процесса. `env.File` разбирает KEY=value-файл с теми же target-aware tags. В тестах используйте `t.Setenv` или временный env-файл.
