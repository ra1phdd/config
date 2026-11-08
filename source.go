package config

import (
	"context"
	"fmt"
	"reflect"
)

// Source provides one ordered configuration layer.
type Source interface {
	Name() string
	Load(context.Context) (Layer, error)
}

// TargetSource optionally resolves a layer against the destination type.
type TargetSource interface {
	Source
	// LoadFor resolves the source against the destination type.
	LoadFor(context.Context, reflect.Type) (Layer, error)
}

// Parser converts bytes into a normalized configuration layer.
type Parser interface {
	Parse([]byte) (Layer, error)
}

// TargetParser optionally parses bytes using the destination type.
type TargetParser interface {
	Parser
	// ParseFor may use target struct metadata; strict requests unknown-field validation.
	ParseFor([]byte, reflect.Type, bool) (Layer, error)
}

// Layer is a source result. Data must be a map with string keys.
type Layer struct {
	Name    string
	Data    map[string]any
	Strict  bool
	Decoder LayerDecoder
}

// LayerDecoder applies a source-format-specific decoder to one value.
type LayerDecoder func(input any, target any) (handled bool, err error)

func wrapSourceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
