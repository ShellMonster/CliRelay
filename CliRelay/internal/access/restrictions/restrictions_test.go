package restrictions

import (
	"reflect"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestNormalizeProviderAccess_MergesEntries(t *testing.T) {
	input := []config.APIKeyProviderAccess{
		{Provider: " codex ", Channels: []string{"auth-a", "auth-a"}, Models: []string{"gpt-5.2", "gpt-5.2", "gpt-5.2(high)"}},
		{Provider: "codex", Channels: []string{"auth-b"}, Models: []string{"gpt-5.3"}},
		{Provider: "gemini", Channels: []string{"gem-1"}},
		{Provider: "gemini", Models: []string{"gemini-2.5-pro"}},
		{Provider: ""},
	}

	got := NormalizeProviderAccess(input)
	want := []config.APIKeyProviderAccess{
		{Provider: "codex", Channels: []string{"auth-a", "auth-b"}, Models: []string{"gpt-5.2", "gpt-5.2(high)", "gpt-5.3"}},
		{Provider: "gemini"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeProviderAccess() = %#v, want %#v", got, want)
	}
}

func TestParseRestrictions_AllowsBaseModelForSuffix(t *testing.T) {
	metadata := BuildRestrictionMetadata([]string{"gpt-5.2"}, []config.APIKeyProviderAccess{
		{Provider: "codex", Channels: []string{"auth-codex"}, Models: []string{"gpt-5.2"}},
	})
	restrictions := ParseRestrictions(metadata)
	allowedAuth := &coreauth.Auth{ID: "auth-codex", Provider: "codex"}
	otherAuth := &coreauth.Auth{ID: "auth-other", Provider: "codex"}

	if restrictions.IsZero() {
		t.Fatal("expected restrictions to be parsed")
	}
	if !restrictions.AllowsProviderModel("codex", "gpt-5.2(high)") {
		t.Fatal("expected suffix variant to be allowed when base model is configured")
	}
	if !restrictions.AllowsAuth(allowedAuth, "gpt-5.2(high)") {
		t.Fatal("expected configured auth channel to be allowed")
	}
	if restrictions.AllowsAuth(otherAuth, "gpt-5.2(high)") {
		t.Fatal("expected unmatched auth channel to be denied")
	}
	if restrictions.AllowsProviderModel("gemini", "gpt-5.2(high)") {
		t.Fatal("expected unmatched provider to be denied")
	}
}

func TestRestrictions_FilterProviders(t *testing.T) {
	restrictions := ParseRestrictions(BuildRestrictionMetadata(nil, []config.APIKeyProviderAccess{
		{Provider: "codex", Models: []string{"gpt-5.2"}},
	}))

	got := restrictions.FilterProviders([]string{"codex", "gemini"}, "gpt-5.2(high)")
	want := []string{"codex"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FilterProviders() = %#v, want %#v", got, want)
	}
}

func TestRestrictions_AllowsOpenAICompatibilityAuthFamily(t *testing.T) {
	restrictions := ParseRestrictions(BuildRestrictionMetadata(nil, []config.APIKeyProviderAccess{
		{Provider: "openai-compatibility", Channels: []string{"compat-auth"}},
	}))
	auth := &coreauth.Auth{
		ID:       "compat-auth",
		Provider: "github-openai-compatible",
		Attributes: map[string]string{
			"compat_name": "Github OpenAI Compatible",
		},
	}

	if !restrictions.AllowsAuth(auth, "gpt-5.4") {
		t.Fatal("expected openai-compatible auth to match provider family restriction")
	}
}
