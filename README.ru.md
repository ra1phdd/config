<div align="center">
<h1>config</h1>
<p>Go-библиотека для загрузки конфигурации приложения из JSON, YAML, .env и переменных окружения с безопасными оверлеями секретов, файловыми ссылками и шифрованием учетных данных.</p>

<p>
    <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://goreportcard.com/badge/github.com/ra1phdd/config" alt="Go report">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
</p>

[English](README.md) | **Russian**
</div>

## Возможности

- Загружает конфигурацию в типизированные Go-структуры.
- Поддерживает файлы `.json`, `.yaml`, `.yml` и `.env`.
- Объединяет значения по умолчанию, основной конфиг, security overlay, `.env` и переменные окружения.
- Не сохраняет `SecureString` в основном конфиге.
- Хранит секреты в отдельном YAML overlay-файле.
- Поддерживает `file://` ссылки для секретов и sidecar-файлов.
- Поддерживает зашифрованные значения `enc://`.
- По умолчанию запрещает симлинки для более безопасной работы с файлами.
- Использует атомарную запись и права `0600` при сохранении.

## Установка

```bash
go get github.com/ra1phdd/config
```

## Быстрый старт

```go
package main

import (
	"fmt"

	"github.com/ra1phdd/config"
)

type AppConfig struct {
	Port   int                 `json:"port" yaml:"port" env:"APP_PORT"`
	Name   string              `json:"name" yaml:"name" env:"APP_NAME"`
	Token  config.SecureString `json:"token,omitzero" yaml:"token,omitempty" env:"APP_TOKEN"`
	Labels map[string]string   `json:"labels,omitempty" yaml:"labels,omitempty" config:"file=labels.yaml"`
}

func defaults() *AppConfig {
	return &AppConfig{
		Port: 8080,
		Name: "app",
	}
}

func main() {
	cfg, err := config.LoadGeneric(
		defaults,
		config.WithConfigPath("config.yaml"),
		config.WithSecurityPath(".security.yml"),
		config.WithDotEnv(".env"),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(cfg.Port, cfg.Name, cfg.Token.String())

	err = config.SaveGeneric(
		cfg,
		config.WithConfigPath("config.yaml"),
		config.WithSecurityPath(".security.yml"),
	)
	if err != nil {
		panic(err)
	}
}
```

## Порядок объединения

Значения применяются в таком порядке:

1. Значения по умолчанию, переданные в `LoadGeneric`
2. Основной конфиг-файл
3. YAML-файл security overlay
4. Дополнительные `.env` файлы
5. Переменные окружения процесса

По умолчанию уже установленные переменные окружения имеют приоритет над `.env`. Если нужно, чтобы `.env` перезаписывал уже заданные значения до финального парсинга, используйте `WithDotEnvOverride(true)`.

## Файлы

Основной конфиг:

- Поддерживается для загрузки: `.env`, `.json`, `.yaml`, `.yml`
- Поддерживается для сохранения: `.json`, `.yaml`, `.yml`

Security-файл:

- Только YAML overlay
- Отсутствующий файл допускается
- Содержит только security-значения, например `SecureString`

Пути можно передать явно или через переменные окружения:

- `APP_CONFIG`
- `APP_SECURITY`

## Секреты

Для секретов используйте `config.SecureString`.

При сохранении:

- В основной конфиг записывается заглушка вместо секрета.
- Реальное значение попадает в security overlay.
- Если задан `CONFIG_KEY_PASSPHRASE` и доступен приватный SSH-ключ, секрет сохраняется как `enc://...`.

При загрузке `SecureString` умеет разрешать:

- Обычные строковые значения
- `file://relative/or/absolute/path`
- `enc://base64...`

Переменные окружения для шифрования:

- `CONFIG_KEY_PASSPHRASE`
- `CONFIG_SSH_KEY_PATH`

Если `CONFIG_SSH_KEY_PATH` не задан, библиотека ищет SSH-ключ по умолчанию в `~/.ssh`.

## Файловые ссылки для структурированных данных

Часть конфигурации можно вынести в отдельный sidecar-файл через тег структуры:

```go
type AppConfig struct {
	Data map[string]string `json:"data,omitempty" yaml:"data,omitempty" config:"file=data.json"`
}
```

С таким тегом:

- При загрузке `data: file://data.json` читает и декодирует `data.json` в поле `Data`.
- При сохранении ссылка `file://...` остается в основном конфиге.
- Sidecar-файл записывается автоматически.

Это работает для JSON- и YAML-sidecar-файлов.

## Опции

Основные опции:

- `WithConfigPath(path)`
- `WithSecurityPath(path)`
- `WithDotEnv(paths...)`
- `WithEnvironment(enabled)`
- `WithDotEnvEnabled(enabled)`
- `WithDotEnvOverride(enabled)`
- `WithStrictJSON(enabled)`
- `WithValidation(enabled)`
- `WithMissingConfigAllowed(allowed)`
- `WithEmptyConfigAllowed(allowed)`
- `WithSymlinksAllowed(allowed)`
- `WithPathResolver(resolver)`

Если структура конфига реализует:

```go
type ConfigValidator interface {
	Validate() error
}
```

валидация автоматически запускается при загрузке и сохранении, если ее не отключить.

## Разрешение путей

Встроенный resolver путей поддерживает маркеры:

- `{HOME}`: домашняя директория текущего пользователя
- `{PWD}`: текущая рабочая директория
- `{CWD}`: алиас текущей рабочей директории
- `{TMP}`: системная временная директория
- `{TEMP}`: алиас системной временной директории

## Примечания

- Пути к основному конфигу и security overlay обязательны. Если не передать их явно и не задать через переменные окружения, создание `Loader` завершится panic.
- Симлинки запрещены, если явно не включить `WithSymlinksAllowed(true)`.
- Сохранение выполняется атомарно.

## Лицензия

MIT
