package management

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestBuildUserAgentRoutingOptions_GroupsProvidersAndModels(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	now := time.Now().Unix()

	auths := []*coreauth.Auth{
		{ID: "codex-auth-1", Provider: "codex", Label: "Primary Codex"},
		{
			ID:       "compat-auth-1",
			Provider: "github-openai-compatible",
			Label:    "Github OpenAI Compatible",
			Attributes: map[string]string{
				"compat_name":  "Github OpenAI Compatible",
				"provider_key": "github-openai-compatible",
			},
		},
	}

	for _, auth := range auths {
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth %s: %v", auth.ID, err)
		}
	}

	modelRegistry := registry.GetGlobalRegistry()
	modelRegistry.RegisterClient("codex-auth-1", "codex", []*registry.ModelInfo{
		{ID: "gpt-5", DisplayName: "GPT-5", Created: now},
		{ID: "gpt-5-mini", DisplayName: "GPT-5 Mini", Created: now},
	})
	modelRegistry.RegisterClient("compat-auth-1", "github-openai-compatible", []*registry.ModelInfo{
		{ID: "gpt-5", DisplayName: "GPT-5", Created: now},
		{ID: "o3", DisplayName: "o3", Created: now},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient("codex-auth-1")
		modelRegistry.UnregisterClient("compat-auth-1")
	})

	options := buildUserAgentRoutingOptions(&config.Config{}, manager)

	if len(options.Providers) != 2 {
		t.Fatalf("expected 2 provider options, got %#v", options.Providers)
	}
	if options.Providers[0].ID != "codex" {
		t.Fatalf("expected first provider to be codex, got %#v", options.Providers)
	}
	if options.Providers[1].ID != "openai-compatibility" {
		t.Fatalf("expected second provider to be openai-compatibility, got %#v", options.Providers)
	}

	gotModels := make(map[string]struct{}, len(options.Models))
	for _, model := range options.Models {
		gotModels[model.ID] = struct{}{}
	}
	for _, want := range []string{"gpt-5", "gpt-5-mini", "o3"} {
		if _, exists := gotModels[want]; !exists {
			t.Fatalf("expected model %q in %#v", want, options.Models)
		}
	}
}

func TestBuildUserAgentRoutingOptions_UsesConfiguredFallbackModels(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:       "codex-compat-auth-1",
		Provider: "codex-compat",
		Label:    "Opencode Compat",
		Prefix:   "compat",
		Attributes: map[string]string{
			"api_key":  "codex-compat-key",
			"base_url": "https://example.com/responses",
		},
	}

	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	cfg := &config.Config{
		CodexCompatKey: []config.CodexKey{{
			APIKey:  "codex-compat-key",
			BaseURL: "https://example.com/responses",
			Prefix:  "compat",
			Models: []config.CodexModel{
				{Name: "gpt-5.4", Alias: "gpt-5.4"},
				{Name: "gpt-5.4-mini", Alias: "gpt-5.4-mini"},
			},
		}},
	}

	options := buildUserAgentRoutingOptions(cfg, manager)

	if len(options.Providers) != 1 || options.Providers[0].ID != "codex-compat" {
		t.Fatalf("expected codex-compat provider option, got %#v", options.Providers)
	}

	gotModels := make(map[string]struct{}, len(options.Models))
	for _, model := range options.Models {
		gotModels[model.ID] = struct{}{}
	}
	for _, want := range []string{"gpt-5.4", "gpt-5.4-mini", "compat/gpt-5.4", "compat/gpt-5.4-mini"} {
		if _, exists := gotModels[want]; !exists {
			t.Fatalf("expected model %q in %#v", want, options.Models)
		}
	}
}
