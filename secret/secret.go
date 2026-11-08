// Package secret provides redacted configuration values with explicit reveal access.
package secret

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/ra1phdd/config/internal/clone"
)

// Secret stores a value without exposing it through formatting or JSON output.
type Secret[T any] struct {
	value T
	set   bool
}

// Reveal returns the secret value explicitly.
func (s Secret[T]) Reveal() T { return s.value }

// IsSet reports whether a source supplied a value.
func (s Secret[T]) IsSet() bool { return s.set }

// String returns a redacted representation.
func (Secret[T]) String() string { return "[REDACTED]" }

// GoString returns a redacted representation for Go formatting verbs.
func (Secret[T]) GoString() string { return "[REDACTED]" }

// SetConfigValue decodes a normalized source value into the secret.
//
//nolint:deadcode // Called by the core through the public ValueSetter contract.
func (s *Secret[T]) SetConfigValue(input any) error {
	if s == nil {
		return errors.New("nil secret")
	}
	destination := reflect.New(reflect.TypeFor[T]()).Elem()
	if err := setValue(input, destination); err != nil {
		return err
	}
	if destination.Kind() == reflect.Interface && destination.IsNil() {
		var zero T
		s.value = zero
		s.set = true
		return nil
	}
	decoded, ok := reflect.TypeAssert[T](destination)
	if !ok {
		return errors.New("secret value has an unexpected type")
	}
	s.value = decoded
	s.set = true
	return nil
}

// CloneConfigValue returns an independent redacted snapshot value.
func (s Secret[T]) CloneConfigValue() any {
	if !s.set {
		return s
	}
	input := clone.Reflect(reflect.ValueOf(s.value))
	if !input.IsValid() {
		var zero T
		s.value = zero
		return s
	}
	cloned, ok := reflect.TypeAssert[T](input)
	if ok {
		s.value = cloned
	}
	return s
}

// MarshalJSON emits a redaction marker and never serializes the secret value.
func (Secret[T]) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// UnmarshalText decodes a scalar secret using configuration text rules.
//
//nolint:deadcode // Called through encoding.TextUnmarshaler and reflection.
func (s *Secret[T]) UnmarshalText(text []byte) error {
	if s == nil {
		return errors.New("nil secret")
	}
	destination := reflect.New(reflect.TypeFor[T]()).Elem()
	if err := setText(string(text), destination); err != nil {
		return err
	}
	decoded, ok := reflect.TypeAssert[T](destination)
	if !ok {
		return errors.New("secret value has an unexpected type")
	}
	s.value = decoded
	s.set = true
	return nil
}

func setValue(input any, destination reflect.Value) error {
	if input == nil {
		destination.SetZero()
		return nil
	}
	if value := reflect.ValueOf(input); value.Type().AssignableTo(destination.Type()) {
		destination.Set(value)
		return nil
	}
	if destination.Kind() == reflect.Pointer {
		destination.Set(reflect.New(destination.Type().Elem()))
		return setValue(input, destination.Elem())
	}
	if destination.Kind() == reflect.String || destination.Kind() == reflect.Bool || destination.Kind() >= reflect.Int && destination.Kind() <= reflect.Float64 || destination.Kind() == reflect.Uint || destination.Kind() >= reflect.Uint8 && destination.Kind() <= reflect.Uintptr {
		return setText(fmt.Sprint(input), destination)
	}
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal secret value: %w", err)
	}
	if err := json.Unmarshal(data, destination.Addr().Interface()); err != nil {
		return fmt.Errorf("unmarshal secret value: %w", err)
	}
	return nil
}

func setText(text string, destination reflect.Value) error {
	if destination.CanAddr() && destination.Addr().Type().Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
		unmarshaler, ok := reflect.TypeAssert[encoding.TextUnmarshaler](destination.Addr())
		if !ok {
			return errors.New("invalid text unmarshaler")
		}
		if err := unmarshaler.UnmarshalText([]byte(text)); err != nil {
			return fmt.Errorf("unmarshal secret text: %w", err)
		}
		return nil
	}
	if destination.Type() == reflect.TypeFor[time.Duration]() {
		value, err := time.ParseDuration(text)
		if err != nil {
			return errors.New("invalid duration")
		}
		destination.SetInt(int64(value))
		return nil
	}
	switch destination.Kind() {
	case reflect.String:
		destination.SetString(text)
	case reflect.Bool:
		value, err := strconv.ParseBool(text)
		if err != nil {
			return errors.New("invalid boolean")
		}
		destination.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(text, 10, destination.Type().Bits())
		if err != nil {
			return errors.New("invalid integer")
		}
		destination.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		value, err := strconv.ParseUint(text, 10, destination.Type().Bits())
		if err != nil {
			return errors.New("invalid unsigned integer")
		}
		destination.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(text, destination.Type().Bits())
		if err != nil {
			return errors.New("invalid float")
		}
		destination.SetFloat(value)
	default:
		return fmt.Errorf("unsupported secret type %s", destination.Type())
	}
	return nil
}

var (
	_ encoding.TextUnmarshaler               = (*Secret[string])(nil)
	_ interface{ SetConfigValue(any) error } = (*Secret[string])(nil)
	_                                        = (*Secret[string]).SetConfigValue
	_                                        = (*Secret[string]).UnmarshalText
)
