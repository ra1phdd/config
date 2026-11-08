<div align="center">

[English](../en/DECODING.md) | **Русский**

</div>

# Декодирование

`config.Load` применяет defaults, загружает источники, объединяет normalized data, декодирует свежий объект, проверяет required fields и запускает validators.

```go
type HTTP struct {
	Address string        `config:"address,required" env:"HTTP_ADDRESS"`
	Timeout time.Duration `config:"timeout"`
}
```

Без имени в `config` используется первая буква Go-поля в нижнем регистре. `config:"-"` пропускает поле. `required` отклоняет missing или nil; явно заданное нулевое значение допустимо.

JSON и YAML сначала используют `config` tag, затем соответствующий `json` или `yaml` tag и, наконец, имя поля по умолчанию. Если заданы несколько имён поля, приоритет у `config`, затем у форматного tag, затем у имени по умолчанию. Поле с `json:"-"` или `yaml:"-"` игнорируется в соответствующем формате, если для него явно не задан `config` tag. Для ENV каноническое имя строится из полного config-пути в верхнем регистре с `_` между частями (`http.address` становится `HTTP_ADDRESS`). Явный `env` tag является fallback alias и проверяется, только если каноническая переменная отсутствует. Если заданы обе, выигрывает config-derived имя. `env` tag на nested struct означает, что вся структура читается как одно ENV-значение.

Map и object объединяются рекурсивно. Scalar, arrays и slices заменяются целиком. Отсутствующие ключи сохраняют предыдущее значение. `null` очищает pointers, maps, slices и interfaces, но недопустим для обычных scalar. Пустая строка, `false`, `0`, пустые map и slice считаются явно заданными.

Поддерживаются pointers, nested structs, maps, slices, arrays, named scalar types, `time.Duration`, URL, IP, `encoding.TextUnmarshaler`, format-specific decoders и loader-local `config.WithDecoder`.

Для проверки структуры по тегам подключите опциональный пакет `validator`:

```go
engine := validator.New()
config.Load[AppConfig](ctx,
	config.WithSource(source),
	validator.With[AppConfig](engine),
)
```

Адаптер использует `go-playground/validator`, применяет `validate:"..."`, поддерживает custom validations и передаёт контекст загрузчика в `StructCtx`. Встроенные проверки required-полей и strict-источников остаются независимыми от этой интеграции.

Дополнительные value packages могут реализовать `config.ValueSetter`. Core вызывает этот контракт и не импортирует пакет, владеющий конкретным типом.

Strict включается через `source.Strict` или `StrictFile`/`StrictBytes` формата. Required checks и validators запускаются только после успешной загрузки и decode всех источников.

Независимые ошибки возвращаются как `config.Errors`. `FieldError` содержит source, canonical field path, operation и wrapped cause, не раскрывая secret values.
