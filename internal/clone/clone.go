// Package clone provides cycle-preserving reflection copies for configuration values.
package clone

import "reflect"

type visit struct {
	typ  reflect.Type
	kind reflect.Kind
	ptr  uintptr
	len  int
	cap  int
}

// Value returns an independent copy of value, preserving reference cycles.
func Value(value any) any {
	if value == nil {
		return nil
	}
	cloned := reflectValue(reflect.ValueOf(value), make(map[visit]reflect.Value))
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

// Reflect returns an independent reflection value, preserving reference cycles.
func Reflect(value reflect.Value) reflect.Value {
	return reflectValue(value, make(map[visit]reflect.Value))
}

func reflectValue(value reflect.Value, seen map[visit]reflect.Value) reflect.Value { //nolint:gocognit // Reflection dispatch handles all reference kinds and cycle tracking.
	if !value.IsValid() {
		return value
	}
	if value.CanInterface() {
		if cloner, ok := reflect.TypeAssert[interface{ CloneConfigValue() any }](value); ok {
			return reflect.ValueOf(cloner.CloneConfigValue())
		}
	}
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if cloned, ok := seen[key]; ok {
			return cloned
		}
		out := reflect.New(value.Type().Elem())
		seen[key] = out
		out.Elem().Set(reflectValue(value.Elem(), seen))
		return out
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type()).Elem()
		out.Set(reflectValue(value.Elem(), seen))
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}
		if cloned, ok := seen[key]; ok {
			return cloned
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		seen[key] = out
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(reflectValue(iter.Key(), seen), reflectValue(iter.Value(), seen))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		key := visit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}
		if cloned, ok := seen[key]; ok {
			return cloned
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		seen[key] = out
		for index := range value.Len() {
			out.Index(index).Set(reflectValue(value.Index(index), seen))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for index := range value.Len() {
			out.Index(index).Set(reflectValue(value.Index(index), seen))
		}
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		out.Set(value)
		for index := range value.NumField() {
			if out.Field(index).CanSet() && value.Field(index).CanInterface() {
				out.Field(index).Set(reflectValue(value.Field(index), seen))
			}
		}
		return out
	default:
		return value
	}
}
