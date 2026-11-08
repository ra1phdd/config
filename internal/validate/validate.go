// Package validate implements reflection-based configuration validation policies.
package validate

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/ra1phdd/config/internal/path"
	"github.com/ra1phdd/config/internal/schema"
)

// Issue describes one validation failure without a public source name.
type Issue struct {
	Path string
	Err  error
}

type visit struct {
	typ  reflect.Type
	kind reflect.Kind
	ptr  uintptr
	len  int
	cap  int
}

// Strict rejects fields that do not exist in typ.
func Strict(input any, typ reflect.Type, parent string) []Issue {
	return strictLayer(input, typ, parent)
}

func strictLayer(input any, typ reflect.Type, parent string) []Issue {
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil {
		return nil
	}
	if schema.IsStruct(typ) {
		return strictStruct(input, typ, parent)
	}
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		values, ok := input.([]any)
		if !ok {
			return nil
		}
		var issues []Issue
		for index, value := range values {
			issues = append(issues, strictLayer(value, typ.Elem(), fmt.Sprintf("%s[%d]", parent, index))...)
		}
		return issues
	}
	if typ.Kind() == reflect.Map && typ.Key().Kind() == reflect.String {
		values, ok := input.(map[string]any)
		if !ok {
			return nil
		}
		var issues []Issue
		for key, value := range values {
			issues = append(issues, strictLayer(value, typ.Elem(), path.Join(parent, key))...)
		}
		return issues
	}
	return nil
}

func strictStruct(input any, typ reflect.Type, parent string) []Issue {
	values, ok := input.(map[string]any)
	if !ok {
		return nil
	}
	known := make(map[string]schema.Field)
	for _, field := range schema.Fields(typ) {
		if !field.Skip {
			known[field.Name] = field
		}
	}
	var issues []Issue
	for _, name := range sortedKeys(values) {
		field, ok := known[name]
		fieldPath := path.Join(parent, name)
		if !ok {
			issues = append(issues, Issue{Path: fieldPath, Err: errors.New("unknown field")})
			continue
		}
		issues = append(issues, strictLayer(values[name], typ.Field(field.Index).Type, fieldPath)...)
	}
	return issues
}

// Required returns missing required-field failures for value.
func Required(value reflect.Value, parent string, present map[string]bool) []Issue {
	return requiredValue(value, parent, present, make(map[visit]bool))
}

func requiredValue(value reflect.Value, parent string, present map[string]bool, active map[visit]bool) []Issue { //nolint:gocognit // Required checks recurse through arbitrary typed values.
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if active[key] {
			return nil
		}
		active[key] = true
		defer delete(active, key)
		value = value.Elem()
	}
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return nil
		}
		return requiredValue(value.Elem(), parent, present, active)
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		return requiredSequence(value, parent, present, active)
	}
	if value.Kind() == reflect.Map {
		if value.IsNil() || value.Type().Key().Kind() != reflect.String {
			return nil
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if active[key] {
			return nil
		}
		active[key] = true
		defer delete(active, key)
		var issues []Issue
		keys := make([]string, 0, value.Len())
		items := make(map[string]reflect.Value, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			name := iter.Key().String()
			keys = append(keys, name)
			items[name] = iter.Value()
		}
		sort.Strings(keys)
		for _, name := range keys {
			issues = append(issues, requiredValue(items[name], path.Join(parent, name), present, active)...)
		}
		return issues
	}
	if !schema.IsStruct(value.Type()) {
		return nil
	}
	var issues []Issue
	for _, field := range schema.Fields(value.Type()) {
		if field.Skip {
			continue
		}
		fieldValue := value.Field(field.Index)
		fieldPath := path.Join(parent, field.Name)
		nilRequired := (fieldValue.Kind() == reflect.Pointer || fieldValue.Kind() == reflect.Map || fieldValue.Kind() == reflect.Slice || fieldValue.Kind() == reflect.Interface) && fieldValue.IsNil()
		if field.Required && (nilRequired || fieldValue.IsZero() && !present[fieldPath]) {
			issues = append(issues, Issue{Path: fieldPath, Err: errors.New("value is missing")})
		}
		issues = append(issues, requiredValue(fieldValue, fieldPath, present, active)...)
	}
	return issues
}

