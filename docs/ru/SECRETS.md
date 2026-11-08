<div align="center">

[English](../en/SECRETS.md) | **Русский**

</div>

# Секреты

`secret.Secret[T]` — необязательный redacted value type из `github.com/ra1phdd/config/v2/secret`, не зависящий от core.

```go
type Config struct {
	Token secret.Secret[string] `config:"token" env:"APP_TOKEN"`
}

token := cfg.Token.Reveal()
if cfg.Token.IsSet() { /* источник задал значение */ }
```

Форматирование всегда возвращает `[REDACTED]`. JSON тоже выдаёт marker. `Reveal` — явная граница доступа к plaintext. Результат `Reveal` нельзя логировать или форматировать.

Пакет реализует `config.ValueSetter`, поэтому core декодирует normalized values, не импортируя `secret`. Также поддерживаются `encoding.TextUnmarshaler` и deep snapshot cloning.

Библиотека автоматически не шифрует, не сохраняет, не обращается к vault, не разделяет security overlays и не выполняет rotation. Для этого используйте explicit source или application adapter. Optional secret files подключайте через `config.WithOptionalSource`, если отсутствие допустимо.
