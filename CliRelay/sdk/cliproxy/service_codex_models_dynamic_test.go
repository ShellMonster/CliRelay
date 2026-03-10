package cliproxy

import (
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestRegisterModelsForAuth_CodexFallbackToStaticWhenDynamicFails(t *testing.T) {
	service := &Service{
		cfg: &config.Config{},
	}
	auth := &coreauth.Auth{
		ID:       "codex-dynamic-fallback",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "test-key",
			"base_url": "http://127.0.0.1:1/v1",
		},
	}

	reg := registry.GetGlobalRegistry()
	reg.UnregisterClient(auth.ID)
	t.Cleanup(func() { reg.UnregisterClient(auth.ID) })

	service.registerModelsForAuth(auth)
	models := reg.GetModelsForClient(auth.ID)
	if len(models) == 0 {
		t.Fatal("expected static fallback models for codex auth")
	}

	found := false
	for _, model := range models {
		if model != nil && strings.EqualFold(model.ID, "gpt-5.3-codex") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected static fallback to contain gpt-5.3-codex")
	}
}