func requiredSequence(value reflect.Value, parent string, present map[string]bool, active map[visit]bool) []Issue {
	if value.Kind() == reflect.Slice {
		if value.IsNil() {
			return nil
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
		if active[key] {
			return nil
		}
		active[key] = true
		defer delete(active, key)
	}
	var issues []Issue
	for index := range value.Len() {
		issues = append(issues, requiredValue(value.Index(index), fmt.Sprintf("%s[%d]", parent, index), present, active)...)
	}
	return issues
}

// MarkPresent records paths present in normalized source data.
func MarkPresent(values map[string]any, parent string, present map[string]bool) {
	markMap(values, parent, present, make(map[visit]bool))
}

func markMap(values map[string]any, parent string, present map[string]bool, active map[visit]bool) {
	key := visit{typ: reflect.TypeOf(values), kind: reflect.Map, ptr: reflect.ValueOf(values).Pointer()}
	if active[key] {
		return
	}
	active[key] = true
	defer delete(active, key)
	for name, value := range values {
		fieldPath := path.Join(parent, name)
		present[fieldPath] = true
		markValue(value, fieldPath, present, active)
	}
}

func markValue(value any, parent string, present map[string]bool, active map[visit]bool) {
	switch nested := value.(type) {
	case map[string]any:
		markMap(nested, parent, present, active)
	case []any:
		key := visit{typ: reflect.TypeOf(nested), kind: reflect.Slice, ptr: reflect.ValueOf(nested).Pointer(), len: len(nested), cap: cap(nested)}
		if active[key] {
			return
		}
		active[key] = true
		defer delete(active, key)
		for index, item := range nested {
			fieldPath := fmt.Sprintf("%s[%d]", parent, index)
			present[fieldPath] = true
			markValue(item, fieldPath, present, active)
		}
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			markReflect(rv, parent, present, active)
		}
	}
}

func markReflect(value reflect.Value, parent string, present map[string]bool, active map[visit]bool) { //nolint:gocognit // Presence follows arbitrary normalized reference shapes.
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Interface {
		if !value.IsNil() {
			markReflect(value.Elem(), parent, present, active)
		}
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if active[key] {
			return
		}
		active[key] = true
		defer delete(active, key)
		markReflect(value.Elem(), parent, present, active)
		return
	}
	if value.Kind() == reflect.Map && value.Type().Key().Kind() == reflect.String {
		if value.IsNil() {
			return
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if active[key] {
			return
		}
		active[key] = true
		defer delete(active, key)
		iter := value.MapRange()
		for iter.Next() {
			fieldPath := path.Join(parent, iter.Key().String())
			present[fieldPath] = true
			markReflect(iter.Value(), fieldPath, present, active)
		}
		return
	}
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		if value.Kind() == reflect.Slice && value.IsNil() {
			return
		}
		if value.Kind() == reflect.Slice {
			key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
			if active[key] {
				return
			}
			active[key] = true
			defer delete(active, key)
		}
		for index := range value.Len() {
			fieldPath := fmt.Sprintf("%s[%d]", parent, index)
			present[fieldPath] = true
			markReflect(value.Index(index), fieldPath, present, active)
		}
		return
	}
	if !schema.IsStruct(value.Type()) {
		return
	}
	for _, field := range schema.Fields(value.Type()) {
		if field.Skip {
			continue
		}
		fieldValue := value.Field(field.Index)
		if !fieldValue.CanInterface() {
			continue
		}
		fieldPath := path.Join(parent, field.Name)
		present[fieldPath] = true
		markReflect(fieldValue, fieldPath, present, active)
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
