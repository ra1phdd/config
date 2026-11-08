# config v2

Типизированная библиотека конфигурации для Go-сервисов. Версия v2 спроектирована заново: она не совместима с API v1, не изменяет `os.Environ`, применяет источники в явном порядке и возвращает только полностью загруженные снимки.

Старая реализация и её тесты сохранены в [`legacy/`](legacy/). Новая библиотека не использует внешние ENV-библиотеки.

## Возможности

- Независимые источники JSON, YAML и переменных окружения/файлов окружения.
- JSON-источники используют реализацию `encoding/json/v2` из Go 1.27.
- Формат разбирается отдельным пакетом; ядро не импортирует парсеры и принимает пользовательский `config.Parser`.
- Defaults, последовательное переопределение и рекурсивное объединение map.
- Поддержка вложенных структур, указателей, массивов, слайсов, map и именованных типов.
- `time.Duration`, URL, IP и `encoding.TextUnmarshaler`.
- Необязательный источник секретов и редактируемый в диагностике `Secret[T]`.
- Hot reload с debounce, отслеживанием обычной записи и atomic rename, last-good snapshot.
- Ошибки с source/path и агрегированием независимых ошибок.

## Установка

```bash
go get github.com/ra1phdd/config
```

## Быстрый старт

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
	Address string                  `config:"address,required" env:"APP_ADDRESS"`
	Timeout time.Duration           `config:"timeout" env:"APP_TIMEOUT"`
	Token   secret.Secret[string]   `config:"token" env:"APP_TOKEN"`
}

func main() {
	cfg, err := config.Load[AppConfig](context.Background(),
		config.WithDefaults(func() AppConfig {
			return AppConfig{Address: ":8080", Timeout: 5 * time.Second}
		}),
		config.WithSource(json.File("config")),
		config.WithSource(env.Process()),
	)
	if err != nil {
		panic(err)
	}
	_ = cfg
}
```

Подключайте только используемые пакеты источников. Их конструкторы выбирают формат независимо от расширения файла. Пользовательские парсеры реализуют `config.Parser` и передаются в `source.File` или `source.Bytes` из нейтрального пакета `source`. ENV API состоит из `env.Process()` и `env.File(path)`.

```go
config.WithSource(yaml.File("application.conf"))
config.WithOptionalSource(yaml.File(".security"))
config.WithSource(env.File(".env.local"))
```

Источники применяются слева направо. Поздний источник имеет больший приоритет. Security-файл не является обязательной частью API: подключайте его только через `WithOptionalSource`, если сервису нужны отдельные секреты.

## Переменные окружения

Имя ENV по умолчанию строится из полного `config`-пути в верхнем регистре: `http.address` становится `HTTP_ADDRESS`. Явный `env` tag задаёт запасное имя, проверяемое, если каноническая переменная отсутствует:

```go
type HTTP struct {
	Address string `config:"address" env:"SERVICE_HTTP_ADDRESS"`
}

type App struct {
	HTTP HTTP `config:"http"`
}

// WithSource(env.Process()) reads HTTP_ADDRESS, then SERVICE_HTTP_ADDRESS if needed.
```

Для тестов задавайте переменные через `t.Setenv`; production API не содержит in-memory source:

```go
t.Setenv("HTTP_ADDRESS", ":8080")
```

## Валидация и ошибки

Обязательные поля помечаются `config:",required"`. Пользовательская cross-field проверка выполняется после загрузки всех источников:

```go
func validate(ctx context.Context, cfg AppConfig) error {
	if cfg.Address == "" {
		return errors.New("address must not be empty")
	}
	return nil
}

config.WithValidator[AppConfig](validate)
```

`FieldError` содержит `Source`, `Path`, `Op` и обёрнутую причину. `Errors` агрегирует независимые ошибки и совместим с `errors.Is`/`errors.As`. Значения секретов не включаются в текст ошибок и форматирование.

Для стандартной проверки по `validate:"..."` используйте адаптер `validator`:

```go
engine := validator.New()
config.Load[AppConfig](ctx,
	config.WithSource(source),
	validator.With[AppConfig](engine),
)
```

Он использует `go-playground/validator`, поддерживает custom validations и передаёт контекст через `StructCtx`. Проверки required-полей и strict-источников остаются частью ядра.

## Hot reload

Наблюдатель находится в отдельном пакете `reload`:

```go
r, err := reload.New(loader,
	reload.WithWatchFile("config"),
	reload.WithWatchFile(".security"),
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
		continue // прежний valid snapshot остаётся активным
	}
	apply(update.Config)
}
```

Каждое изменение вызывает полную сборку конфигурации. Некорректный файл публикуется как `Update.Err`, а `Current()` продолжает возвращать последний успешный снимок. Пересоздание пулов, listener'ов и клиентов остаётся ответственностью приложения.

Для process environment и удалённых adapters используйте `r.Reload(ctx)` после `Start`: он пересобирает snapshot без ожидания изменения файла.

`Start` вызывается один раз. `Close` идемпотентен и завершает lifecycle навсегда; после него `Start` и `Reload` возвращают `reload.ErrClosed`. Для другого способа обнаружения изменений передайте `reload.WithTrigger`.

Снимки глубоко копируют экспортированные reference-поля. Типы со скрытым изменяемым состоянием должны реализовать `config.SnapshotCloner` и вернуть независимую копию.

## Документация

## Документация

- [Примеры](docs/ru/EXAMPLES.md)
- [Источники](docs/ru/SOURCES.md)
- [Декодирование](docs/ru/DECODING.md)
- [Reload](docs/ru/RELOAD.md)
- [Секреты](docs/ru/SECRETS.md)
- [Архитектура](docs/ru/ARCHITECTURE.md)

Английские версии находятся в [`docs/en/`](docs/en/).
