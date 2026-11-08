// Package envmap maps environment names to configuration field paths.
package envmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/ra1phdd/config/v2/internal/schema"
)

type fieldDescriptor struct {
	field        reflect.StructField
	name         string
	canonicalEnv string
	skip         bool
	stringMap    bool
}

var descriptorCache sync.Map

// ParseStruct reads fields described by config and env tags into a nested map.
func ParseStruct(t reflect.Type, lookup func(string) (string, bool)) (map[string]any, error) {
	if t == nil {
		return nil, errors.New("environment target must be a struct")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, errors.New("environment target must be a struct")
	}
	values := make(map[string]any, len(fieldsFor(t)))
	if err := readFields(t, nil, lookup, values); err != nil {
		return nil, err
	}
	return values, nil
}

// ValidateKeys rejects environment-file keys that do not map to an exported target field.
func ValidateKeys(t reflect.Type, values map[string]string) error {
	if t == nil {
		return errors.New("environment target must be a struct")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return errors.New("environment target must be a struct")
	}
	known := make(map[string]struct{})
	if err := collectEnvNames(t, nil, known); err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := known[key]; !ok {
			return fmt.Errorf("unknown environment key %q", key)
		}
	}
	return nil
}

func collectEnvNames(t reflect.Type, parent []string, names map[string]struct{}) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return errors.New("environment target must be a struct")
	}
	for _, descriptor := range fieldsFor(t) {
		field := descriptor.field
		if field.PkgPath != "" || descriptor.skip {
			continue
		}
		path := append(append([]string(nil), parent...), descriptor.name)
		if schema.IsStruct(field.Type) && field.Tag.Get("env") == "" {
			if err := collectEnvNames(field.Type, path, names); err != nil {
				return err
			}
			continue
		}
		canonical := canonicalEnvName(path, descriptor.canonicalEnv)
		names[canonical] = struct{}{}
		if alias := field.Tag.Get("env"); alias != "" && alias != canonical {
			names[alias] = struct{}{}
		}
	}
	return nil
}

func readFields(t reflect.Type, parent []string, lookup func(string) (string, bool), dst map[string]any) error { //nolint:gocognit // Nested environment names resolve in one recursive walk.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return errors.New("environment target must be a struct")
	}
	for _, descriptor := range fieldsFor(t) {
		field := descriptor.field
		if field.PkgPath != "" {
			continue
		}
		if descriptor.skip {
			continue
		}
		name := descriptor.name
		if schema.IsStruct(field.Type) && field.Tag.Get("env") == "" {
			path := append(append([]string(nil), parent...), name)
			if err := readFields(field.Type, path, lookup, dst); err != nil {
				return err
			}
			continue
		}
		var value string
		envName := canonicalEnvName(parent, descriptor.canonicalEnv)
		value, found := lookup(envName)
		if !found {
			alias := field.Tag.Get("env")
			if alias != "" && alias != envName {
				envName = alias
				value, found = lookup(envName)
			}
		}
		if found {
			var parsed any = value
			if descriptor.stringMap {
				var err error
				parsed, err = parseMapField(field.Type, value)
				if err != nil {
					return fmt.Errorf("parse environment value %q: %w", envName, err)
				}
			}
			put(dst, parent, name, parsed)
		}
	}
	return nil
}

func canonicalEnvName(parent []string, canonicalName string) string {
	if len(parent) == 0 {
		return canonicalName
	}
	return strings.ToUpper(strings.Join(parent, "_") + "_" + canonicalName)
}

func parseMapField(fieldType reflect.Type, text string) (any, error) {
	value := reflect.New(fieldType)
	if err := json.Unmarshal([]byte(text), value.Interface()); err != nil {
		return nil, fmt.Errorf("decode map value: %w", err)
	}
	return value.Elem().Interface(), nil
}

func fieldsFor(t reflect.Type) []fieldDescriptor {
	if cached, ok := descriptorCache.Load(t); ok {
		fields, ok := cached.([]fieldDescriptor)
		if !ok {
			panic("environment descriptor cache contains an invalid value")
		}
		return fields
	}
	metadata := schema.Fields(t)
	fields := make([]fieldDescriptor, 0, len(metadata))
	for _, item := range metadata {
		field := item.StructField
		name, skip := item.Name, item.Skip
		fields = append(fields, fieldDescriptor{
			field:        field,
			name:         name,
			canonicalEnv: strings.ToUpper(name),
			skip:         skip,
			stringMap:    schema.IsStringMap(field.Type),
		})
	}
	actual, _ := descriptorCache.LoadOrStore(t, fields)
	fields, ok := actual.([]fieldDescriptor)
	if !ok {
		panic("environment descriptor cache contains an invalid value")
	}
	return fields
}

func put(dst map[string]any, parent []string, name string, value any) {
	for _, name := range parent {
		child, ok := dst[name].(map[string]any)
		if !ok {
			child = map[string]any{}
			dst[name] = child
		}
		dst = child
	}
	dst[name] = value
}
