package validator

import (
	"context"
	"errors"
	"testing"

	playground "github.com/go-playground/validator/v10"
	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/source"
)

func TestForValidatesConfigUsingStructTags(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `config:"name" validate:"required,min=3"`
	}
	_, err := config.Load[target](context.Background(),
		config.WithSource(source.Values("test", map[string]any{"name": "ab"})),
		config.WithValidator(For[target](New())),
	)
	if err == nil {
		t.Fatal("Load() accepted a value rejected by validate tags")
	}
	var validationErrs playground.ValidationErrors
	if !errors.As(err, &validationErrs) || len(validationErrs) != 1 || validationErrs[0].Tag() != "min" {
		t.Fatalf("validation error = %v", err)
	}
}

func TestForAcceptsValidConfig(t *testing.T) {
	t.Parallel()
	type target struct {
		Title string `config:"title" validate:"required,min=3"`
	}
	got, err := config.Load[target](context.Background(),
		config.WithSource(source.Values("test", map[string]any{"title": "valid"})),
		config.WithValidator(For[target](New())),
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Title != "valid" {
		t.Fatalf("config = %+v", got)
	}
}

func TestWithAddsValidatorOption(t *testing.T) {
	t.Parallel()
	type target struct {
		Title string `config:"title" validate:"required,min=3"`
	}
	_, err := config.Load[target](context.Background(),
		config.WithSource(source.Values("test", map[string]any{"title": "ab"})),
		With[target](New()),
	)
	if err == nil {
		t.Fatal("Load() accepted a value rejected by validate tags")
	}
}

func TestForRejectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	type target struct{}
	err := For[target](New())(ctx, target{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestForReportsNilEngine(t *testing.T) {
	t.Parallel()
	type target struct{}
	err := For[target](nil)(context.Background(), target{})
	if err == nil || err.Error() != "nil validator engine" {
		t.Fatalf("error = %v", err)
	}
}

func TestForUsesCustomValidationRegisteredOnEngine(t *testing.T) {
	t.Parallel()
	type target struct {
		Mode string `config:"mode" validate:"safe_mode"`
	}
	engine := New()
	if err := engine.RegisterValidation("safe_mode", func(_ playground.FieldLevel) bool {
		return false
	}); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load[target](context.Background(),
		config.WithSource(source.Values("test", map[string]any{"mode": "unsafe"})),
		config.WithValidator(For[target](engine)),
	)
	if err == nil {
		t.Fatal("Load() accepted a value rejected by a custom validation")
	}
}
