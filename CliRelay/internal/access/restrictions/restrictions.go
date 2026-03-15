package restrictions

import (
	"encoding/json"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

const (
	// MetadataAllowedModelsKey stores the global allowed model list for an API key.
	MetadataAllowedModelsKey = "allowed-models"
	// MetadataProviderAccessKey stores provider-scoped access rules for an API key.
	MetadataProviderAccessKey = "allowed-provider-access"
)

const openAICompatibilityProviderFamily = "openai-compatibility"

// ProviderRule describes an allowed provider and its optional channel/model subsets.
type ProviderRule struct {
	AllChannels bool
	Channels    map[string]struct{}
	AllModels   bool
	Models      map[string]struct{}
}

// Restrictions contains the effective provider/model restrictions for an API key.
type Restrictions struct {
	AllowedModels map[string]struct{}
	ProviderRules map[string]ProviderRule
}

// NormalizeAPIKeyEntry trims and deduplicates API key entry fields before persistence/use.
func NormalizeAPIKeyEntry(entry config.APIKeyEntry) config.APIKeyEntry {
	entry.Key = strings.TrimSpace(entry.Key)
	entry.Name = strings.TrimSpace(entry.Name)
	entry.CreatedAt = strings.TrimSpace(entry.CreatedAt)
	entry.AllowedModels = NormalizeModelList(entry.AllowedModels)
	entry.ProviderAccess = NormalizeProviderAccess(entry.ProviderAccess)
	if len(entry.AllowedModels) == 0 {
		entry.AllowedModels = nil
	}
	if len(entry.ProviderAccess) == 0 {
		entry.ProviderAccess = nil
	}
	return entry
}

// NormalizeModelList trims, deduplicates and preserves model order.
func NormalizeModelList(models []string) []string {
	return normalizeStringList(models)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeProviderAccess trims, merges and deduplicates provider-scoped access rules.
func NormalizeProviderAccess(entries []config.APIKeyProviderAccess) []config.APIKeyProviderAccess {
	if len(entries) == 0 {
		return nil
	}

	type providerState struct {
		allChannels  bool
		channels     []string
		seenChannels map[string]struct{}
		allModels    bool
		models       []string
		seenModels   map[string]struct{}
	}

	order := make([]string, 0, len(entries))
	stateByProvider := make(map[string]*providerState, len(entries))

	for _, entry := range entries {
		provider := NormalizeProviderName(entry.Provider)
		if provider == "" {
			continue
		}

		state := stateByProvider[provider]
		if state == nil {
			state = &providerState{
				seenChannels: make(map[string]struct{}),
				seenModels:   make(map[string]struct{}),
			}
			stateByProvider[provider] = state
			order = append(order, provider)
		}

		channels := normalizeStringList(entry.Channels)
		if len(channels) == 0 {
			state.allChannels = true
			state.channels = nil
			state.seenChannels = nil
		} else if !state.allChannels {
			for _, channel := range channels {
				if _, exists := state.seenChannels[channel]; exists {
					continue
				}
				state.seenChannels[channel] = struct{}{}
				state.channels = append(state.channels, channel)
			}
		}

		models := NormalizeModelList(entry.Models)
		if len(models) == 0 {
			state.allModels = true
			state.models = nil
			state.seenModels = nil
		} else if !state.allModels {
			for _, model := range models {
				if _, exists := state.seenModels[model]; exists {
					continue
				}
				state.seenModels[model] = struct{}{}
				state.models = append(state.models, model)
			}
		}
	}

	out := make([]config.APIKeyProviderAccess, 0, len(order))
	for _, provider := range order {
		state := stateByProvider[provider]
		if state == nil {
			continue
		}
		item := config.APIKeyProviderAccess{Provider: provider}
		if !state.allChannels && len(state.channels) > 0 {
			item.Channels = append([]string(nil), state.channels...)
		}
		if !state.allModels && len(state.models) > 0 {
			item.Models = append([]string(nil), state.models...)
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildRestrictionMetadata serializes API key restrictions into auth metadata.
func BuildRestrictionMetadata(allowedModels []string, providerAccess []config.APIKeyProviderAccess) map[string]string {
	normalizedModels := NormalizeModelList(allowedModels)
	normalizedProviderAccess := NormalizeProviderAccess(providerAccess)
	if len(normalizedModels) == 0 && len(normalizedProviderAccess) == 0 {
		return nil
	}

	metadata := make(map[string]string, 2)
	if len(normalizedModels) > 0 {
		metadata[MetadataAllowedModelsKey] = strings.Join(normalizedModels, ",")
	}
	if len(normalizedProviderAccess) > 0 {
		if payload, err := json.Marshal(normalizedProviderAccess); err == nil && len(payload) > 0 {
			metadata[MetadataProviderAccessKey] = string(payload)
		}
	}
	return metadata
}

// ParseRestrictions builds a Restrictions object from auth metadata.
func ParseRestrictions(metadata map[string]string) Restrictions {
	var restrictions Restrictions
	if len(metadata) == 0 {
		return restrictions
	}

	if raw := strings.TrimSpace(metadata[MetadataAllowedModelsKey]); raw != "" {
		restrictions.AllowedModels = buildModelLookup(strings.Split(raw, ","))
	}

	if raw := strings.TrimSpace(metadata[MetadataProviderAccessKey]); raw != "" {
		var providerAccess []config.APIKeyProviderAccess
		if err := json.Unmarshal([]byte(raw), &providerAccess); err == nil {
			normalized := NormalizeProviderAccess(providerAccess)
			if len(normalized) > 0 {
				restrictions.ProviderRules = make(map[string]ProviderRule, len(normalized))
				for _, item := range normalized {
					channelLookup := buildExactLookup(item.Channels)
					modelLookup := buildModelLookup(item.Models)
					restrictions.ProviderRules[NormalizeProviderName(item.Provider)] = ProviderRule{
						AllChannels: len(item.Channels) == 0,
						Channels:    channelLookup,
						AllModels:   len(item.Models) == 0,
						Models:      modelLookup,
					}
				}
			}
		}
	}

	return restrictions
}

// HasProviderRules reports whether provider-level restrictions are configured.
func (r Restrictions) HasProviderRules() bool {
	return len(r.ProviderRules) > 0
}

// IsZero reports whether no restrictions are configured.
func (r Restrictions) IsZero() bool {
	return len(r.AllowedModels) == 0 && len(r.ProviderRules) == 0
}

// AllowsProviderModel reports whether a provider/model pair is allowed.
func (r Restrictions) AllowsProviderModel(provider, model string) bool {
	if len(r.ProviderRules) > 0 {
		rule, exists := r.ProviderRules[NormalizeProviderName(provider)]
		if !exists {
			return false
		}
		if !rule.AllModels && len(rule.Models) > 0 && !lookupModel(rule.Models, model) {
			return false
		}
	}
	if len(r.AllowedModels) > 0 && !lookupModel(r.AllowedModels, model) {
		return false
	}
	return true
}

// AllowsAuth reports whether a concrete auth channel can serve the supplied model.
func (r Restrictions) AllowsAuth(auth *coreauth.Auth, model string) bool {
	if auth == nil {
		return false
	}
	if len(r.ProviderRules) > 0 {
		rule, exists := r.ProviderRules[ProviderFamilyForAuth(auth)]
		if !exists {
			return false
		}
		if !rule.AllChannels {
			channelID := ChannelIDForAuth(auth)
			if channelID == "" {
				return false
			}
			if _, ok := rule.Channels[channelID]; !ok {
				return false
			}
		}
		if !rule.AllModels && len(rule.Models) > 0 && !lookupModel(rule.Models, model) {
			return false
		}
	}
	if len(r.AllowedModels) > 0 && !lookupModel(r.AllowedModels, model) {
		return false
	}
	return true
}

// FilterProviders keeps the providers allowed for the supplied model.
func (r Restrictions) FilterProviders(providers []string, model string) []string {
	if len(providers) == 0 {
		return nil
	}
	if r.IsZero() {
		out := make([]string, 0, len(providers))
		for _, provider := range providers {
			normalized := NormalizeProviderName(provider)
			if normalized == "" {
				continue
			}
			out = append(out, normalized)
		}
		return out
	}

	out := make([]string, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		normalized := NormalizeProviderName(provider)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		if !r.AllowsProviderModel(normalized, model) {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func buildExactLookup(values []string) map[string]struct{} {
	normalized := normalizeStringList(values)
	if len(normalized) == 0 {
		return nil
	}

	lookup := make(map[string]struct{}, len(normalized))
	for _, value := range normalized {
		lookup[value] = struct{}{}
	}
	return lookup
}

func buildModelLookup(models []string) map[string]struct{} {
	normalized := NormalizeModelList(models)
	if len(normalized) == 0 {
		return nil
	}

	lookup := make(map[string]struct{}, len(normalized)*2)
	for _, model := range normalized {
		for _, key := range modelLookupKeys(model) {
			lookup[key] = struct{}{}
		}
	}
	return lookup
}

func lookupModel(lookup map[string]struct{}, model string) bool {
	if len(lookup) == 0 {
		return false
	}
	for _, key := range modelLookupKeys(model) {
		if _, exists := lookup[key]; exists {
			return true
		}
	}
	return false
}

func modelLookupKeys(model string) []string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return nil
	}

	keys := []string{strings.ToLower(trimmed)}
	baseModel := strings.TrimSpace(thinking.ParseSuffix(trimmed).ModelName)
	if baseModel != "" && !strings.EqualFold(baseModel, trimmed) {
		keys = append(keys, strings.ToLower(baseModel))
	}
	return keys
}

// NormalizeProviderName canonicalizes provider family names used by API key restrictions.
func NormalizeProviderName(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	switch normalized {
	case "openai":
		return openAICompatibilityProviderFamily
	default:
		return normalized
	}
}

// ProviderFamilyForAuth resolves the restriction provider family for a runtime auth entry.
func ProviderFamilyForAuth(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if auth.Attributes != nil && strings.TrimSpace(auth.Attributes["compat_name"]) != "" {
		return openAICompatibilityProviderFamily
	}
	return NormalizeProviderName(auth.Provider)
}

// ChannelIDForAuth returns the stable channel identifier used in restrictions.
func ChannelIDForAuth(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	return strings.TrimSpace(auth.ID)
}

// ChannelLabelForAuth returns a user-facing label for a runtime auth entry.
func ChannelLabelForAuth(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}
	if label := strings.TrimSpace(auth.Label); label != "" {
		return label
	}
	if auth.Attributes != nil {
		if label := strings.TrimSpace(auth.Attributes["compat_name"]); label != "" {
			return label
		}
	}
	if provider := NormalizeProviderName(auth.Provider); provider != "" {
		return provider
	}
	return strings.TrimSpace(auth.ID)
}
