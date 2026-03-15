package configaccess

import (
	"reflect"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestBuildKeyEntriesMap_EntryKeysOverrideLegacyAPIKeys(t *testing.T) {
	cfg := &sdkconfig.SDKConfig{
		APIKeys: []string{"disabled-dup", "restricted-dup", "legacy-only"},
		APIKeyEntries: []internalconfig.APIKeyEntry{
			{Key: "disabled-dup", Disabled: true},
			{Key: "restricted-dup", AllowedModels: []string{"gpt-5"}},
		},
	}

	got := buildKeyEntriesMap(cfg)

	if _, exists := got["disabled-dup"]; exists {
		t.Fatal("expected disabled api-key-entry to block the same legacy api-key")
	}

	restricted, exists := got["restricted-dup"]
	if !exists {
		t.Fatal("expected api-key-entry to remain active for duplicate key")
	}
	if !reflect.DeepEqual(restricted.AllowedModels, []string{"gpt-5"}) {
		t.Fatalf("expected restricted models to come from api-key-entry, got %#v", restricted.AllowedModels)
	}

	if _, exists := got["legacy-only"]; !exists {
		t.Fatal("expected unrelated legacy api-key to remain available")
	}
}
