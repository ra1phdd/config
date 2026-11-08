// Package yaml parses YAML configuration without coupling the core to YAML.
package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/ra1phdd/config/v2"
	"github.com/ra1phdd/config/v2/internal/schema"
	genericSource "github.com/ra1phdd/config/v2/source"
	yamlv3 "gopkg.in/yaml.v3"
)

// Parser implements config.Parser for a single YAML document.
type Parser struct{}

// Parse decodes one YAML document and normalizes its maps and sequences.
func (Parser) Parse(data []byte) (config.Layer, error) {
	decoder := yamlv3.NewDecoder(bytes.NewReader(data))
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return config.Layer{}, fmt.Errorf("decode YAML document: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple YAML documents in config")
		}
		return config.Layer{}, fmt.Errorf("decode YAML document: %w", err)
	}
	values, ok := normalize(raw).(map[string]any)
	if !ok {
		return config.Layer{}, errors.New("YAML root must be an object")
	}
	return config.Layer{Data: values, Decoder: decodeValue}, nil
}

// ParseFor parses one YAML document and resolves field names against target metadata.
func (p Parser) ParseFor(data []byte, target reflect.Type, _ bool) (config.Layer, error) {
	layer, err := p.Parse(data)
	if err != nil {
		return config.Layer{}, err
	}
	layer.Data = schema.NormalizeKeys(layer.Data, target, schema.YAML, hasCustomUnmarshaler)
	return layer, nil
}

func hasCustomUnmarshaler(t reflect.Type) bool {
	interfaceType := reflect.TypeFor[yamlv3.Unmarshaler]()
	return t.Implements(interfaceType) || t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(interfaceType)
}

func decodeValue(input any, target any) (bool, error) {
	if _, ok := target.(yamlv3.Unmarshaler); !ok {
		return false, nil
	}
	data, err := yamlv3.Marshal(input)
	if err != nil {
		return true, fmt.Errorf("marshal YAML custom value: %w", err)
	}
	if err := yamlv3.Unmarshal(data, target); err != nil {
		return true, fmt.Errorf("unmarshal YAML custom value: %w", err)
	}
	return true, nil
}

func normalize(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = normalize(item)
		}
		return v
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = normalize(item)
		}
		return out
	case []any:
		for i, item := range v {
			v[i] = normalize(item)
		}
		return v
	default:
		return value
	}
}

// File creates a YAML source for path.
func File(path string, names ...string) config.Source {
	return genericSource.File(path, Parser{}, names...)
}

// StrictFile creates a YAML source that rejects unknown fields.
func StrictFile(path string, names ...string) config.Source {
	return genericSource.Strict(File(path, names...))
}

// Bytes creates an in-memory YAML source.
func Bytes(data []byte, names ...string) config.Source {
	name := "yaml"
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return genericSource.Bytes(data, Parser{}, name)
}

// StrictBytes creates an in-memory YAML source that rejects unknown fields.
func StrictBytes(data []byte, names ...string) config.Source {
	return genericSource.Strict(Bytes(data, names...))
}
