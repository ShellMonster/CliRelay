package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

func TestFetchCodexModels_Success(t *testing.T) {
	codexModelsCache = sync.Map{}

	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.4","object":"model","created":1770000000,"owned_by":"openai"},{"id":"gpt-5.3-codex","object":"model","created":1770000001,"owned_by":"openai"}]}`))
	}))
	defer server.Close()

	auth := &cliproxyauth.Auth{
		ID:       "codex-auth-test-success",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "test-key",
			"base_url": server.URL + "/v1",
		},
	}
	models := FetchCodexModels(context.Background(), auth, &config.Config{})
	if len(models) < 2 {
		t.Fatalf("expected at least 2 models, got %d", len(models))
	}
	if got := strings.TrimSpace(authHeader); got != "Bearer test-key" {
		t.Fatalf("expected Authorization header %q, got %q", "Bearer test-key", got)
	}

	var foundDynamic bool
	var foundStaticMerged bool
	for _, model := range models {
		if model == nil {
			continue
		}
		if model.ID == "gpt-5.4" {
			foundDynamic = true
		}
		if model.ID == "gpt-5.3-codex" && model.MaxCompletionTokens > 0 {
			foundStaticMerged = true
		}
	}
	if !foundDynamic {
		t.Fatal("expected dynamic model gpt-5.4 in fetched models")
	}
	if !foundStaticMerged {
		t.Fatal("expected gpt-5.3-codex to include static metadata fields")
	}
}

func TestFetchCodexModels_FallbackToModelsDev(t *testing.T) {
	codexModelsCache = sync.Map{}

	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"Missing scopes: api.model.read"}}`))
	}))
	defer openAIServer.Close()

	modelsDevServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api.json" {
			t.Fatalf("unexpected models.dev path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"openai": {
				"models": {
					"openai/gpt-5.4": {"name":"GPT-5.4"},
					"openai/gpt-5.3-codex": {"name":"GPT-5.3 Codex"}
				}
			},
			"anthropic": {
				"models": {
					"anthropic/claude-sonnet-4": {"name":"Claude Sonnet 4"}
				}
			}
		}`))
	}))
	defer modelsDevServer.Close()

	previousModelsDevEndpoint := codexModelsDevAPIEndpoint
	codexModelsDevAPIEndpoint = modelsDevServer.URL + "/api.json"
	t.Cleanup(func() { codexModelsDevAPIEndpoint = previousModelsDevEndpoint })

	auth := &cliproxyauth.Auth{
		ID:       "codex-auth-test-models-dev-fallback",
		Provider: "codex",
		Attributes: map[string]string{
			"api_key":  "test-key",
			"base_url": openAIServer.URL + "/v1",
		},
	}
	models := FetchCodexModels(context.Background(), auth, &config.Config{})
	if len(models) == 0 {
		t.Fatal("expected models from models.dev fallback")
	}

	var hasGPT54, hasGPT53Codex, hasAnthropic bool
	for _, model := range models {
		if model == nil {
			continue
		}
		switch model.ID {
		case "gpt-5.4":
			hasGPT54 = true
		case "gpt-5.3-codex":
			hasGPT53Codex = model.MaxCompletionTokens > 0
		case "anthropic/claude-sonnet-4":
			hasAnthropic = true
		}
	}

	if !hasGPT54 {
		t.Fatal("expected gpt-5.4 from models.dev fallback")
	}
	if !hasGPT53Codex {
		t.Fatal("expected static metadata merge for gpt-5.3-codex from models.dev fallback")
	}
	if hasAnthropic {
		t.Fatal("unexpected non-openai model in models.dev fallback result")
	}
}

func TestParseCodexModelsDevResponse_OpenAIOnly(t *testing.T) {
	models := parseCodexModelsDevResponse([]byte(`{
		"openai": {
			"models": {
				"openai/gpt-5.4": {"name":"GPT-5.4"},
				"gpt-5.1": {"name":"GPT-5.1"}
			}
		},
		"other-provider": {
			"models": {
				"openai/o3": {"name":"O3"},
				"other/model": {"name":"Other"}
			}
		}
	}`))

	if len(models) != 3 {
		t.Fatalf("expected 3 openai models, got %d", len(models))
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if model != nil {
			ids = append(ids, model.ID)
		}
	}
	sort.Strings(ids)
	got := strings.Join(ids, ",")
	if got != "gpt-5.1,gpt-5.4,o3" {
		t.Fatalf("unexpected model IDs: %s", got)
	}
}
