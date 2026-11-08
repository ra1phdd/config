package schema

import (
	"reflect"
	"testing"
)

func TestFieldsParseConfigNamesAndRequiredOption(t *testing.T) {
	t.Parallel()
	type nested struct {
		Host string `config:"host,required" env:"APP_HOST"`
		Skip string `config:"-"`
	}
	typ := reflect.TypeFor[nested]()
	fields := Fields(typ)
	if len(fields) != 2 {
		t.Fatalf("Fields() returned %d fields, want 2", len(fields))
	}
	if fields[0].Index != 0 || fields[0].Name != "host" || !fields[0].Required || fields[0].Skip {
		t.Fatalf("first field metadata = %+v", fields[0])
	}
	if fields[1].Index != 1 || !fields[1].Skip {
		t.Fatalf("skipped field metadata = %+v", fields[1])
	}
}
