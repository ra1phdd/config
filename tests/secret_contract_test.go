package config_test

import (
	"testing"

	"github.com/ra1phdd/config/secret"
)

func TestSecretPackageContract(t *testing.T) {
	t.Parallel()
	var value secret.Secret[string]
	if err := value.SetConfigValue("value"); err != nil {
		t.Fatal(err)
	}
	if err := value.UnmarshalText([]byte("next")); err != nil {
		t.Fatal(err)
	}
	if value.Reveal() != "next" || !value.IsSet() {
		t.Fatalf("secret = %q, set=%v", value.Reveal(), value.IsSet())
	}
}
