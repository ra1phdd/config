// Package config loads typed configuration from ordered sources.
package config

import (
	"context"
	"encoding"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	configmerge "github.com/ra1phdd/config/internal/merge"
	configpath "github.com/ra1phdd/config/internal/path"
	"github.com/ra1phdd/config/internal/schema"
	configvalidate "github.com/ra1phdd/config/internal/validate"
)

// Option configures a typed Loader.
type Option func(*options)

type options struct {
	defaults   any
	sources    []sourceOption
	validators []any
	decoders   []decoderEntry
}

type sourceOption struct {
	source   Source
	optional bool
}

// Decoder converts one source value into a typed configuration value.
type Decoder[T any] func(any) (T, error)

type decoderEntry struct {
	typ     reflect.Type
	decode  func(any, reflect.Value) error
	nilFunc bool
}

// WithSource appends a source; later sources override earlier values.
func WithSource(source Source) Option {
	return func(o *options) { o.sources = append(o.sources, sourceOption{source: source}) }
}

// WithOptionalSource ignores ErrNotFound from the source.
func WithOptionalSource(source Source) Option {
	return func(o *options) { o.sources = append(o.sources, sourceOption{source: source, optional: true}) }
}

// WithDefaults supplies a fresh starting value for each Load call.
func WithDefaults[T any](defaults func() T) Option {
	return func(o *options) { o.defaults = defaults }
}

// Validator checks a fully loaded value.
type Validator[T any] func(context.Context, T) error

// ValueSetter is implemented by optional value packages that decode
// normalized source values without coupling those packages to the core loader.
type ValueSetter interface {
	SetConfigValue(any) error
}

// WithValidator appends a validator that runs after required-field checks.
func WithValidator[T any](validator Validator[T]) Option {
	return func(o *options) { o.validators = append(o.validators, validator) }
}

// WithDecoder registers a decoder for one loader and one exact destination type.
// The registry is local to the loader and never changes global state.
func WithDecoder[T any](decoder Decoder[T]) Option {
	return func(o *options) {
		o.decoders = append(o.decoders, decoderEntry{
			typ:     reflect.TypeFor[T](),
			nilFunc: decoder == nil,
			decode: func(input any, dst reflect.Value) error {
				value, err := decoder(input)
				if err != nil {
					return err
				}
				result := reflect.ValueOf(value)
				if !result.IsValid() {
					dst.SetZero()
					return nil
				}
				if !result.Type().AssignableTo(dst.Type()) {
					return fmt.Errorf("decoder returned %s, want %s", result.Type(), dst.Type())
				}
				dst.Set(result)
				return nil
			},
		})
	}
}

// Loader is an immutable reusable configuration plan.
type Loader[T any] struct {
	options          options
	requiresPresence bool
}

type fieldDescriptor = schema.Field

func fieldsFor(t reflect.Type) []fieldDescriptor { return schema.Fields(t) }

func typeHasRequired(t reflect.Type, seen map[reflect.Type]bool) bool {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true

	if schema.IsStruct(t) {
		for _, field := range fieldsFor(t) {
			if field.Skip {
				continue
			}
			if field.Required || typeHasRequired(field.StructField.Type, seen) {
				return true
			}
		}
		return false
	}
	if t.Kind() == reflect.Interface {
		return true
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		return typeHasRequired(t.Elem(), seen)
	}
	return false
}

// New validates loader options and returns a reusable loader.
func New[T any](opts ...Option) (*Loader[T], error) {
	var o options
	for _, option := range opts {
		if option != nil {
			option(&o)
		}
	}
	want := reflect.TypeFor[T]()
	if o.defaults != nil {
		fn, ok := o.defaults.(func() T)
		if !ok {
			return nil, fmt.Errorf("defaults type does not match %v", want)
		}
		if reflect.ValueOf(fn).IsNil() {
			return nil, errors.New("nil defaults function")
		}
		o.defaults = fn
	}
	for _, validator := range o.validators {
		fn, ok := validator.(Validator[T])
		if !ok {
			return nil, fmt.Errorf("validator type does not match %v", want)
		}
		if reflect.ValueOf(fn).IsNil() {
			return nil, errors.New("nil validator function")
		}
	}
	for _, decoder := range o.decoders {
		if decoder.nilFunc {
			return nil, fmt.Errorf("nil decoder for %v", decoder.typ)
		}
	}
	for _, source := range o.sources {
		if isNilValue(source.source) {
			return nil, errors.New("nil config source")
		}
	}
	if want.Kind() == reflect.Struct {
		fieldsFor(want)
	}
	requiresPresence := typeHasRequired(want, make(map[reflect.Type]bool))
	return &Loader[T]{options: o, requiresPresence: requiresPresence}, nil
}

