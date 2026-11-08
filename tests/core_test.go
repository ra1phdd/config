//nolint:goconst // Fixtures intentionally retain literal field names and values.
package config_test

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type coreTestConfig struct {
	Name    string        `config:"name,required" env:"APP_NAME"`
	Timeout time.Duration `config:"timeout" env:"APP_TIMEOUT"`
	Nested  struct {
		Enabled bool `config:"enabled" env:"APP_ENABLED"`
	} `config:"nested"`
}

const (
	testDataKey      = "data"
	testFirstKey     = "first"
	testDefaultValue = "default"
	testNameKey      = "name"
	testLabelsKey    = "labels"
	testCountKey     = "count"
	testLowValue     = "low"
	testHighValue    = "high"
	testKeptKey      = "kept"
	testTwoKey       = "two"
	testFastValue    = "fast"
	testChangedKey   = "changed"
	testSecretValue  = "do-not-print"
)

type nilSource struct{}

func (*nilSource) Name() string { return "nil" }

func (*nilSource) Load(context.Context) (Layer, error) { return Layer{}, nil }

type fieldErrorSource struct{}

func (fieldErrorSource) Name() string { return "outer" }

func (fieldErrorSource) Load(context.Context) (Layer, error) {
	return Layer{}, &FieldError{Source: "inner", Path: "nested.value", Op: "parse", Err: ErrInvalid}
}

func TestNewRejectsTypedNilOptions(t *testing.T) {
	t.Parallel()
	var defaults func() coreTestConfig
	if _, err := New[coreTestConfig](WithDefaults(defaults)); err == nil {
		t.Fatal("typed-nil defaults function was accepted")
	}
	var validator Validator[coreTestConfig]
	if _, err := New[coreTestConfig](WithValidator(validator)); err == nil {
		t.Fatal("typed-nil validator was accepted")
	}
	var source *nilSource
	if _, err := New[coreTestConfig](WithSource(source)); err == nil {
		t.Fatal("typed-nil source was accepted")
	}
}

func TestLoadPreservesSourceFieldError(t *testing.T) {
	t.Parallel()
	_, err := Load[coreTestConfig](context.Background(), WithSource(fieldErrorSource{}))
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("expected FieldError, got %v", err)
	}
	if fieldErr.Source != "inner" || fieldErr.Path != "nested.value" || fieldErr.Op != "parse" {
		t.Fatalf("source error metadata = %+v", fieldErr)
	}
}

