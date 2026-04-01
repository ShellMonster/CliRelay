package modelsync

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func TestCollectMissingCodexModels_AppendsOnlyNewModels(t *testing.T) {
	out := collectMissingCodexModels([]config.CodexModel{
		{Name: "gpt-5", Alias: "g5"},
	}, []*registry.ModelInfo{
		{ID: "gpt-5"},
		{ID: "gpt-5.4"},
		{Name: "o3"},
	})

	if len(out) != 2 {
		t.Fatalf("expected 2 new models, got %d", len(out))
	}
	if out[0].Name != "gpt-5.4" || out[0].Alias != "" {
		t.Fatalf("unexpected first appended model: %#v", out[0])
	}
	if out[1].Name != "o3" {
		t.Fatalf("unexpected second appended model: %#v", out[1])
	}
}

func TestApplySyncPlan_AppendsModelsWithoutOverwritingExistingAlias(t *testing.T) {
	cfg := &config.Config{
		CodexKey: []config.CodexKey{{
			APIKey:   "k",
			BaseURL:  "https://api.example.com",
			ProxyURL: "socks5://127.0.0.1:1080",
			Headers:  map[string]string{"X-Test": "1"},
			Models: []config.CodexModel{
				{Name: "gpt-5", Alias: "g5"},
			},
		}},
	}

	changed := applySyncPlan(cfg, &providerSyncPlan{
		targets: []codexSyncTarget{{
			kind:        "codex",
			apiKey:      "k",
			baseURL:     "https://api.example.com",
			proxyURL:    "socks5://127.0.0.1:1080",
			headers:     map[string]string{"X-Test": "1"},
			modelsToAdd: []config.CodexModel{{Name: "gpt-5.4"}},
		}},
	})

	if !changed {
		t.Fatal("expected applySyncPlan to report changes")
	}
	if len(cfg.CodexKey[0].Models) != 2 {
		t.Fatalf("expected 2 models after apply, got %d", len(cfg.CodexKey[0].Models))
	}
	if cfg.CodexKey[0].Models[0].Alias != "g5" {
		t.Fatalf("expected existing alias to remain unchanged, got %q", cfg.CodexKey[0].Models[0].Alias)
	}
}

func TestAppendMissingCodexModels_DedupesAgainstLiveEntries(t *testing.T) {
	existing := []config.CodexModel{
		{Name: "gpt-5"},
		{Name: "gpt-5.4"},
	}

	out := appendMissingCodexModels(existing, []config.CodexModel{
		{Name: "gpt-5.4"},
		{Name: "o3"},
	})

	if len(out) != 3 {
		t.Fatalf("expected 3 models after dedupe append, got %d", len(out))
	}
	if out[2].Name != "o3" {
		t.Fatalf("expected only new model to be appended, got %#v", out[2])
	}
}