// Load constructs a fresh configuration value from defaults and ordered sources.
func (l *Loader[T]) Load(ctx context.Context) (T, error) {
	var zero T
	if err := ctx.Err(); err != nil {
		return zero, fmt.Errorf("loading configuration: %w", err)
	}
	cfg := zero
	if defaults, ok := l.options.defaults.(func() T); ok {
		cfg = defaults()
	}
	target := reflect.ValueOf(&cfg).Elem()
	var present map[string]bool
	merged, origins, decoders, errs, err := l.loadSources(ctx, target.Type())
	if err != nil {
		return zero, err
	}
	if l.requiresPresence {
		present = make(map[string]bool)
		configvalidate.MarkPresent(merged, "", present)
	}
	if len(merged) != 0 {
		if err := decodeValue(merged, target, "", "merged", false, decoders, l.options.decoders); err != nil {
			annotateSources(err, origins)
			errs.add(err)
		}
	}
	l.addRequiredErrors(target, present, &errs)
	if errs.Err() == nil {
		if err := l.runValidators(ctx, cfg, &errs); err != nil {
			return zero, err
		}
	}
	if err := errs.Err(); err != nil {
		return zero, err
	}
	return cfg, nil
}

func (l *Loader[T]) addRequiredErrors(target reflect.Value, present map[string]bool, errs *Errors) {
	if !l.requiresPresence {
		return
	}
	if issues := configvalidate.Required(target, "", present); len(issues) > 0 {
		errs.add(validationIssues(issues, "merged", "required"))
	}
}

func (l *Loader[T]) runValidators(ctx context.Context, cfg T, errs *Errors) error {
	for _, validator := range l.options.validators {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("validating configuration: %w", err)
		}
		fn, ok := validator.(Validator[T])
		if !ok {
			return errors.New("invalid validator type")
		}
		if err := fn(ctx, cfg); err != nil {
			errs.add(&FieldError{Source: "validator", Op: "validate", Err: err})
		}
	}
	return nil
}

// Clone returns an independent snapshot using the library's snapshot cloning rules.
// Reload adapters use it to prevent callers from mutating stored configuration.
func (l *Loader[T]) Clone(value T) T { return cloneSnapshot(value) }

func (l *Loader[T]) loadSources(ctx context.Context, target reflect.Type) (map[string]any, map[string]string, map[string]LayerDecoder, Errors, error) {
	merged := make(map[string]any)
	origins := make(map[string]string)
	decoders := make(map[string]LayerDecoder)
	var errs Errors
	for _, configured := range l.options.sources {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, errs, fmt.Errorf("loading configuration: %w", err)
		}
		layer, skip, err := loadSource(ctx, configured, target)
		if skip {
			continue
		}
		if err != nil {
			if fieldErr, ok := errors.AsType[*FieldError](err); ok {
				errs.add(fieldErr)
			} else {
				errs.add(&FieldError{Source: configured.source.Name(), Op: "load", Err: err})
			}
			continue
		}
		mergeSourceLayer(layer, configured.source.Name(), target, merged, origins, decoders, &errs)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, errs, fmt.Errorf("loading configuration: %w", err)
	}
	return merged, origins, decoders, errs, nil
}

func loadSource(ctx context.Context, configured sourceOption, target reflect.Type) (Layer, bool, error) {
	var layer Layer
	var err error
	if typed, ok := configured.source.(TargetSource); ok {
		layer, err = typed.LoadFor(ctx, target)
	} else {
		layer, err = configured.source.Load(ctx)
	}
	if configured.optional && errors.Is(err, ErrNotFound) {
		return Layer{}, true, nil
	}
	return layer, false, wrapSourceError("loading source", err)
}

func mergeSourceLayer(layer Layer, sourceName string, target reflect.Type, merged map[string]any, origins map[string]string, decoders map[string]LayerDecoder, errs *Errors) {
	if layer.Name == "" {
		layer.Name = sourceName
	}
	origin := layer.Name
	if layer.Strict {
		if issues := configvalidate.Strict(layer.Data, target, ""); len(issues) > 0 {
			errs.add(validationIssues(issues, origin, "decode"))
		}
	}
	if layer.Decoder != nil {
		decoders[""] = layer.Decoder
	}
	configmerge.Merge(merged, layer.Data, "", origin, origins, decoders, layer.Decoder)
}

