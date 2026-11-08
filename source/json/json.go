// Package json parses JSON configuration without coupling the core to JSON.
package json

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/ra1phdd/config"
	"github.com/ra1phdd/config/internal/schema"
	genericSource "github.com/ra1phdd/config/source"
)

// Parser implements config.Parser for a single JSON document.
type Parser struct{}

// Parse decodes a JSON object while preserving number precision.
func (Parser) Parse(data []byte) (config.Layer, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(data))
	start, err := decoder.ReadToken()
	if err != nil {
		return config.Layer{}, fmt.Errorf("decode JSON object: %w", err)
	}
	if start.Kind() != jsontext.KindBeginObject {
		return config.Layer{}, errors.New("JSON root is not an object")
	}
	values, err := decodeObject(decoder)
	if err != nil {
		return config.Layer{}, fmt.Errorf("decode JSON object: %w", err)
	}
	if _, err := decoder.ReadToken(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON documents in config")
		}
		return config.Layer{}, fmt.Errorf("decode JSON object: %w", err)
	}
	return config.Layer{Data: values, Decoder: decodeValue}, nil
}

// ParseFor parses a JSON object and resolves field names against target metadata.
func (p Parser) ParseFor(data []byte, target reflect.Type, _ bool) (config.Layer, error) {
	layer, err := p.Parse(data)
	if err != nil {
		return config.Layer{}, err
	}
	layer.Data = schema.NormalizeKeys(layer.Data, target, schema.JSON, hasCustomUnmarshaler)
	return layer, nil
}

func hasCustomUnmarshaler(t reflect.Type) bool {
	interfaceType := reflect.TypeFor[jsonv2.Unmarshaler]()
	return t.Implements(interfaceType) || t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(interfaceType)
}

type jsonNumber string

func (n jsonNumber) MarshalJSON() ([]byte, error) { return []byte(n), nil }

func decodeObject(decoder *jsontext.Decoder) (map[string]any, error) {
	values := make(map[string]any, 8)
	for {
		key, err := decoder.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("read JSON object token: %w", err)
		}
		if key.Kind() == jsontext.KindEndObject {
			return values, nil
		}
		if key.Kind() != jsontext.KindString {
			return nil, fmt.Errorf("expected object name, got %s", key.Kind())
		}
		keyName := key.String()
		value, err := decodeValueToken(decoder)
		if err != nil {
			return nil, fmt.Errorf("decode member %q: %w", keyName, err)
		}
		values[keyName] = value
	}
}

func decodeArray(decoder *jsontext.Decoder) ([]any, error) {
	values := make([]any, 0, 4)
	for {
		token, err := decoder.ReadToken()
		if err != nil {
			return nil, fmt.Errorf("read JSON array token: %w", err)
		}
		if token.Kind() == jsontext.KindEndArray {
			return values, nil
		}
		value, err := decodeToken(decoder, token)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
}

func decodeValueToken(decoder *jsontext.Decoder) (any, error) {
	token, err := decoder.ReadToken()
	if err != nil {
		return nil, fmt.Errorf("read JSON value token: %w", err)
	}
	return decodeToken(decoder, token)
}

func decodeToken(decoder *jsontext.Decoder, token jsontext.Token) (any, error) {
	switch token.Kind() {
	case jsontext.KindBeginObject:
		return decodeObject(decoder)
	case jsontext.KindBeginArray:
		return decodeArray(decoder)
	case jsontext.KindNull:
		return nil, nil //nolint:nilnil // JSON null is a valid configuration value.
	case jsontext.KindFalse:
		return false, nil
	case jsontext.KindTrue:
		return true, nil
	case jsontext.KindString:
		return token.String(), nil
	case jsontext.KindNumber:
		return jsonNumber(token.String()), nil
	default:
		return nil, fmt.Errorf("unsupported JSON value kind %s", token.Kind())
	}
}

func decodeValue(input any, target any) (bool, error) {
	unmarshaler, ok := target.(jsonv2.Unmarshaler)
	if !ok {
		return false, nil
	}
	data, err := jsonv2.Marshal(input)
	if err != nil {
		return true, fmt.Errorf("marshal JSON custom value: %w", err)
	}
	if err := unmarshaler.UnmarshalJSON(data); err != nil {
		return true, fmt.Errorf("unmarshal JSON custom value: %w", err)
	}
	return true, nil
}

type cachedBytesSource struct {
	name string
	data []byte

	once  sync.Once
	layer config.Layer
	err   error
}

func (s *cachedBytesSource) Name() string { return s.name }

func (s *cachedBytesSource) Load(ctx context.Context) (config.Layer, error) {
	return s.LoadFor(ctx, nil)
}

func (s *cachedBytesSource) LoadFor(ctx context.Context, target reflect.Type) (config.Layer, error) {
	if err := ctx.Err(); err != nil {
		return config.Layer{}, fmt.Errorf("checking source context: %w", err)
	}
	s.once.Do(func() {
		s.layer, s.err = (Parser{}).Parse(s.data)
		if s.err != nil {
			s.err = &config.FieldError{Source: s.name, Op: "parse", Err: fmt.Errorf("%w: %w", config.ErrInvalid, s.err)}
		}
	})
	if s.err != nil {
		return config.Layer{}, s.err
	}
	data := schema.NormalizeKeys(s.layer.Data, target, schema.JSON, hasCustomUnmarshaler)
	source := genericSource.Values(s.name, data)
	layer, err := source.Load(ctx)
	if err != nil {
		return config.Layer{}, fmt.Errorf("load cached JSON source: %w", err)
	}
	layer.Decoder = s.layer.Decoder
	return layer, nil
}

// File creates a JSON source for path.
func File(path string, names ...string) config.Source {
	return genericSource.File(path, Parser{}, names...)
}

// StrictFile creates a JSON source that rejects unknown fields.
func StrictFile(path string, names ...string) config.Source {
	return genericSource.Strict(File(path, names...))
}

// Bytes creates a defensive, cached JSON source for immutable in-memory data.
func Bytes(data []byte, names ...string) config.Source {
	name := "json"
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return &cachedBytesSource{name: name, data: append([]byte(nil), data...)}
}

// StrictBytes creates a strict cached JSON source that rejects unknown fields.
func StrictBytes(data []byte, names ...string) config.Source {
	return genericSource.Strict(Bytes(data, names...))
}

var _ config.Parser = Parser{}
var _ config.TargetParser = Parser{}
