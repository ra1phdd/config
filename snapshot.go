package config

import (
	"reflect"

	"github.com/ra1phdd/config/v2/internal/clone"
)

func cloneSnapshot[T any](value T) T {
	cloned := clone.Reflect(reflect.ValueOf(value))
	if !cloned.IsValid() {
		var zero T
		return zero
	}
	result, ok := reflect.TypeAssert[T](cloned)
	if !ok {
		var zero T
		return zero
	}
	return result
}

type cloneVisit struct {
	typ  reflect.Type
	kind reflect.Kind
	ptr  uintptr
	len  int
	cap  int
}

// SnapshotCloner supplies an explicit deep copy for types with unexported mutable state.
type SnapshotCloner interface {
	CloneConfigValue() any
}