func validationIssues(issues []configvalidate.Issue, source, operation string) error {
	var errs Errors
	for _, issue := range issues {
		errs.add(invalid(source, issue.Path, operation, issue.Err))
	}
	return errs.Err()
}

// Load applies options and loads one configuration value.
func Load[T any](ctx context.Context, opts ...Option) (T, error) {
	loader, err := New[T](opts...)
	if err != nil {
		var zero T
		return zero, err
	}
	return loader.Load(ctx)
}

var (
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
	durationType        = reflect.TypeFor[time.Duration]()
	urlType             = reflect.TypeFor[url.URL]()
	ipType              = reflect.TypeFor[net.IP]()
)

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func decodeValue(input any, dst reflect.Value, path, source string, strict bool, decoders map[string]LayerDecoder, registry []decoderEntry) error {
	return decodeValueSeen(input, dst, path, source, strict, decoders, registry, make(map[cloneVisit]bool))
}

func decodeVisit(value reflect.Value) (cloneVisit, bool) {
	if !value.IsValid() {
		return cloneVisit{}, false
	}
	switch value.Kind() {
	case reflect.Map:
		return cloneVisit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}, true
	case reflect.Slice:
		return cloneVisit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer(), len: value.Len(), cap: value.Cap()}, true
	case reflect.Pointer:
		return cloneVisit{typ: value.Type(), kind: value.Kind(), ptr: value.Pointer()}, true
	default:
		return cloneVisit{}, false
	}
}

