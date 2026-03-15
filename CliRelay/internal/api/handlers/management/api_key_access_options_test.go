package management

import (
	"context"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestBuildAPIKeyAccessOptions_GroupsChannelsUnderProviderFamily(t *testing.T) {
	manager := coreauth.NewManager(nil, nil, nil)
	now := time.Now().Unix()

	auths := []*coreauth.Auth{
		{ID: "claude-auth-1", Provider: "claude", Label: "Kimi-For-Coding"},
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
	modelRegistry.RegisterClient("claude-auth-1", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4-5", Created: now},
	})
	modelRegistry.RegisterClient("compat-auth-1", "github-openai-compatible", []*registry.ModelInfo{
		{ID: "gpt-5.4", Created: now},
	})
	t.Cleanup(func() {
		modelRegistry.UnregisterClient("claude-auth-1")
		modelRegistry.UnregisterClient("compat-auth-1")
	})

	options := buildAPIKeyAccessOptions(&config.Config{}, manager)
	if len(options) != 2 {
		t.Fatalf("expected 2 provider groups, got %#v", options)
	}

	var claudeOption *apiKeyAccessProviderOption
	var compatOption *apiKeyAccessProviderOption
	for i := range options {
		switch options[i].Provider {
		case "claude":
			claudeOption = &options[i]
		case "openai-compatibility":
			compatOption = &options[i]
		}
	}

	if claudeOption == nil {
		t.Fatal("expected claude provider option")
	}
	if len(claudeOption.Channels) != 1 {
		t.Fatalf("expected 1 claude channel, got %#v", claudeOption.Channels)
	}
	if claudeOption.Channels[0].ID != "claude-auth-1" || claudeOption.Channels[0].Label != "Kimi-For-Coding" {
		t.Fatalf("unexpected claude channel option: %#v", claudeOption.Channels[0])
	}
	if len(claudeOption.Channels[0].Models) != 1 || claudeOption.Channels[0].Models[0].ID != "claude-sonnet-4-5" {
		t.Fatalf("unexpected claude models: %#v", claudeOption.Channels[0].Models)
	}

	if compatOption == nil {
		t.Fatal("expected openai-compatibility provider option")
	}
	if len(compatOption.Channels) != 1 {
		t.Fatalf("expected 1 openai-compatible channel, got %#v", compatOption.Channels)
	}
	if compatOption.Channels[0].ID != "compat-auth-1" || compatOption.Channels[0].Label != "Github OpenAI Compatible" {
		t.Fatalf("unexpected compat channel option: %#v", compatOption.Channels[0])
	}
	if len(compatOption.Channels[0].Models) != 1 || compatOption.Channels[0].Models[0].ID != "gpt-5.4" {
		t.Fatalf("unexpected compat models: %#v", compatOption.Channels[0].Models)
	}
}

func TestBuildAPIKeyAccessOptions_CodexCompatFallsBackToConfiguredModels(t *testing.T) {
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

	options := buildAPIKeyAccessOptions(cfg, manager)

	var compatOption *apiKeyAccessProviderOption
	for i := range options {
		if options[i].Provider == "codex-compat" {
			compatOption = &options[i]
			break
		}
	}

	if compatOption == nil {
		t.Fatalf("expected codex-compat provider option, got %#v", options)
	}
	if len(compatOption.Channels) != 1 {
		t.Fatalf("expected 1 codex-compat channel, got %#v", compatOption.Channels)
	}

	gotModels := make(map[string]struct{}, len(compatOption.Channels[0].Models))
	for _, model := range compatOption.Channels[0].Models {
		gotModels[model.ID] = struct{}{}
	}

	for _, want := range []string{"gpt-5.4", "gpt-5.4-mini", "compat/gpt-5.4", "compat/gpt-5.4-mini"} {
		if _, exists := gotModels[want]; !exists {
			t.Fatalf("expected model %q in %#v", want, compatOption.Channels[0].Models)
		}
	}
}
