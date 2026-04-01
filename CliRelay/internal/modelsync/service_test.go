package modelsync

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		targets: []syncTarget{{
			kind: "codex",
			match: entryMatcher{
				apiKey:   "k",
				baseURL:  "https://api.example.com",
				proxyURL: "socks5://127.0.0.1:1080",
				headers:  map[string]string{"X-Test": "1"},
			},
			codex: []config.CodexModel{{Name: "gpt-5.4"}},
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

func TestBuildOpenAICompatTarget_FallsBackToLaterAPIKeyEntry(t *testing.T) {
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if got := r.Header.Get("Authorization"); got != "Bearer second-key" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"forbidden"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"o3","object":"model","created":1770000000,"owned_by":"test"}]}`))
	}))
	defer server.Close()

	target, err := buildOpenAICompatTarget(context.Background(), config.OpenAICompatibility{
		Name:           "test-openai-compat",
		BaseURL:        server.URL + "/v1",
		AutoSyncModels: true,
		APIKeyEntries: []config.OpenAICompatibilityAPIKey{
			{APIKey: "first-key"},
			{APIKey: "second-key"},
		},
	})
	if err != nil {
		t.Fatalf("expected fallback to second API key entry, got error: %v", err)
	}
	if target == nil {
		t.Fatal("expected sync target when later API key entry succeeds")
	}
	if len(authHeaders) < 2 || authHeaders[0] != "Bearer first-key" || authHeaders[1] != "Bearer second-key" {
		t.Fatalf("expected both API key entries to be attempted in order, got %#v", authHeaders)
	}
	found := false
	for _, model := range target.openai {
		if model.Name == "o3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected synced models to include o3, got %#v", target.openai)
	}
}

func TestBuildOpenAICompatTarget_SkipsWhenBaseURLMissing(t *testing.T) {
	target, err := buildOpenAICompatTarget(context.Background(), config.OpenAICompatibility{
		Name:           "kimi",
		BaseURL:        "",
		AutoSyncModels: true,
		APIKeyEntries: []config.OpenAICompatibilityAPIKey{
			{APIKey: "fake-key"},
		},
	})
	if err != nil {
		t.Fatalf("expected missing base-url to be skipped without error, got %v", err)
	}
	if target != nil {
		t.Fatalf("expected no sync target when base-url is missing, got %#v", target)
	}
}