func decodeValueSeen(input any, dst reflect.Value, path, source string, strict bool, decoders map[string]LayerDecoder, registry []decoderEntry, active map[cloneVisit]bool) error { //nolint:gocognit // Central dispatcher for all supported destination kinds.
	if !dst.CanSet() {
		return invalid(source, path, "decode", errors.New("field is not settable"))
	}
	if input == nil {
		if dst.Kind() == reflect.Pointer || dst.Kind() == reflect.Interface || dst.Kind() == reflect.Map || dst.Kind() == reflect.Slice {
			dst.SetZero()
			return nil
		}
		return invalid(source, path, "decode", fmt.Errorf("null is not valid for %s", dst.Type()))
	}
	inputValue := reflect.ValueOf(input)
	if inputValue.IsValid() && inputValue.Type().AssignableTo(dst.Type()) {
		dst.Set(inputValue)
		return nil
	}
	if key, ok := decodeVisit(inputValue); ok {
		if active[key] {
			return invalid(source, path, "decode", errors.New("cyclic configuration value"))
		}
		active[key] = true
		defer delete(active, key)
	}
	if dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return decodeValueSeen(input, dst.Elem(), path, source, strict, decoders, registry, active)
	}
	if dst.CanAddr() {
		if setter, ok := reflect.TypeAssert[ValueSetter](dst.Addr()); ok {
			if err := setter.SetConfigValue(input); err != nil {
				return invalid(source, path, "decode", errors.New("invalid custom configuration value"))
			}
			return nil
		}
	}
	if dst.CanAddr() && dst.Addr().Type().Implements(textUnmarshalerType) {
		text, ok := input.(string)
		if !ok {
			return invalid(source, path, "decode", fmt.Errorf("expected text for %s", dst.Type()))
		}
		unmarshaler, ok := reflect.TypeAssert[encoding.TextUnmarshaler](dst.Addr())
		if !ok {
			return invalid(source, path, "decode", errors.New("invalid text unmarshaler"))
		}
		if err := unmarshaler.UnmarshalText([]byte(text)); err != nil {
			return invalid(source, path, "decode", errors.New("invalid text value"))
		}
		return nil
	}
	if decoder := decoders[path]; decoder != nil && dst.CanAddr() {
		handled, err := decoder(input, dst.Addr().Interface())
		if handled {
			if err != nil {
				return invalid(source, path, "decode", err)
			}
			return nil
		}
	}
	for _, decoder := range registry {
		if decoder.typ != dst.Type() {
			continue
		}
		if err := decoder.decode(input, dst); err != nil {
			return invalid(source, path, "decode", err)
		}
		return nil
	}
	if dst.Type() == durationType {
		text, ok := input.(string)
		if !ok {
			return invalid(source, path, "decode", errors.New("expected duration string"))
		}
		value, err := time.ParseDuration(text)
		if err != nil {
			return invalid(source, path, "decode", errors.New("invalid duration"))
		}
		dst.SetInt(int64(value))
		return nil
	}
	if dst.Type() == urlType || dst.Type() == ipType { //nolint:nestif // URL and IP share one scalar conversion branch.
		text, ok := input.(string)
		if !ok {
			return invalid(source, path, "decode", fmt.Errorf("expected text for %s", dst.Type()))
		}
		if dst.Type() == urlType {
			value, err := url.Parse(text)
			if err != nil {
				return invalid(source, path, "decode", errors.New("invalid URL"))
			}
			dst.Set(reflect.ValueOf(*value))
		} else {
			value := net.ParseIP(text)
			if value == nil {
				return invalid(source, path, "decode", errors.New("invalid IP address"))
			}
			dst.Set(reflect.ValueOf(value))
		}
		return nil
	}
	switch dst.Kind() {
	case reflect.Struct:
		values, ok := input.(map[string]any)
		if !ok {
			return invalid(source, path, "decode", fmt.Errorf("expected object for %s", dst.Type()))
		}
		fields := fieldsFor(dst.Type())
		var known map[string]struct{}
		if strict {
			known = make(map[string]struct{}, len(fields))
		}
		var errs Errors
		for _, descriptor := range fields {
			if descriptor.Skip {
				continue
			}
			if strict {
				known[descriptor.Name] = struct{}{}
			}
			value, exists := values[descriptor.Name]
			if !exists {
				continue
			}
			fieldPath := joinPath(path, descriptor.Name)
			if err := decodeValueSeen(value, dst.Field(descriptor.Index), fieldPath, source, strict, decoders, registry, active); err != nil {
				errs.add(err)
			}
		}
		if strict {
			for _, name := range sortedKeys(values) {
				if _, ok := known[name]; !ok {
					errs.add(invalid(source, joinPath(path, name), "decode", errors.New("unknown field")))
				}
			}
		}
		return errs.Err()
	case reflect.Slice:
		if text, ok := input.(string); ok {
			parts, err := parseCSV(text)
			if err != nil {
				return invalid(source, path, "decode", err)
			}
			out := reflect.MakeSlice(dst.Type(), len(parts), len(parts))
			var errs Errors
			for i, part := range parts {
				if err := decodeValueSeen(part, out.Index(i), fmt.Sprintf("%s[%d]", path, i), source, strict, decoders, registry, active); err != nil {
					errs.add(err)
				}
			}
			dst.Set(out)
			return errs.Err()
		}
		values, ok := input.([]any)
		if !ok {
			return invalid(source, path, "decode", fmt.Errorf("expected array for %s", dst.Type()))
		}
		out := reflect.MakeSlice(dst.Type(), len(values), len(values))
		var errs Errors
		for i, value := range values {
			if err := decodeValueSeen(value, out.Index(i), fmt.Sprintf("%s[%d]", path, i), source, strict, decoders, registry, active); err != nil {
				errs.add(err)
			}
		}
		dst.Set(out)
		return errs.Err()
	case reflect.Array:
		if text, ok := input.(string); ok {
			parts, err := parseCSV(text)
			if err != nil || len(parts) != dst.Len() {
				return invalid(source, path, "decode", fmt.Errorf("expected array of length %d", dst.Len()))
			}
			var errs Errors
			for i, part := range parts {
				if err := decodeValueSeen(part, dst.Index(i), fmt.Sprintf("%s[%d]", path, i), source, strict, decoders, registry, active); err != nil {
					errs.add(err)
				}
			}
			return errs.Err()
		}
		values, ok := input.([]any)
		if !ok || len(values) != dst.Len() {
			return invalid(source, path, "decode", fmt.Errorf("expected array of length %d", dst.Len()))
		}
		var errs Errors
		for i, value := range values {
			if err := decodeValueSeen(value, dst.Index(i), fmt.Sprintf("%s[%d]", path, i), source, strict, decoders, registry, active); err != nil {
				errs.add(err)
			}
		}
		return errs.Err()
	case reflect.Map:
		values, ok := input.(map[string]any)
		if !ok || dst.Type().Key().Kind() != reflect.String {
			return invalid(source, path, "decode", fmt.Errorf("expected string-keyed object for %s", dst.Type()))
		}
		out := reflect.MakeMapWithSize(dst.Type(), dst.Len()+len(values))
		iter := dst.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), iter.Value())
		}
		var errs Errors
		for _, key := range sortedKeys(values) {
			value := values[key]
			item := reflect.New(dst.Type().Elem()).Elem()
			if existing := dst.MapIndex(reflect.ValueOf(key).Convert(dst.Type().Key())); existing.IsValid() {
				item.Set(existing)
			}
			if err := decodeValueSeen(value, item, joinPath(path, key), source, strict, decoders, registry, active); err != nil {
				errs.add(err)
				continue
			}
			out.SetMapIndex(reflect.ValueOf(key).Convert(dst.Type().Key()), item)
		}
		dst.Set(out)
		return errs.Err()
	case reflect.String:
		text, ok := input.(string)
		if !ok {
			return invalid(source, path, "decode", fmt.Errorf("expected string for %s", dst.Type()))
		}
		dst.SetString(text)
	case reflect.Bool:
		value, err := parseBool(input)
		if err != nil {
			return invalid(source, path, "decode", errors.New("invalid boolean"))
		}
		dst.SetBool(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := parseInt(input, dst.Type().Bits())
		if err != nil {
			return invalid(source, path, "decode", fmt.Errorf("invalid %s", dst.Type()))
		}
		dst.SetInt(value)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		value, err := parseUint(input, dst.Type().Bits())
		if err != nil {
			return invalid(source, path, "decode", fmt.Errorf("invalid %s", dst.Type()))
		}
		dst.SetUint(value)
	case reflect.Float32, reflect.Float64:
		value, err := parseFloat(input, dst.Type().Bits())
		if err != nil {
			return invalid(source, path, "decode", fmt.Errorf("invalid %s", dst.Type()))
		}
		dst.SetFloat(value)
	case reflect.Interface:
		value := reflect.ValueOf(input)
		if !value.IsValid() {
			dst.SetZero()
			return nil
		}
		if !value.Type().AssignableTo(dst.Type()) && !value.Type().Implements(dst.Type()) {
			return invalid(source, path, "decode", fmt.Errorf("%s does not implement %s", value.Type(), dst.Type()))
		}
		dst.Set(value)
	default:
		return invalid(source, path, "decode", fmt.Errorf("unsupported type %s", dst.Type()))
	}
	return nil
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func annotateSources(err error, origins map[string]string) {
	{
		var typed *FieldError
		var typed1 *Errors
		switch {
		case errors.As(err, &typed):
			if typed.Source == "merged" {
				if source := originForPath(typed.Path, origins); source != "" {
					typed.Source = source
				}
			}
		case errors.As(err, &typed1):
			for _, item := range typed1.list {
				annotateSources(item, origins)
			}
		}
	}
}

