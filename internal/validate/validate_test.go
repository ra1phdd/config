package validate

import (
	"reflect"
	"testing"
)

func TestStrictReportsUnknownFields(t *testing.T) {
	t.Parallel()
	type target struct {
		Name string `config:"name"`
	}
	issues := Strict(map[string]any{"unknown": true}, reflect.TypeFor[target](), "")
	if len(issues) != 1 || issues[0].Path != "unknown" {
		t.Fatalf("issues = %#v", issues)
	}
}
