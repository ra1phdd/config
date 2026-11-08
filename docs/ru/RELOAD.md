<div align="center">

[English](../en/RELOAD.md) | **Русский**

</div>

# Reload

Reload необязателен. Импортируйте `github.com/ra1phdd/config/reload` только для live updates.

```go
r, err := reload.New(loader,
	reload.WithWatchFile("config"),
	reload.WithDebounce(100*time.Millisecond),
)
if err != nil { return err }

updates, err := r.Start(ctx)
if err != nil { return err }
defer r.Close()

for update := range updates {
	if update.Err != nil { continue }
	apply(update.Config)
}
```

`Start` загружает initial snapshot и запускает trigger. Он вызывается один раз. `Current` возвращает deep copy последнего valid snapshot. `Reload(ctx)` немедленно пересобирает snapshot. `Close` идемпотентен, terminal и дожидается остановки watcher.

File watcher поддерживает обычную запись и atomic replacement, опрашивает metadata, считает hash после изменения и debounce-ит burst. Ошибка загрузки приходит в `Update.Err` и не заменяет last-good snapshot.

Другую стратегию можно подключить через `reload.Trigger`:

```go
type Trigger interface {
	Start(context.Context) (<-chan error, error)
}
```

Trigger отправляет `nil` при изменении, error при ошибке обнаружения и закрывает channel после завершения context. Используйте либо `reload.WithTrigger`, либо watched files. Пересборка clients, pools, listeners и workers остаётся ответственностью приложения.