func originForPath(path string, origins map[string]string) string {
	return configpath.Origin(path, origins)
}

func parseBool(input any) (bool, error) {
	if value, ok := input.(bool); ok {
		return value, nil
	}
	text, ok := input.(string)
	if !ok {
		return false, errors.New("not text")
	}
	value, err := strconv.ParseBool(text)
	if err != nil {
		return false, fmt.Errorf("parse boolean: %w", err)
	}
	return value, nil
}

func parseInt(input any, bits int) (int64, error) {
	var value int64
	var err error
	if text, ok := input.(string); ok {
		value, err = strconv.ParseInt(text, 10, bits)
	} else {
		value, err = strconv.ParseInt(fmt.Sprint(input), 10, bits)
	}
	if err != nil {
		return 0, fmt.Errorf("parse integer: %w", err)
	}
	return value, nil
}

func parseUint(input any, bits int) (uint64, error) {
	var value uint64
	var err error
	if text, ok := input.(string); ok {
		value, err = strconv.ParseUint(text, 10, bits)
	} else {
		value, err = strconv.ParseUint(fmt.Sprint(input), 10, bits)
	}
	if err != nil {
		return 0, fmt.Errorf("parse unsigned integer: %w", err)
	}
	return value, nil
}

func parseFloat(input any, bits int) (float64, error) {
	var value float64
	var err error
	if text, ok := input.(string); ok {
		value, err = strconv.ParseFloat(text, bits)
	} else {
		value, err = strconv.ParseFloat(fmt.Sprint(input), bits)
	}
	if err != nil {
		return 0, fmt.Errorf("parse float: %w", err)
	}
	return value, nil
}

func parseCSV(input string) ([]string, error) {
	if input == "" {
		return []string{}, nil
	}
	var out []string
	var current strings.Builder
	quoted := false
	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '"':
			if quoted && i+1 < len(input) && input[i+1] == '"' {
				current.WriteByte('"')
				i++
				continue
			}
			quoted = !quoted
		case ',':
			if !quoted {
				out = append(out, strings.TrimSpace(current.String()))
				current.Reset()
				continue
			}
			current.WriteByte(input[i])
		default:
			current.WriteByte(input[i])
		}
	}
	if quoted {
		return nil, errors.New("unterminated CSV quote")
	}
	out = append(out, strings.TrimSpace(current.String()))
	return out, nil
}

func joinPath(parent, name string) string {
	return configpath.Join(parent, name)
}

var _ = url.URL{}
