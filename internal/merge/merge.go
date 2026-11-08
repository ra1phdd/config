// Package merge combines normalized configuration maps and tracks source metadata.
package merge

import (
	"fmt"
	"reflect"

	"github.com/ra1phdd/config/internal/clone"
	"github.com/ra1phdd/config/internal/path"
	"github.com/ra1phdd/config/internal/schema"
)

type visit struct {
	typ  reflect.Type
	kind reflect.Kind
	ptr  uintptr
	len  int
	cap  int
}

// Merge recursively merges src into dst. D is the source-format decoder type.
func Merge[D any](dst, src map[string]any, parent, source string, origins map[string]string, decoders map[string]D, decoder D) {
	normalized, ok := normalize(src).(map[string]any)
	if !ok {
		return
	}
	mergeSeen(dst, normalized, parent, source, origins, decoders, decoder, make(map[visit]bool))
}

func mergeSeen[D any](dst, src map[string]any, parent, source string, origins map[string]string, decoders map[string]D, decoder D, active map[visit]bool) {
	key := visit{typ: reflect.TypeOf(src), kind: reflect.Map, ptr: reflect.ValueOf(src).Pointer()}
	if active[key] {
		return
	}
	active[key] = true
	defer delete(active, key)
	for name, value := range src {
		mergeField(dst, name, value, parent, source, origins, decoders, decoder, active)
	}
}

func mergeField[D any](dst map[string]any, name string, value any, parent, source string, origins map[string]string, decoders map[string]D, decoder D, active map[visit]bool) {
	fieldPath := path.Join(parent, name)
	if sourceMap, ok := value.(map[string]any); ok {
		if targetMap, ok := dst[name].(map[string]any); ok {
			if origins != nil {
				origins[fieldPath] = source
			}
			mergeSeen(targetMap, sourceMap, fieldPath, source, origins, decoders, decoder, active)
			return
		}
	}
	_, replacesMap := dst[name].(map[string]any)
	if origins != nil {
		if replacesMap {
			clearOrigins(origins, fieldPath)
		}
		origins[fieldPath] = source
	}
	if decoders != nil && decoderPresent(decoder) {
		if replacesMap {
			clearDecoders(decoders, fieldPath)
		}
		setDecoderPaths(decoders, value, fieldPath, decoder)
	}
	dst[name] = clone.Value(value)
}

func clearDecoders[D any](decoders map[string]D, fieldPath string) {
	for key := range decoders {
		if path.Within(key, fieldPath) {
			delete(decoders, key)
		}
	}
}

func setDecoderPaths[D any](decoders map[string]D, value any, fieldPath string, decoder D) {
	if !decoderPresent(decoder) {
		return
	}
	decoders[fieldPath] = decoder
	switch nested := value.(type) {
	case map[string]any:
		for key, child := range nested {
			setDecoderPaths(decoders, child, path.Join(fieldPath, key), decoder)
		}
	case []any:
		for index, child := range nested {
			setDecoderPaths(decoders, child, fmt.Sprintf("%s[%d]", fieldPath, index), decoder)
		}
	}
}

func decoderPresent[D any](decoder D) bool {
	value := reflect.ValueOf(decoder)
	if !value.IsValid() {
		return false
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func clearOrigins(origins map[string]string, fieldPath string) {
	for key := range origins {
		if path.Within(key, fieldPath) {
			delete(origins, key)
		}
	}
}

func normalize(value any) any {
	if value == nil || !requiresNormalization(reflect.ValueOf(value), make(map[visit]bool)) {
		return value
	}
	return normalizeReflect(reflect.ValueOf(value), make(map[visit]any))
}

func requiresNormalization(value reflect.Value, active map[visit]bool) bool { //nolint:gocognit // Recursive normalization inspects arbitrary typed values.
	if !value.IsValid() {
		return false
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		return requiresNormalization(value.Elem(), active)
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String || value.Type() != reflect.TypeFor[map[string]any]() {
			return true
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if active[key] {
			return false
		}
		active[key] = true
		defer delete(active, key)
		iter := value.MapRange()
		for iter.Next() {
			if requiresNormalization(iter.Value(), active) {
				return true
			}
		}
		return false
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 || value.Type() != reflect.TypeFor[[]any]() {
			return value.Type().Elem().Kind() != reflect.Uint8
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
		if active[key] {
			return false
		}
		active[key] = true
		defer delete(active, key)
		for index := range value.Len() {
			if requiresNormalization(value.Index(index), active) {
				return true
			}
		}
		return false
	case reflect.Array:
		return true
	case reflect.Struct:
		return schema.IsStruct(value.Type())
	default:
		return false
	}
}

func normalizeReflect(value reflect.Value, seen map[visit]any) any { //nolint:gocognit // Recursive normalization handles all supported reference kinds.
	if !value.IsValid() {
		return nil
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return nil
		}
		return normalizeReflect(value.Elem(), seen)
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			return clone.Value(value.Interface())
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if existing, ok := seen[key]; ok {
			return existing
		}
		out := make(map[string]any, value.Len())
		seen[key] = out
		iter := value.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = normalizeReflect(iter.Value(), seen)
		}
		return out
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return clone.Value(value.Interface())
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
		if existing, ok := seen[key]; ok {
			return existing
		}
		out := make([]any, value.Len())
		seen[key] = out
		for index := range value.Len() {
			out[index] = normalizeReflect(value.Index(index), seen)
		}
		return out
	case reflect.Array:
		out := make([]any, value.Len())
		for index := range value.Len() {
			out[index] = normalizeReflect(value.Index(index), seen)
		}
		return out
	default:
		return clone.Value(value.Interface())
	}
}
