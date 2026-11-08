package secret

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestSecretRedactsAndDecodesValues(t *testing.T) {
	t.Parallel()
	var value Secret[map[string]string]
	if err := value.SetConfigValue(map[string]any{"token": "value"}); err != nil {
		t.Fatalf("SetConfigValue() error = %v", err)
	}
	if value.Reveal()["token"] != "value" || !value.IsSet() {
		t.Fatalf("secret value = %#v, set=%v", value.Reveal(), value.IsSet())
	}
	if got := fmt.Sprint(value); got != "[REDACTED]" {
		t.Fatalf("String() = %q", got)
	}
	data, err := json.Marshal(value)
	if err != nil || string(data) != `"[REDACTED]"` {
		t.Fatalf("MarshalJSON() = %s, %v", data, err)
	}
}

func TestSecretClonesNilInterfaceValue(t *testing.T) {
	t.Parallel()
	var value Secret[any]
	if err := value.SetConfigValue(nil); err != nil {
		t.Fatal(err)
	}
	cloned, ok := value.CloneConfigValue().(Secret[any])
	if !ok || !cloned.IsSet() || cloned.Reveal() != nil {
		t.Fatalf("clone = %#v, set=%v", cloned.Reveal(), cloned.IsSet())
	}
}
