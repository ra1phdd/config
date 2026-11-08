package reload

import (
	"context"
	"testing"
	"time"

	"github.com/ra1phdd/config"
	genericSource "github.com/ra1phdd/config/source"
)

type immediateTrigger struct{}

func (immediateTrigger) Start(ctx context.Context) (<-chan error, error) {
	events := make(chan error, 1)
	events <- nil
	go func() {
		<-ctx.Done()
		close(events)
	}()
	return events, nil
}

func TestCustomTriggerCanDriveReloadWithoutFileWatcher(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `config:"name"`
	}
	loader, err := config.New[target](config.WithSource(genericSource.Values("values", map[string]any{"name": "service"})))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := New(loader, WithTrigger(immediateTrigger{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	select {
	case update := <-updates:
		if update.Err != nil || update.Config.Name != "service" {
			t.Fatalf("trigger update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("custom trigger did not cause a reload")
	}
}
