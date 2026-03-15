package management

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestUsageChannelResolverResolveDisplayNameDoesNotLeakSource(t *testing.T) {
	resolver := usageChannelResolver{
		displayByAuthIndex: map[string]string{},
		displayBySource:    map[string]string{},
		ambiguousAuthIndex: map[string]struct{}{},
		ambiguousSource:    map[string]struct{}{},
	}

	if got := resolver.ResolveDisplayName("", "", "user@example.com"); got != "" {
		t.Fatalf("expected empty display name when only source exists, got %q", got)
	}

	if got := resolver.ResolveDisplayName("", "legacy-channel", "user@example.com"); got != "legacy-channel" {
		t.Fatalf("expected channel name fallback, got %q", got)
	}
}

func TestUsageChannelResolverResolveDisplayNameFallsBackToLoggedNameWhenSourceIsAmbiguous(t *testing.T) {
	resolver := usageChannelResolver{
		displayByAuthIndex: map[string]string{},
		displayBySource:    map[string]string{},
		ambiguousAuthIndex: map[string]struct{}{},
		ambiguousSource:    map[string]struct{}{},
	}

	resolver.assignSourceDisplay("shared-key", "Channel A")
	resolver.assignSourceDisplay("shared-key", "Channel B")

	if got := resolver.ResolveDisplayName("", "logged-channel", "shared-key"); got != "logged-channel" {
		t.Fatalf("expected logged channel name fallback for ambiguous source, got %q", got)
	}
}

func TestUsageChannelResolverResolveDisplayNameFallsBackToLoggedNameWhenAuthIndexIsAmbiguous(t *testing.T) {
	resolver := usageChannelResolver{
		displayByAuthIndex: map[string]string{},
		displayBySource:    map[string]string{},
		ambiguousAuthIndex: map[string]struct{}{},
		ambiguousSource:    map[string]struct{}{},
	}

	resolver.assignAuthDisplay("idx-1", "Channel A")
	resolver.assignAuthDisplay("idx-1", "Channel B")

	if got := resolver.ResolveDisplayName("idx-1", "logged-channel", ""); got != "logged-channel" {
		t.Fatalf("expected logged channel name fallback for ambiguous auth index, got %q", got)
	}
}

func TestUsageChannelResolverResolveChannelFilterSupportsTokensAndLabels(t *testing.T) {
	resolver := usageChannelResolver{
		sourcesByAuthIndex: map[string][]string{
			"idx-1": []string{"src-1"},
		},
		channelNamesByAuthIndex: map[string][]string{
			"idx-1": []string{"current-channel", "old-channel"},
		},
		filterByLabel: map[string]usage.ChannelFilter{
			"Current Channel": {
				AuthIndexes:  []string{"idx-1"},
				Sources:      []string{"src-1"},
				ChannelNames: []string{"current-channel", "old-channel"},
			},
			"Deleted Channel [legacy]": {
				ChannelNames: []string{"deleted-channel"},
			},
		},
	}

	filter := resolver.ResolveChannelFilter([]string{
		makeUsageAuthChannelToken("idx-1"),
		"Current Channel",
		makeUsageLegacyNameToken("legacy-only"),
		"Deleted Channel [legacy]",
	})

	if len(filter.AuthIndexes) != 1 || filter.AuthIndexes[0] != "idx-1" {
		t.Fatalf("expected auth index filter to contain idx-1 once, got %+v", filter.AuthIndexes)
	}
	if len(filter.Sources) != 1 || filter.Sources[0] != "src-1" {
		t.Fatalf("expected source filter to contain src-1 once, got %+v", filter.Sources)
	}
	if len(filter.ChannelNames) != 4 {
		t.Fatalf("expected 4 channel names, got %+v", filter.ChannelNames)
	}
	if filter.ChannelNames[0] != "current-channel" || filter.ChannelNames[1] != "old-channel" || filter.ChannelNames[2] != "legacy-only" || filter.ChannelNames[3] != "deleted-channel" {
		t.Fatalf("unexpected channel names order/content: %+v", filter.ChannelNames)
	}
}

func TestUsageChannelResolverKeepsLegacyOptionWhenLabelCollides(t *testing.T) {
	resolver := (&Handler{}).newUsageChannelResolver([]usage.ChannelRef{
		{AuthIndex: "idx-1", ChannelName: "same-name"},
		{ChannelName: "same-name"},
	})

	if len(resolver.channelOptions) != 2 {
		t.Fatalf("expected 2 channel options, got %+v", resolver.channelOptions)
	}
	if resolver.channelOptions[0].Label != "same-name" {
		t.Fatalf("expected auth option label to keep base label, got %+v", resolver.channelOptions)
	}
	if resolver.channelOptions[1].Label != "same-name [legacy]" {
		t.Fatalf("expected legacy option label to be suffixed, got %+v", resolver.channelOptions)
	}
	if len(resolver.displayChannelNames) != 2 {
		t.Fatalf("expected compatibility labels to remain complete, got %+v", resolver.displayChannelNames)
	}
}

func TestResolveUsageChannelStableSourceIgnoresOAuthEmail(t *testing.T) {
	auth := &coreauth.Auth{
		Provider: "claude",
		Metadata: map[string]any{"email": "user@example.com"},
	}
	if got := resolveUsageChannelStableSource(auth); got != "" {
		t.Fatalf("expected oauth email source to be ignored, got %q", got)
	}

	auth.Attributes = map[string]string{"api_key": "sk-test"}
	if got := resolveUsageChannelStableSource(auth); got != "sk-test" {
		t.Fatalf("expected api key source to be retained, got %q", got)
	}
}

func TestUsageChannelResolverIncludesConfiguredChannelsWithoutRefs(t *testing.T) {
	handler := &Handler{
		cfg: &config.Config{
			CodexCompatKey: []config.CodexKey{
				{
					APIKey:  "sk-compat",
					Name:    "Github_Compat",
					BaseURL: "https://api.githubcopilot.com",
				},
			},
		},
	}

	resolver := handler.newUsageChannelResolver(nil)
	if len(resolver.channelOptions) != 1 {
		t.Fatalf("expected 1 configured channel option, got %+v", resolver.channelOptions)
	}

	option := resolver.channelOptions[0]
	if option.Label != "Github_Compat" {
		t.Fatalf("expected configured channel label, got %+v", resolver.channelOptions)
	}
	if source, ok := parseUsageSourceChannelToken(option.Value); !ok || source != "sk-compat" {
		t.Fatalf("expected source token for configured channel, got %q", option.Value)
	}

	filter := resolver.ResolveChannelFilter([]string{option.Value})
	if len(filter.Sources) != 1 || filter.Sources[0] != "sk-compat" {
		t.Fatalf("expected configured channel source filter, got %+v", filter.Sources)
	}
	if len(filter.ChannelNames) != 1 || filter.ChannelNames[0] != "Github_Compat" {
		t.Fatalf("expected configured channel name fallback, got %+v", filter.ChannelNames)
	}
	if len(resolver.displayChannelNames) != 1 || resolver.displayChannelNames[0] != "Github_Compat" {
		t.Fatalf("expected configured channel name in compatibility list, got %+v", resolver.displayChannelNames)
	}
}

func TestUsageChannelSelectionNeedsRefsSupportsSourceToken(t *testing.T) {
	if usageChannelSelectionNeedsRefs([]string{makeUsageSourceChannelToken("sk-compat")}) {
		t.Fatal("expected source token selection to skip ref query")
	}
}
