// Package validator integrates go-playground/validator with config loaders.
package validator

import (
	"context"
	"errors"
	"fmt"

	playground "github.com/go-playground/validator/v10"
	"github.com/ra1phdd/config"
)

// Validate is the go-playground validator engine used by this adapter.
type Validate = playground.Validate

// Option configures a validator engine before it is created.
type Option = playground.Option

// New creates a go-playground validator engine.
//
// The returned engine is safe for concurrent use and can be reused by multiple
// loaders. Register custom validations on it before passing it to For.
func New(options ...Option) *Validate {
	return playground.New(options...)
}

// For adapts a go-playground validator engine to config.WithValidator.
// StructCtx receives the loader context, so custom validations can observe
// cancellation and request-scoped values.
func For[T any](engine *Validate) config.Validator[T] {
	return func(ctx context.Context, value T) error {
		if engine == nil {
			return errors.New("nil validator engine")
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("checking validator context: %w", err)
		}
		return engine.StructCtx(ctx, value)
	}
}

// With returns a config option that validates T with the supplied engine.
// It is shorthand for config.WithValidator(For[T](engine)).
func With[T any](engine *Validate) config.Option {
	return config.WithValidator(For[T](engine))
}
