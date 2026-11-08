// Package source contains format-neutral source adapters for config.Parser implementations.
package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/internal/clone"
)

type valueSource struct {
	name   string
	values map[string]any
}

// Values creates a defensive in-memory source.
func Values(name string, values map[string]any) config.Source {
	return valueSource{name: name, values: cloneMap(values)}
}

func (s valueSource) Name() string { return s.name }
func (s valueSource) Load(context.Context) (config.Layer, error) {
	return config.Layer{Name: s.name, Data: cloneMap(s.values)}, nil
}

type parsedSource struct {
	name, path string
	data       []byte
	parser     config.Parser
	strict     bool
}

// File creates a source that reads path and parses it with parser. An optional
// name overrides the automatically derived basename.
func File(path string, parser config.Parser, names ...string) config.Source {
	name := filepath.Base(path)
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return parsedSource{name: name, path: path, parser: parser}
}

// Bytes creates an in-memory source that parses a defensive copy of data.
func Bytes(data []byte, parser config.Parser, names ...string) config.Source {
	name := "bytes"
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return parsedSource{name: name, data: append([]byte(nil), data...), parser: parser}
}

// Strict wraps a source and requests unknown-field validation.
func Strict(source config.Source) config.Source {
	if isNilValue(source) {
		return source
	}
	if parsed, ok := source.(parsedSource); ok {
		parsed.strict = true
		return parsed
	}
	return strictSource{Source: source}
}

func (s parsedSource) Name() string { return s.name }
func (s parsedSource) Load(ctx context.Context) (config.Layer, error) {
	return s.LoadFor(ctx, nil)
}

func (s parsedSource) LoadFor(ctx context.Context, target reflect.Type) (config.Layer, error) {
	if err := ctx.Err(); err != nil {
		return config.Layer{}, fmt.Errorf("checking source context: %w", err)
	}
	if isNilValue(s.parser) {
		return config.Layer{}, errors.New("nil configuration parser")
	}
	data := s.data
	if s.path != "" {
		var err error
		data, err = os.ReadFile(s.path)
		if err != nil {
			return config.Layer{}, readError(s.name, s.path, err)
		}
	}
	var layer config.Layer
	var err error
	if parser, ok := s.parser.(config.TargetParser); ok {
		layer, err = parser.ParseFor(data, target, s.strict)
	} else {
		layer, err = s.parser.Parse(data)
	}
	if err != nil {
		fieldErr, ok := errors.AsType[*config.FieldError](err)
		if !ok || fieldErr == nil {
			err = &config.FieldError{Source: s.name, Path: s.path, Op: "parse", Err: fmt.Errorf("%w: %w", config.ErrInvalid, err)}
		}
		return config.Layer{}, withPath(err, s.path)
	}
	if layer.Name == "" {
		layer.Name = s.name
	}
	layer.Strict = layer.Strict || s.strict
	return layer, nil
}

type strictSource struct{ config.Source }

func (s strictSource) Load(ctx context.Context) (config.Layer, error) {
	layer, err := s.Source.Load(ctx)
	layer.Strict = true
	if err != nil {
		return layer, fmt.Errorf("loading strict source %q: %w", s.Name(), err)
	}
	return layer, nil
}

func (s strictSource) LoadFor(ctx context.Context, target reflect.Type) (config.Layer, error) {
	var layer config.Layer
	var err error
	if typed, ok := s.Source.(config.TargetSource); ok {
		layer, err = typed.LoadFor(ctx, target)
	} else {
		layer, err = s.Source.Load(ctx)
	}
	layer.Strict = true
	if err != nil {
		return layer, fmt.Errorf("loading strict source %q: %w", s.Name(), err)
	}
	return layer, nil
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = clone.Value(value)
	}
	return out
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func readError(name, path string, err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return &config.FieldError{Source: name, Path: path, Op: "read", Err: fmt.Errorf("%w: %s", config.ErrNotFound, path)}
	}
	return &config.FieldError{Source: name, Path: path, Op: "read", Err: err}
}

func withPath(err error, path string) error {
	if err == nil || path == "" {
		return err
	}
	var fieldErr *config.FieldError
	if errors.As(err, &fieldErr) && fieldErr.Path == "" {
		fieldErr.Path = path
	}
	return err
}
