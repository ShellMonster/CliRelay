package auth

import (
	"context"
	"net/http"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
)

type userAgentRoutingTestExecutor struct {
	provider string
}

func (e userAgentRoutingTestExecutor) Identifier() string {
	return e.provider
}

func (e userAgentRoutingTestExecutor) Execute(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{Payload: []byte(auth.ID)}, nil
}

func (e userAgentRoutingTestExecutor) ExecuteStream(_ context.Context, _ *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e userAgentRoutingTestExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

func (e userAgentRoutingTestExecutor) CountTokens(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{Payload: []byte(auth.ID)}, nil
}

func (e userAgentRoutingTestExecutor) HttpRequest(_ context.Context, _ *Auth, _ *http.Request) (*http.Response, error) {
	return nil, nil
}

func TestManagerExecute_UserAgentForceProviders(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "opencode-force-compat",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			ForceProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", "", []string{"codex", "codex-compat"})
	if authID != "auth-compat" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-compat")
	}
}

func TestManagerExecute_UserAgentPreferProviders(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:            "opencode-prefer-compat",
			Enabled:         boolPtr(true),
			MatchMode:       "contains",
			Pattern:         "opencode",
			PreferProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", "", []string{"codex", "codex-compat"})
	if authID != "auth-compat" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-compat")
	}
}

func TestManagerExecute_UserAgentRoutingNoMatchKeepsDefaultSelection(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:            "opencode-prefer-compat",
			Enabled:         boolPtr(true),
			MatchMode:       "contains",
			Pattern:         "opencode",
			PreferProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "codex-cli/0.1.0", "", []string{"codex", "codex-compat"})
	if authID != "auth-codex" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-codex")
	}
}

func TestManagerExecute_UserAgentForceProvidersFallsBackWhenNoIntersection(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "missing-provider",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			ForceProviders: []string{"gemini"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", "", []string{"codex", "codex-compat"})
	if authID != "auth-codex" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-codex")
	}
}

func TestManagerExecute_UserAgentAndModelRoutingMatch(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "opencode-gpt5-force-compat",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			Models:         []string{"gpt-5"},
			ForceProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", "gpt-5(high)", []string{"codex", "codex-compat"})
	if authID != "auth-compat" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-compat")
	}
}

func TestManagerExecute_UserAgentModelMismatchKeepsDefaultSelection(t *testing.T) {
	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "opencode-gpt5-force-compat",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			Models:         []string{"gpt-5"},
			ForceProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", "gpt-4.1", []string{"codex", "codex-compat"})
	if authID != "auth-codex" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-codex")
	}
}

func TestUserAgentRoutingRuleMatchesModel_StripsNamespacePrefix(t *testing.T) {
	rule := internalconfig.UserAgentRoutingRule{
		Models: []string{"gpt-5.4"},
	}

	if !userAgentRoutingRuleMatchesModel(rule, "codex-compat/gpt-5.4(high)") {
		t.Fatalf("expected namespaced model to match base model rule")
	}
	if userAgentRoutingRuleMatchesModel(rule, "codex-compat/gpt-4.1") {
		t.Fatalf("expected different namespaced model to not match")
	}
}

func newUserAgentRoutingTestManager(t *testing.T, rules []internalconfig.UserAgentRoutingRule) *Manager {
	t.Helper()

	manager := NewManager(nil, &FillFirstSelector{}, nil)
	manager.RegisterExecutor(userAgentRoutingTestExecutor{provider: "codex"})
	manager.RegisterExecutor(userAgentRoutingTestExecutor{provider: "codex-compat"})
	manager.SetConfig(&internalconfig.Config{
		Routing: internalconfig.RoutingConfig{
			UserAgentRules: rules,
		},
	})

	for _, auth := range []*Auth{
		{ID: "auth-codex", Provider: "codex"},
		{ID: "auth-compat", Provider: "codex-compat"},
	} {
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth %s: %v", auth.ID, err)
		}
	}

	modelRegistry := registry.GetGlobalRegistry()
	models := []*registry.ModelInfo{
		{ID: "gpt-5"},
		{ID: "gpt-4.1"},
	}
	modelRegistry.RegisterClient("auth-codex", "codex", models)
	modelRegistry.RegisterClient("auth-compat", "codex-compat", models)
	if !modelRegistry.ClientSupportsModel("auth-codex", "gpt-5") || !modelRegistry.ClientSupportsModel("auth-compat", "gpt-5") {
		t.Fatalf("test fixture failed to register expected models")
	}
	t.Cleanup(func() {
		modelRegistry.UnregisterClient("auth-codex")
		modelRegistry.UnregisterClient("auth-compat")
	})

	return manager
}

func executeUserAgentRoutedRequest(t *testing.T, manager *Manager, userAgent, model string, providers []string) string {
	t.Helper()

	selectedAuthID := ""
	opts := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.UserAgentMetadataKey:            userAgent,
			cliproxyexecutor.SelectedAuthCallbackMetadataKey: func(authID string) { selectedAuthID = authID },
		},
	}
	_, err := manager.Execute(context.Background(), providers, cliproxyexecutor.Request{Model: model}, opts)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}

	if selectedAuthID != "" {
		return selectedAuthID
	}
	authID, _ := opts.Metadata[cliproxyexecutor.SelectedAuthMetadataKey].(string)
	return authID
}

func boolPtr(v bool) *bool {
	return &v
}
