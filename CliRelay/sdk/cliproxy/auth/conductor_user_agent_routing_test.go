package auth

import (
	"context"
	"net/http"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v6/internal/config"
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
	t.Parallel()

	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "opencode-force-compat",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			ForceProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", []string{"codex", "codex-compat"})
	if authID != "auth-compat" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-compat")
	}
}

func TestManagerExecute_UserAgentPreferProviders(t *testing.T) {
	t.Parallel()

	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:            "opencode-prefer-compat",
			Enabled:         boolPtr(true),
			MatchMode:       "contains",
			Pattern:         "opencode",
			PreferProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", []string{"codex", "codex-compat"})
	if authID != "auth-compat" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-compat")
	}
}

func TestManagerExecute_UserAgentRoutingNoMatchKeepsDefaultSelection(t *testing.T) {
	t.Parallel()

	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:            "opencode-prefer-compat",
			Enabled:         boolPtr(true),
			MatchMode:       "contains",
			Pattern:         "opencode",
			PreferProviders: []string{"codex-compat"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "codex-cli/0.1.0", []string{"codex", "codex-compat"})
	if authID != "auth-codex" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-codex")
	}
}

func TestManagerExecute_UserAgentForceProvidersFallsBackWhenNoIntersection(t *testing.T) {
	t.Parallel()

	manager := newUserAgentRoutingTestManager(t, []internalconfig.UserAgentRoutingRule{
		{
			Name:           "missing-provider",
			Enabled:        boolPtr(true),
			MatchMode:      "contains",
			Pattern:        "opencode",
			ForceProviders: []string{"gemini"},
		},
	})

	authID := executeUserAgentRoutedRequest(t, manager, "opencode/1.0.0", []string{"codex", "codex-compat"})
	if authID != "auth-codex" {
		t.Fatalf("selected auth = %q, want %q", authID, "auth-codex")
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

	return manager
}

func executeUserAgentRoutedRequest(t *testing.T, manager *Manager, userAgent string, providers []string) string {
	t.Helper()

	opts := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.UserAgentMetadataKey: userAgent,
		},
	}
	_, err := manager.Execute(context.Background(), providers, cliproxyexecutor.Request{}, opts)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}

	authID, _ := opts.Metadata[cliproxyexecutor.SelectedAuthMetadataKey].(string)
	return authID
}

func boolPtr(v bool) *bool {
	return &v
}
