package source

import (
	"context"
	"testing"

	"github.com/ra1phdd/config"
)

type textParser struct{}

const originalValue = "original"

func (textParser) Parse(data []byte) (config.Layer, error) {
	return config.Layer{Data: map[string]any{"value": string(data)}}, nil
}

func TestBytesCopiesInputBeforeLoad(t *testing.T) {
	t.Parallel()
	data := []byte(originalValue)
	configured := Bytes(data, textParser{}, "bytes")
	copy(data, "changed!")
	layer, err := configured.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := layer.Data["value"]; got != originalValue {
		t.Fatalf("parsed value = %v, want original", got)
	}
}

func TestValuesCopiesInputAndLoadedLayers(t *testing.T) {
	t.Parallel()
	input := map[string]any{"nested": map[string]any{"name": "original"}}
	configured := Values("values", input)
	nested, ok := input["nested"].(map[string]any)
	if !ok {
		t.Fatal("nested value is not a map")
	}
	nested["name"] = "changed input"
	first, err := configured.Load(context.Background())
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	firstNested, ok := first.Data["nested"].(map[string]any)
	if !ok {
		t.Fatal("first nested value is not a map")
	}
	firstNested["name"] = "changed layer"
	second, err := configured.Load(context.Background())
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	secondNested, ok := second.Data["nested"].(map[string]any)
	if !ok {
		t.Fatal("second nested value is not a map")
	}
	if got := secondNested["name"]; got != originalValue {
		t.Fatalf("loaded value = %v, want original", got)
	}
}
