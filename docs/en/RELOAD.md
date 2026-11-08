<div align="center">

**English** | [Русский](../ru/RELOAD.md)

</div>

# Reload

Reload is optional. Import `github.com/ra1phdd/config/reload` only when an application needs live updates.

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
	if update.Err != nil {
		continue // The last valid snapshot remains current.
	}
	apply(update.Config)
}
```

`Start` loads the initial snapshot and starts the trigger. It may be called once. `Current` returns a deep copy of the last valid snapshot. `Reload(ctx)` rebuilds a snapshot immediately and returns the result directly. `Close` is idempotent, terminal, and waits for watcher shutdown.

File watching handles ordinary writes and atomic replacement, polls metadata, hashes changed files, and debounces bursts. A failed load is published as `Update.Err`; it never replaces the last valid snapshot.

Implement `reload.Trigger` for another change detector:

```go
type Trigger interface {
	Start(context.Context) (<-chan error, error)
}
```

The trigger sends `nil` for a change, an error for detection failure, and closes its channel when the context ends. Configure either `reload.WithTrigger` or watched files, not both. Applications remain responsible for rebuilding dependent clients, pools, listeners, and workers after applying a snapshot.
