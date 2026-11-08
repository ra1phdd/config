// Package schema caches reflection metadata shared by configuration loading and sources.
package schema

import (
	"encoding"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
)

// TagFormat selects the source-specific struct tag used to normalize object keys.
type TagFormat uint8

const (
	// JSON selects the json struct tag.
	JSON TagFormat = iota + 1
	// YAML selects the yaml struct tag.
	YAML
)

// Field describes an exported struct field and its config tag options.
type Field struct {
	StructField reflect.StructField
	Index       int
	Name        string
	HasConfig   bool
	JSONName    string
	HasJSON     bool
	JSONSkip    bool
	YAMLName    string
	HasYAML     bool
	YAMLSkip    bool
	Required    bool
	Skip        bool
}

var descriptors sync.Map

// Fields returns cached metadata for the exported fields of t.
// Callers must not modify the returned slice.
func Fields(t reflect.Type) []Field {
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	if cached, ok := descriptors.Load(t); ok {
		fields, ok := cached.([]Field)
		if !ok {
			panic("schema descriptor cache contains an invalid value")
		}
		return fields
	}
	fields := make([]Field, 0, t.NumField())
	for index := range t.NumField() {
		field := t.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, hasConfig, skip, required := fieldName(field)
		jsonName, hasJSON, jsonSkip := tagName(field.Tag.Get("json"))
		yamlName, hasYAML, yamlSkip := tagName(field.Tag.Get("yaml"))
		fields = append(fields, Field{
			StructField: field,
			Index:       index,
			Name:        name,
			HasConfig:   hasConfig,
			JSONName:    jsonName,
			HasJSON:     hasJSON,
			JSONSkip:    jsonSkip,
			YAMLName:    yamlName,
			HasYAML:     hasYAML,
			YAMLSkip:    yamlSkip,
			Required:    required,
			Skip:        skip,
		})
	}
	actual, _ := descriptors.LoadOrStore(t, fields)
	result, ok := actual.([]Field)
	if !ok {
		panic("schema descriptor cache contains an invalid value")
	}
	return result
}

// IsStruct reports whether t should be treated as a nested configuration struct.
func IsStruct(t reflect.Type) bool {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t == reflect.TypeFor[time.Duration]() {
		return false
	}
	if reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		return false
	}
	if t.PkgPath() == "net/url" && t.Name() == "URL" {
		return false
	}
	return t.Kind() == reflect.Struct && t.Name() != "Duration"
}

// IsStringMap reports whether t is a string-keyed map, optionally behind pointers.
func IsStringMap(t reflect.Type) bool {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t != nil && t.Kind() == reflect.Map && t.Key().Kind() == reflect.String
}

// NormalizeKeys maps source-tagged object keys to canonical config field names.
// Unknown keys are preserved for strict validation. opaque identifies custom
// unmarshaler types whose contents belong to the format-specific decoder.
func NormalizeKeys(input map[string]any, typ reflect.Type, format TagFormat, opaque func(reflect.Type) bool) map[string]any {
	value, changed := normalizeKeys(input, typ, format, opaque)
	if !changed {
		return input
	}
	if normalized, ok := value.(map[string]any); ok {
		return normalized
	}
	return input
}

func normalizeKeys(input any, typ reflect.Type, format TagFormat, opaque func(reflect.Type) bool) (any, bool) {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil {
		return input, false
	}
	switch {
	case typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array:
		return normalizeSequence(input, typ.Elem(), format, opaque)
	case typ.Kind() == reflect.Map && typ.Key().Kind() == reflect.String:
		return normalizeMapValues(input, typ.Elem(), format, opaque)
	}
	if !IsStruct(typ) || opaque != nil && opaque(typ) {
		return input, false
	}
	return normalizeStruct(input, typ, format, opaque)
}

func normalizeSequence(input any, elem reflect.Type, format TagFormat, opaque func(reflect.Type) bool) (any, bool) {
	values, ok := input.([]any)
	if !ok {
		return input, false
	}
	var out []any
	for index, value := range values {
		normalized, changed := normalizeKeys(value, elem, format, opaque)
		if changed {
			if out == nil {
				out = append([]any(nil), values...)
			}
			out[index] = normalized
		}
	}
	return out, out != nil
}

func normalizeMapValues(input any, elem reflect.Type, format TagFormat, opaque func(reflect.Type) bool) (any, bool) {
	values, ok := input.(map[string]any)
	if !ok {
		return input, false
	}
	var out map[string]any
	for key, value := range values {
		normalized, changed := normalizeKeys(value, elem, format, opaque)
		if changed {
			if out == nil {
				out = maps.Clone(values)
			}
			out[key] = normalized
		}
	}
	return out, out != nil
}

func normalizeStruct(input any, typ reflect.Type, format TagFormat, opaque func(reflect.Type) bool) (any, bool) {
	values, ok := input.(map[string]any)
	if !ok {
		return input, false
	}
	var out map[string]any
	for _, field := range Fields(typ) {
		if field.Skip {
			continue
		}
		if (format == JSON && field.JSONSkip || format == YAML && field.YAMLSkip) && !field.HasConfig {
			continue
		}
		if normalized, aliases, changed := normalizeField(field, values, typ, format, opaque); changed {
			if out == nil {
				out = maps.Clone(values)
			}
			for _, alias := range aliases {
				delete(out, alias)
			}
			out[field.Name] = normalized
		}
	}
	return out, out != nil
}

func normalizeField(field Field, values map[string]any, typ reflect.Type, format TagFormat, opaque func(reflect.Type) bool) (any, []string, bool) {
	var candidates [3]string
	count := fieldNames(field, format, &candidates)
	selectedName := ""
	var selected any
	var aliases []string
	for _, candidate := range candidates[:count] {
		value, exists := values[candidate]
		if !exists {
			continue
		}
		if candidate != field.Name {
			aliases = append(aliases, candidate)
		}
		if selectedName == "" {
			selected = value
			selectedName = candidate
		}
	}
	if selectedName == "" {
		return nil, nil, false
	}
	normalized, nestedChanged := normalizeKeys(selected, typ.Field(field.Index).Type, format, opaque)
	return normalized, aliases, selectedName != field.Name || len(aliases) != 0 || nestedChanged
}

func fieldNames(field Field, format TagFormat, names *[3]string) int {
	count := 0
	if field.HasConfig {
		count = appendFieldName(names, count, field.Name)
	}
	switch format {
	case JSON:
		if field.HasJSON && !field.JSONSkip {
			count = appendFieldName(names, count, field.JSONName)
		}
	case YAML:
		if field.HasYAML && !field.YAMLSkip {
			count = appendFieldName(names, count, field.YAMLName)
		}
	}
	if !slices.Contains(names[:count], field.Name) {
		names[count] = field.Name
		count++
	}
	return count
}

func appendFieldName(names *[3]string, count int, name string) int {
	if !slices.Contains(names[:count], name) {
		names[count] = name
		return count + 1
	}
	return count
}

func fieldName(field reflect.StructField) (string, bool, bool, bool) {
	parts := strings.Split(field.Tag.Get("config"), ",")
	if parts[0] == "-" {
		return "", false, true, false
	}
	name := parts[0]
	hasConfig := name != ""
	if name == "" {
		name = lowerFirst(field.Name)
	}
	required := false
	for _, option := range parts[1:] {
		required = required || option == "required"
	}
	return name, hasConfig, false, required
}

func tagName(tag string) (string, bool, bool) {
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return "", false, true
	}
	return name, name != "", false
}

func lowerFirst(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
}