func TestLoadAcceptsTypedBytesValues(t *testing.T) {
	t.Parallel()
	type bytesConfig struct {
		Data []byte `config:"data"`
	}
	want := []byte{1, 2, 3}
	cfg, err := Load[bytesConfig](context.Background(), WithSource(Values("values", map[string]any{testDataKey: want})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(cfg.Data, want) {
		t.Fatalf("data = %v, want %v", cfg.Data, want)
	}
}

func TestRequiredFieldsInsideCollections(t *testing.T) {
	t.Parallel()
	type item struct {
		Name string `config:"name,required"`
	}
	type collectionConfig struct {
		Items map[string]item `config:"items"`
	}
	_, err := Load[collectionConfig](context.Background(), WithSource(Values("values", map[string]any{
		"items": map[string]any{testFirstKey: map[string]any{}},
	})))
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "items.first.name" {
		t.Fatalf("expected nested required error, got %v", err)
	}
}

func TestRequiredFieldsInsideInterfaceValues(t *testing.T) {
	t.Parallel()
	type item struct {
		Name *string `config:"name,required"`
	}
	type interfaceConfig struct {
		Value any `config:"value"`
	}

	_, err := Load[interfaceConfig](context.Background(), WithSource(Values("values", map[string]any{
		"value": item{},
	})))
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "value.name" {
		t.Fatalf("expected nested required error, got %v", err)
	}
}

func TestRequiredAcceptsExplicitZeroInTypedNestedValues(t *testing.T) {
	t.Parallel()
	type item struct {
		Name string `config:"name,required"`
	}
	type collectionConfig struct {
		Items map[string]item `config:"items"`
	}
	cfg, err := Load[collectionConfig](context.Background(), WithSource(Values("values", map[string]any{
		"items": map[string]item{"first": {}},
	})))
	if err != nil {
		t.Fatalf("explicit zero was rejected: %v", err)
	}
	if _, ok := cfg.Items["first"]; !ok {
		t.Fatalf("items = %#v", cfg.Items)
	}
}

func TestCloneSnapshotPreservesCycles(t *testing.T) {
	t.Parallel()
	type node struct {
		Next *node
	}
	value := &node{}
	value.Next = value
	clone := cloneSnapshot(value)
	if clone == value || clone.Next != clone {
		t.Fatalf("cycle was not cloned: original=%p clone=%p next=%p", value, clone, clone.Next)
	}
}

func TestValuesHandleCyclicMaps(t *testing.T) {
	t.Parallel()
	type cycleConfig struct {
		Data any `config:"data"`
	}
	values := map[string]any{}
	values["self"] = values
	cfg, err := Load[cycleConfig](context.Background(), WithSource(Values("values", map[string]any{"data": values})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	data, ok := cfg.Data.(map[string]any)
	if !ok || data["self"] == nil {
		t.Fatalf("cyclic value was not preserved: %#v", cfg.Data)
	}
}

func TestMergeHandlesCyclesAcrossSources(t *testing.T) {
	t.Parallel()
	type cycleConfig struct {
		Data any `config:"data"`
	}
	base := map[string]any{}
	base["self"] = base
	override := map[string]any{}
	override["self"] = override
	if _, err := Load[cycleConfig](context.Background(),
		WithSource(Values("base", map[string]any{"data": base})),
		WithSource(Values("override", map[string]any{"data": override})),
	); err != nil {
		t.Fatalf("cyclic merge error = %v", err)
	}
}

func TestDecodeRejectsCyclesForTypedStructs(t *testing.T) {
	t.Parallel()
	type node struct {
		Self *node `config:"self"`
	}
	values := map[string]any{}
	values["self"] = values
	_, err := Load[node](context.Background(), WithSource(Values("values", values)))
	if err == nil || !strings.Contains(err.Error(), "cyclic configuration value") {
		t.Fatalf("cyclic typed value error = %v", err)
	}
}

func TestLoadMergesDefaultsAndSources(t *testing.T) {
	t.Setenv("APP_NAME", "from-env")
	t.Setenv("APP_ENABLED", "true")

	cfg, err := Load[coreTestConfig](context.Background(),
		WithDefaults(func() coreTestConfig {
			return coreTestConfig{Name: testDefaultValue, Timeout: time.Second}
		}),
		WithSource(Values("base", map[string]any{
			testNameKey: "from-file",
			"nested":    map[string]any{"enabled": false},
		})),
		WithSource(Values("override", map[string]any{"timeout": "2s"})),
		WithSource(ProcessEnv("APP_")),
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Name != "from-env" || cfg.Timeout != 2*time.Second || !cfg.Nested.Enabled {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestValuesDeepCopiesTypedReferenceValues(t *testing.T) {
	t.Parallel()
	values := map[string]any{
		"labels": map[string]string{"one": "value"},
		"items":  []string{"first"},
	}
	source := Values("values", values)
	labelsInput, ok := values[testLabelsKey].(map[string]string)
	if !ok {
		t.Fatalf("labels input type = %T", values[testLabelsKey])
	}
	labelsInput["one"] = "mutated"
	itemsInput, ok := values["items"].([]string)
	if !ok {
		t.Fatalf("items input type = %T", values["items"])
	}
	itemsInput[0] = "mutated"
	layer, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	labels, ok := layer.Data[testLabelsKey].(map[string]string)
	if !ok {
		t.Fatalf("labels layer type = %T", layer.Data[testLabelsKey])
	}
	items, ok := layer.Data["items"].([]string)
	if !ok {
		t.Fatalf("items layer type = %T", layer.Data["items"])
	}
	if labels["one"] != "value" || items[0] != "first" {
		t.Fatalf("source retained caller aliases: %#v", layer.Data)
	}
	labels["one"] = "changed after load"
	items[0] = "changed after load"
	layerAgain, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	labelsAgain, ok := layerAgain.Data["labels"].(map[string]string)
	if !ok {
		t.Fatalf("labels again type = %T", layerAgain.Data["labels"])
	}
	itemsAgain, ok := layerAgain.Data["items"].([]string)
	if !ok {
		t.Fatalf("items again type = %T", layerAgain.Data["items"])
	}
	if labelsAgain["one"] != "value" || itemsAgain[0] != "first" {
		t.Fatalf("source load retained layer aliases: %#v", layerAgain.Data)
	}
}

func TestLoadRequiredAndFreshSnapshots(t *testing.T) {
	t.Parallel()
	loader, err := New[coreTestConfig](WithSource(Values("empty", map[string]any{})))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = loader.Load(context.Background())
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Path != "name" {
		t.Fatalf("expected required field error, got %v", err)
	}

	loader, err = New[coreTestConfig](WithDefaults(func() coreTestConfig {
		return coreTestConfig{Name: "default"}
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	a, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	a.Name = "mutated"
	b, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if b.Name != "default" {
		t.Fatalf("snapshot was reused: %+v", b)
	}
}

func TestRequiredAcceptsExplicitZeroValues(t *testing.T) {
	t.Parallel()
	type requiredConfig struct {
		Enabled bool   `config:"enabled,required"`
		Count   int    `config:"count,required"`
		Name    string `config:"name,required"`
	}
	cfg, err := Load[requiredConfig](context.Background(), WithSource(Values("values", map[string]any{
		"enabled":    false,
		testCountKey: 0,
		"name":       "",
	})))
	if err != nil {
		t.Fatalf("explicit zero values rejected: %v", err)
	}
	if cfg.Enabled || cfg.Count != 0 || cfg.Name != "" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestDecodeErrorReportsWinningSource(t *testing.T) {
	t.Parallel()
	type requiredInt struct {
		Count int `config:"count"`
	}
	_, err := Load[requiredInt](context.Background(), WithSource(Values("runtime", map[string]any{
		"count": "not-an-int",
	})))
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Source != "runtime" || fieldErr.Path != "count" {
		t.Fatalf("decode error = %v, want runtime/count", err)
	}
}

func TestJSONRejectsTrailingDocument(t *testing.T) {
	t.Parallel()
	_, err := Load[coreTestConfig](context.Background(), WithSource(Bytes("json", []byte(`{"name":"one"}{"name":"two"}`), FormatJSON)))
	if err == nil {
		t.Fatal("trailing JSON document was accepted")
	}
}

func TestJSONPreservesIntegerPrecision(t *testing.T) {
	t.Parallel()
	type integerConfig struct {
		Value int64 `config:"value"`
	}
	cfg, err := Load[integerConfig](context.Background(), WithSource(Bytes(
		"json", []byte(`{"value":9223372036854775807}`), FormatJSON,
	)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Value != int64(^uint64(0)>>1) {
		t.Fatalf("value = %d", cfg.Value)
	}
}

func TestCSVUnquotesValues(t *testing.T) {
	t.Parallel()
	type listConfig struct {
		Values []string `config:"values" env:"VALUES"`
	}
	cfg, err := Load[listConfig](context.Background(), WithSource(Env("env", "", func(key string) (string, bool) {
		return `one,"two,three","escaped ""quote"""`, key == "VALUES"
	})))
	if err != nil {
		t.Fatalf("CSV load error: %v", err)
	}
	want := []string{"one", "two,three", `escaped "quote"`}
	if !reflect.DeepEqual(cfg.Values, want) {
		t.Fatalf("CSV = %#v, want %#v", cfg.Values, want)
	}
}

func TestLaterSourceReplacesInvalidEarlierValue(t *testing.T) {
	t.Parallel()
	type layered struct {
		Count  int                          `config:"count"`
		Labels map[string]map[string]string `config:"labels"`
	}
	cfg, err := Load[layered](context.Background(),
		WithSource(Values("low", map[string]any{
			"count":       "not-an-int",
			testLabelsKey: map[string]any{"one": map[string]any{"a": testLowValue}},
		})),
		WithSource(Values("high", map[string]any{
			"count":       42,
			testLabelsKey: map[string]any{"one": map[string]any{"b": testHighValue}},
		})),
	)
	if err != nil {
		t.Fatalf("later source did not replace earlier value: %v", err)
	}
	if cfg.Count != 42 || cfg.Labels["one"]["a"] != "low" || cfg.Labels["one"]["b"] != "high" {
		t.Fatalf("merged config = %#v", cfg)
	}
}

func TestTypedMapsMergeRecursively(t *testing.T) {
	t.Parallel()
	type layered struct {
		Labels map[string]string `config:"labels"`
	}
	cfg, err := Load[layered](context.Background(),
		WithSource(Values("low", map[string]any{
			testLabelsKey: map[string]string{"one": testLowValue, testKeptKey: "yes"},
		})),
		WithSource(Values("high", map[string]any{
			testLabelsKey: map[string]string{"one": testHighValue, testTwoKey: testHighValue},
		})),
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := map[string]string{"one": "high", "kept": "yes", "two": "high"}
	if !reflect.DeepEqual(cfg.Labels, want) {
		t.Fatalf("typed map merge = %#v, want %#v", cfg.Labels, want)
	}
}

func TestIncompatibleInterfaceValueReturnsError(t *testing.T) {
	t.Parallel()
	type interfaceConfig struct {
		Value interface{ String() string } `config:"value"`
	}
	if _, err := Load[interfaceConfig](context.Background(), WithSource(Values("values", map[string]any{"value": 42}))); err == nil {
		t.Fatal("incompatible interface value did not return an error")
	}
}

func TestReloaderCurrentIsDeepCopied(t *testing.T) {
	t.Parallel()
	type mutableConfig struct {
		Labels map[string]string `config:"labels"`
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"labels":{"one":"value"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[mutableConfig](WithSource(File("config", path, FormatJSON)))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	first := <-updates
	first.Config.Labels["one"] = "mutated"
	current, ok := reloader.Current()
	if !ok || current.Labels["one"] != "value" {
		t.Fatalf("Current() aliases published snapshot: %+v", current)
	}
}

type testMode string

type jsonConfigValue struct {
	Value string
	Count int64
}

func (v *jsonConfigValue) UnmarshalJSON(data []byte) error {
	var value struct {
		Value string `json:"value"`
		Count int64  `json:"count"`
	}
	if err := jsonv2.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode custom JSON value: %w", err)
	}
	v.Value = "json:" + value.Value
	v.Count = value.Count
	return nil
}

type registryConfigValue string

func (m *testMode) UnmarshalText(text []byte) error {
	if string(text) != testFastValue && string(text) != "safe" {
		return errors.New("unsupported mode")
	}
	*m = testMode(text)
	return nil
}

func TestLoadDecodesCustomTypesAndMergesMaps(t *testing.T) {
	t.Parallel()
	type configValue struct {
		Endpoint *url.URL          `config:"endpoint"`
		Timeout  time.Duration     `config:"timeout"`
		Mode     testMode          `config:"mode"`
		Labels   map[string]string `config:"labels"`
		Names    []string          `config:"names"`
		Token    Secret[string]    `config:"token"`
	}
	cfg, err := Load[configValue](context.Background(),
		WithDefaults(func() configValue {
			return configValue{Labels: map[string]string{testKeptKey: "yes", testChangedKey: "old"}, Names: []string{testDefaultValue}}
		}),
		WithSource(Values("runtime", map[string]any{
			"endpoint": "https://example.test/path",
			"timeout":  "250ms",
			"mode":     "fast",
			"labels":   map[string]any{"changed": "new"},
			"names":    []any{"one", "two"},
			"token":    testSecretValue,
		})),
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Endpoint == nil || cfg.Endpoint.String() != "https://example.test/path" || cfg.Timeout != 250*time.Millisecond || cfg.Mode != "fast" {
		t.Fatalf("custom types not decoded: %+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Labels, map[string]string{"kept": "yes", "changed": "new"}) {
		t.Fatalf("map merge = %#v", cfg.Labels)
	}
	if !reflect.DeepEqual(cfg.Names, []string{"one", "two"}) {
		t.Fatalf("slice replacement = %#v", cfg.Names)
	}
	if !cfg.Token.IsSet() || cfg.Token.Reveal() != testSecretValue {
		t.Fatal("secret was not loaded")
	}
	if fmt.Sprint(cfg.Token) == "do-not-print" {
		t.Fatal("secret leaked through String formatting")
	}
}

func TestLoadUsesJSONSpecificUnmarshaler(t *testing.T) {
	t.Parallel()
	type jsonConfig struct {
		Value jsonConfigValue `config:"value"`
	}
	cfg, err := Load[jsonConfig](context.Background(), WithSource(Bytes("json", []byte(`{"value":{"value":"custom","count":9223372036854775807}}`), FormatJSON)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Value.Value != "json:custom" || cfg.Value.Count != int64(^uint64(0)>>1) {
		t.Fatalf("custom JSON value = %q", cfg.Value.Value)
	}
}

func TestLoadUsesExplicitDecoderRegistry(t *testing.T) {
	t.Parallel()
	type registryConfig struct {
		Value registryConfigValue `config:"value"`
	}
	loader, err := New[registryConfig](
		WithDecoder(func(input any) (registryConfigValue, error) {
			text, ok := input.(string)
			if !ok {
				return "", fmt.Errorf("expected string, got %T", input)
			}
			return registryConfigValue("registered:" + text), nil
		}),
		WithSource(Values("values", map[string]any{"value": "custom"})),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Value != "registered:custom" {
		t.Fatalf("registered value = %q", cfg.Value)
	}
}

func TestLoadSkipsValidatorsAfterDecodeError(t *testing.T) {
	t.Parallel()
	type validatorConfig struct {
		Count int `config:"count"`
	}
	called := false
	_, err := Load[validatorConfig](context.Background(),
		WithSource(Values("values", map[string]any{"count": "invalid"})),
		WithValidator(func(context.Context, validatorConfig) error {
			called = true
			return nil
		}),
	)
	if err == nil {
		t.Fatal("invalid value was accepted")
	}
	if called {
		t.Fatal("validator ran after a decode error")
	}
}

func TestStrictSourceRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := Load[coreTestConfig](context.Background(),
		WithSource(Bytes("strict", []byte(`{"name":"x","unknown":true}`), FormatJSON)),
	)
	if err != nil {
		t.Fatalf("non-strict source should ignore unknown keys: %v", err)
	}

	loader, err := New[coreTestConfig](WithSource(StrictBytes("strict", []byte(`{"name":"x","unknown":true}`), FormatJSON)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err = loader.Load(context.Background()); err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestStrictnessIsScopedToItsSource(t *testing.T) {
	t.Parallel()
	loader, err := New[coreTestConfig](
		WithSource(Bytes("loose", []byte(`{"name":"x","unknown":true}`), FormatJSON)),
		WithSource(StrictBytes("strict", []byte(`{"name":"y"}`), FormatJSON)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("unknown key from loose source was rejected: %v", err)
	}

	loader, err = New[coreTestConfig](
		WithSource(Bytes("loose", []byte(`{"name":"x"}`), FormatJSON)),
		WithSource(StrictBytes("strict", []byte(`{"name":"y","unknown":true}`), FormatJSON)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(context.Background()); err == nil {
		t.Fatal("unknown key from strict source was accepted")
	}
}

func TestStrictEnvironmentFileRetainsStrictFlag(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.env")
	if err := os.WriteFile(path, []byte("APP_NAME=ok\nAPP_UNKNOWN=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[coreTestConfig](WithSource(StrictFile("environment-file", path, FormatEnvFile)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(context.Background()); err == nil {
		t.Fatal("strict environment file accepted an unknown field")
	}
}

func TestReloaderClosePreventsFutureLifecycleOperations(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "watch")
	if err := os.WriteFile(path, []byte("watch"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[coreTestConfig](WithDefaults(func() coreTestConfig {
		return coreTestConfig{Name: "default"}
	}))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path))
	if err != nil {
		t.Fatal(err)
	}
	if err := reloader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := reloader.Start(context.Background()); !errors.Is(err, ErrReloaderClosed) {
		t.Fatalf("Start after Close() error = %v", err)
	}
	if _, err := reloader.Reload(context.Background()); !errors.Is(err, ErrReloaderClosed) {
		t.Fatalf("Reload after Close() error = %v", err)
	}
}

func TestReloaderPublishesValidChangesAndKeepsLastGood(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.any")
	if err := os.WriteFile(path, []byte(`{"name":"one"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[coreTestConfig](WithSource(File("settings", path, FormatJSON)))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(20*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	initial := <-updates
	if initial.Err != nil || initial.Config.Name != "one" {
		t.Fatalf("initial update = %+v", initial)
	}
	if err := os.WriteFile(path, []byte(`{"name":"two"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.Err != nil || update.Config.Name != "two" {
			t.Fatalf("valid update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for valid update")
	}
	if err := os.WriteFile(path, []byte(`{"name":`), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.Err == nil {
			t.Fatal("invalid update did not report an error")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for invalid update")
	}
	current, ok := reloader.Current()
	if !ok || current.Name != "two" {
		t.Fatalf("last good snapshot = %+v, ok=%v", current, ok)
	}
}

func TestReloaderSupportsExplicitReload(t *testing.T) {
	t.Parallel()
	type envConfig struct {
		Name string `config:"name" env:"APP_NAME"`
	}
	path := filepath.Join(t.TempDir(), "watch")
	if err := os.WriteFile(path, []byte("watch"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "one"
	loader, err := New[envConfig](WithSource(Env("env", "", func(key string) (string, bool) {
		return name, key == "APP_NAME"
	})))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if initial := <-updates; initial.Config.Name != "one" {
		t.Fatalf("initial config = %+v", initial.Config)
	}
	name = "two"
	update, err := reloader.Reload(context.Background())
	if err != nil || update.Config.Name != "two" {
		t.Fatalf("explicit reload = %+v, %v", update, err)
	}
	name = ""
	if _, err := reloader.Reload(context.Background()); err != nil {
		t.Fatalf("empty value should be valid: %v", err)
	}
}

func TestReloaderReloadRequiresStart(t *testing.T) {
	t.Parallel()
	loader, err := New[coreTestConfig]()
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithTrigger(idleTrigger{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reloader.Reload(context.Background()); !errors.Is(err, ErrReloaderNotStarted) {
		t.Fatalf("Reload() error = %v", err)
	}
}

type idleTrigger struct{}

func (idleTrigger) Start(context.Context) (<-chan error, error) {
	return make(chan error), nil
}

func TestEnvironmentFileUsesFieldTags(t *testing.T) {
	t.Parallel()
	got, err := Load[coreTestConfig](context.Background(),
		WithSource(Bytes("environment-file", []byte("APP_NAME=from-file\nAPP_ENABLED=true\nAPP_TIMEOUT=3s\n"), FormatEnvFile)),
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Name != "from-file" || !got.Nested.Enabled || got.Timeout != 3*time.Second {
		t.Fatalf("environment-file config = %+v", got)
	}
}

func TestEnvironmentFileSupportsSingleQuotedValues(t *testing.T) {
	t.Parallel()
	type environmentFileConfig struct {
		Name string `config:"name" env:"APP_NAME"`
	}
	got, err := Load[environmentFileConfig](context.Background(), WithSource(Bytes(
		"environment-file",
		[]byte("APP_NAME='value with spaces'\n"),
		FormatEnvFile,
	)))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Name != "value with spaces" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestOptionalMissingFileDoesNotFailLoad(t *testing.T) {
	t.Parallel()
	cfg, err := Load[coreTestConfig](context.Background(),
		WithDefaults(func() coreTestConfig { return coreTestConfig{Name: "default"} }),
		WithOptionalSource(File("optional", filepath.Join(t.TempDir(), "missing.json"), FormatJSON)),
	)
	if err != nil || cfg.Name != "default" {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
}

func TestInjectedEnvironmentSupportsNestedCSVAndMaps(t *testing.T) {
	t.Parallel()
	type envConfig struct {
		Ports  []int          `config:"ports" env:"PORTS"`
		Labels map[string]int `config:"labels" env:"LABELS"`
		URL    *url.URL       `config:"url" env:"SERVICE_URL"`
	}
	values := map[string]string{
		"PORTS":       "8000, 8001",
		"LABELS":      `{"blue":2}`,
		"SERVICE_URL": "https://service.test",
	}
	cfg, err := Load[envConfig](context.Background(), WithSource(Env("test-env", "", func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(cfg.Ports, []int{8000, 8001}) || cfg.Labels["blue"] != 2 || cfg.URL.String() != "https://service.test" {
		t.Fatalf("environment config = %+v", cfg)
	}
}

func TestLoadRejectsCancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load[coreTestConfig](ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}
}

func TestReloaderHandlesNilInterfaceSnapshot(t *testing.T) {
	t.Parallel()
	loader, err := New[any]()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "watch")
	if err := os.WriteFile(path, []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if update := <-updates; update.Err != nil || update.Config != nil {
		t.Fatalf("initial update = %+v", update)
	}
	if current, ok := reloader.Current(); !ok || current != nil {
		t.Fatalf("Current() = %#v, %v", current, ok)
	}
}

func TestReloaderStartRejectsCancelledContext(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"name":"one"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[coreTestConfig](WithSource(File("settings", path, FormatJSON)))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reloader.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
}

func TestReloaderDetectsAtomicRename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"name":"one"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[coreTestConfig](WithSource(File("settings", path, FormatJSON)))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(20*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if initial := <-updates; initial.Err != nil || initial.Config.Name != "one" {
		t.Fatalf("initial update = %+v", initial)
	}
	tmp := filepath.Join(dir, "settings.json.tmp")
	if err := os.WriteFile(tmp, []byte(`{"name":"renamed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.Err != nil || update.Config.Name != "renamed" {
			t.Fatalf("atomic rename update = %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for atomic rename update")
	}
}

func TestReloaderDeepCopiesSecretSnapshots(t *testing.T) {
	t.Parallel()
	type secretConfig struct {
		Token Secret[map[string]string] `config:"token"`
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"token":{"key":"value"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New[secretConfig](WithSource(File("settings", path, FormatJSON)))
	if err != nil {
		t.Fatal(err)
	}
	reloader, err := NewReloader(loader, WithWatchFile(path), WithDebounce(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	updates, err := reloader.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reloader.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	first := <-updates
	first.Config.Token.Reveal()["key"] = "mutated"
	current, ok := reloader.Current()
	if !ok || current.Token.Reveal()["key"] != "value" {
		t.Fatalf("Current() aliases secret snapshot: %+v", current.Token.Reveal())
	}
}
