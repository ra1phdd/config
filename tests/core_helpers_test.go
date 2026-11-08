package config_test

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/ra1phdd/config/v2"
	fieldenv "github.com/ra1phdd/config/v2/internal/envmap"
	"github.com/ra1phdd/config/v2/reload"
	"github.com/ra1phdd/config/v2/secret"
	genericSource "github.com/ra1phdd/config/v2/source"
	"github.com/ra1phdd/config/v2/source/env"
	"github.com/ra1phdd/config/v2/source/json"
	"github.com/ra1phdd/config/v2/source/yaml"
)

type (
	Source           = config.Source
	Layer            = config.Layer
	FieldError       = config.FieldError
	Errors           = config.Errors //nolint:errname // Preserve the core's aggregate type in tests.
	Option           = config.Option
	Validator[T any] = config.Validator[T]
	SnapshotCloner   = config.SnapshotCloner
	Secret[T any]    = secret.Secret[T]
	Update[T any]    = reload.Update[T]
	Reloader[T any]  = reload.Reloader[T]
)

var (
	ErrInvalid            = config.ErrInvalid
	ErrNotFound           = config.ErrNotFound
	ErrReloaderClosed     = reload.ErrClosed
	ErrReloaderNotStarted = reload.ErrNotStarted
)

const (
	FormatJSON = iota + 1
	FormatEnvFile
	FormatYAML
)

func WithSource(source Source) Option                { return config.WithSource(source) }
func WithOptionalSource(source Source) Option        { return config.WithOptionalSource(source) }
func WithDefaults[T any](fn func() T) Option         { return config.WithDefaults(fn) }
func WithValidator[T any](fn Validator[T]) Option    { return config.WithValidator(fn) }
func WithDecoder[T any](fn config.Decoder[T]) Option { return config.WithDecoder(fn) }
func Load[T any](ctx context.Context, opts ...Option) (T, error) {
	return config.Load[T](ctx, opts...)
}
func New[T any](opts ...Option) (*config.Loader[T], error) { return config.New[T](opts...) }
func Values(name string, values map[string]any) Source     { return genericSource.Values(name, values) }

func StrictBytes(name string, data []byte, format int) Source {
	switch format {
	case FormatJSON:
		return json.StrictBytes(data, name)
	case FormatEnvFile:
		return genericSource.Strict(genericSource.Bytes(data, env.Parser{}, name))
	case FormatYAML:
		return yaml.StrictBytes(data, name)
	default:
		panic("unsupported test source format")
	}
}

func StrictFile(name, path string, format int) Source {
	switch format {
	case FormatJSON:
		return json.StrictFile(name, path)
	case FormatEnvFile:
		return genericSource.Strict(genericSource.File(path, env.Parser{}, name))
	case FormatYAML:
		return yaml.StrictFile(name, path)
	default:
		panic("unsupported test source format")
	}
}

func Bytes(name string, data []byte, format int) Source {
	switch format {
	case FormatJSON:
		return json.Bytes(data, name)
	case FormatEnvFile:
		return genericSource.Bytes(data, env.Parser{}, name)
	case FormatYAML:
		return yaml.Bytes(data, name)
	default:
		panic("unsupported test source format")
	}
}

func File(name, path string, format int) Source {
	switch format {
	case FormatJSON:
		return json.File(path, name)
	case FormatEnvFile:
		return genericSource.File(path, env.Parser{}, name)
	case FormatYAML:
		return yaml.File(path, name)
	default:
		panic("unsupported test source format")
	}
}

func Env(name, _ string, lookup func(string) (string, bool)) Source {
	return lookupSource{name: name, lookup: lookup}
}
func ProcessEnv(_ string) Source { return env.Process() }

type lookupSource struct {
	name   string
	lookup func(string) (string, bool)
}

func (s lookupSource) Name() string { return s.name }
func (s lookupSource) Load(context.Context) (Layer, error) {
	return Layer{Name: s.name, Data: map[string]any{}}, nil
}

func (s lookupSource) LoadFor(_ context.Context, target reflect.Type) (Layer, error) {
	values, err := fieldenv.ParseStruct(target, s.lookup)
	if err != nil {
		return Layer{}, fmt.Errorf("parse test environment source: %w", err)
	}
	return Layer{Name: s.name, Data: values}, nil
}

func NewReloader[T any](loader *config.Loader[T], opts ...reload.Option) (*Reloader[T], error) {
	reloader, err := reload.New(loader, opts...)
	if err != nil {
		return nil, fmt.Errorf("create test reloader: %w", err)
	}
	return reloader, nil
}
func WithWatchFile(path string) reload.Option          { return reload.WithWatchFile(path) }
func WithDebounce(delay time.Duration) reload.Option   { return reload.WithDebounce(delay) }
func WithTrigger(trigger reload.Trigger) reload.Option { return reload.WithTrigger(trigger) }
func cloneSnapshot[T any](value T) T {
	loader, err := config.New[T]()
	if err != nil {
		panic(err)
	}
	return loader.Clone(value)
}
