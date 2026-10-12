package config

import "testing"

func TestStrictFreeProvidersDefaultsOff(t *testing.T) {
	t.Setenv("STRICT_FREE_PROVIDERS", "")
	if Load().StrictFreeProviders {
		t.Fatal("strict mode must default to off")
	}
	t.Setenv("STRICT_FREE_PROVIDERS", "true")
	if !Load().StrictFreeProviders {
		t.Fatal("strict mode was not enabled")
	}
}
