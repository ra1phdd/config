package json

import (
	"context"
	"reflect"
	"testing"

	"github.com/ra1phdd/config/v2"
)

func TestBytesParserLoadsWithoutOtherFormatPackages(t *testing.T) {
	t.Parallel()
	type target struct {
		Count int64 `config:"count"`
	}
	got, err := config.Load[target](context.Background(), config.WithSource(Bytes([]byte(`{"count":9223372036854775807}`))))
	if err != nil || got.Count != 9223372036854775807 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestJSONTagFallbackAndConfigTagPrecedence(t *testing.T) {
	t.Parallel()
	type target struct {
		Fallback string `json:"log_level"`
		Explicit string `config:"logging" json:"json_logging"`
		Priority string `config:"preferred" json:"legacy"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(Bytes([]byte(`{"log_level":"debug","json_logging":"trace","preferred":"config","legacy":"format"}`))))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Fallback != "debug" || cfg.Explicit != "trace" || cfg.Priority != "config" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestJSONTagsNormalizeNestedCollectionsAndRespectSkip(t *testing.T) {
	t.Parallel()
	type nested struct {
		Value   string `json:"wire_value"`
		Ignored string `json:"-"`
	}
	type target struct {
		Nested nested   `json:"nested_block"`
		Items  []nested `json:"records"`
	}
	cfg, err := config.Load[target](context.Background(), config.WithSource(Bytes([]byte(
		`{"nested_block":{"wire_value":"one","Ignored":"hidden"},"records":[{"wire_value":"two"}]}`,
	))))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Nested.Value != "one" || cfg.Nested.Ignored != "" || len(cfg.Items) != 1 || cfg.Items[0].Value != "two" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestParserHandlesNestedAndEscapedValues(t *testing.T) {
	t.Parallel()
	layer, err := (Parser{}).Parse([]byte(`{"message":"a\/b","items":[{"count":9223372036854775807},null]}`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if layer.Data["message"] != "a/b" {
		t.Fatalf("message = %#v, want a/b", layer.Data["message"])
	}
	want := []any{map[string]any{"count": jsonNumber("9223372036854775807")}, nil}
	if !reflect.DeepEqual(layer.Data["items"], want) {
		t.Fatalf("items = %#v, want %#v", layer.Data["items"], want)
	}
}

func TestBytesReturnsIndependentLayers(t *testing.T) {
	t.Parallel()
	source := Bytes([]byte(`{"nested":{"name":"one"}}`))
	first, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nested, ok := first.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested value type = %T", first.Data["nested"])
	}
	nested["name"] = "changed"

	second, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nested, ok = second.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested value type = %T", second.Data["nested"])
	}
	if got := nested["name"]; got != "one" {
		t.Fatalf("cached source reused mutable data: %v", got)
	}
}
