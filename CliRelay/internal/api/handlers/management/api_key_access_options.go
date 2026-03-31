package management

import (
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	accessrestrictions "github.com/router-for-me/CLIProxyAPI/v6/internal/access/restrictions"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

type apiKeyAccessModelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type apiKeyAccessChannelOption struct {
	ID     string                    `json:"id"`
	Label  string                    `json:"label"`
	Models []apiKeyAccessModelOption `json:"models"`
}

type apiKeyAccessProviderOption struct {
	Provider string                      `json:"provider"`
	Label    string                      `json:"label"`
	Channels []apiKeyAccessChannelOption `json:"channels"`
}

var apiKeyAccessProviderOrder = map[string]int{
	"gemini":               10,
	"gemini-cli":           20,
	"claude":               30,
	"codex":                40,
	"codex-compat":         50,
	"copilot-compat":       60,
	"github-copilot":       60,
	"vertex":               70,
	"openai-compatibility": 80,
	"ampcode":              90,
	"antigravity":          100,
	"qwen":                 110,
	"kimi":                 120,
	"iflow":                130,
	"aistudio":             140,
}

var apiKeyAccessProviderLabels = map[string]string{
	"gemini":               "Gemini",
	"gemini-cli":           "Gemini CLI",
	"claude":               "Claude",
	"codex":                "Codex",
	"codex-compat":         "Codex Compat",
	"copilot-compat":       "GitHub Copilot",
	"github-copilot":       "GitHub Copilot",
	"vertex":               "Vertex",
	"openai-compatibility": "OpenAI Compatible",
	"ampcode":              "Ampcode",
	"antigravity":          "Antigravity",
	"qwen":                 "Qwen",
	"kimi":                 "Kimi",
	"iflow":                "iFlow",
	"aistudio":             "AI Studio",
}

// GetAPIKeyAccessOptions returns provider -> channels -> models options for API key restriction editing.
func (h *Handler) GetAPIKeyAccessOptions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"providers": buildAPIKeyAccessOptions(h.cfg, h.authManager),
	})
}

func buildAPIKeyAccessOptions(cfg *config.Config, manager *coreauth.Manager) []apiKeyAccessProviderOption {
	if manager == nil {
		return nil
	}

	type channelState struct {
		id     string
		label  string
		models map[string]apiKeyAccessModelOption
	}

	type providerState struct {
		provider string
		label    string
		channels map[string]*channelState
	}

	providerStateByID := make(map[string]*providerState)
	providers := make([]string, 0)

	ensureProvider := func(provider string) *providerState {
		normalized := normalizeAccessOptionProvider(provider)
		if normalized == "" {
			return nil
		}
		state := providerStateByID[normalized]
		if state != nil {
			return state
		}
		state = &providerState{
			provider: normalized,
			label:    apiKeyAccessProviderLabel(normalized),
			channels: make(map[string]*channelState),
		}
		providerStateByID[normalized] = state
		providers = append(providers, normalized)
		return state
	}

	reg := registry.GetGlobalRegistry()
	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}
		provider := accessrestrictions.ProviderFamilyForAuth(auth)
		channelID := strings.TrimSpace(accessrestrictions.ChannelIDForAuth(auth))
		if provider == "" || channelID == "" {
			continue
		}

		state := ensureProvider(provider)
		if state == nil {
			continue
		}

		channel := state.channels[channelID]
		if channel == nil {
			channel = &channelState{
				id:     channelID,
				label:  accessrestrictions.ChannelLabelForAuth(auth),
				models: make(map[string]apiKeyAccessModelOption),
			}
			state.channels[channelID] = channel
		}

		for _, model := range resolveAPIKeyAccessModels(cfg, reg, auth) {
			if model == nil {
				continue
			}
			modelID := strings.TrimSpace(model.ID)
			if modelID == "" {
				continue
			}
			channel.models[modelID] = apiKeyAccessModelOption{
				ID:    modelID,
				Label: modelID,
			}
		}
	}

	sort.Slice(providers, func(i, j int) bool {
		left, right := providers[i], providers[j]
		leftOrder, leftExists := apiKeyAccessProviderOrder[left]
		rightOrder, rightExists := apiKeyAccessProviderOrder[right]
		switch {
		case leftExists && rightExists && leftOrder != rightOrder:
			return leftOrder < rightOrder
		case leftExists != rightExists:
			return leftExists
		default:
			return apiKeyAccessProviderLabel(left) < apiKeyAccessProviderLabel(right)
		}
	})

	options := make([]apiKeyAccessProviderOption, 0, len(providers))
	for _, provider := range providers {
		state := providerStateByID[provider]
		if state == nil {
			continue
		}

		channelIDs := make([]string, 0, len(state.channels))
		for channelID := range state.channels {
			channelIDs = append(channelIDs, channelID)
		}
		sort.Slice(channelIDs, func(i, j int) bool {
			leftChannel := state.channels[channelIDs[i]]
			rightChannel := state.channels[channelIDs[j]]
			leftLabel := channelIDs[i]
			rightLabel := channelIDs[j]
			if leftChannel != nil && strings.TrimSpace(leftChannel.label) != "" {
				leftLabel = leftChannel.label
			}
			if rightChannel != nil && strings.TrimSpace(rightChannel.label) != "" {
				rightLabel = rightChannel.label
			}
			leftLabel = strings.ToLower(strings.TrimSpace(leftLabel))
			rightLabel = strings.ToLower(strings.TrimSpace(rightLabel))
			if leftLabel == rightLabel {
				return channelIDs[i] < channelIDs[j]
			}
			return leftLabel < rightLabel
		})

		channels := make([]apiKeyAccessChannelOption, 0, len(channelIDs))
		for _, channelID := range channelIDs {
			channel := state.channels[channelID]
			if channel == nil {
				continue
			}
			modelIDs := make([]string, 0, len(channel.models))
			for modelID := range channel.models {
				modelIDs = append(modelIDs, modelID)
			}
			sort.Slice(modelIDs, func(i, j int) bool {
				return strings.ToLower(modelIDs[i]) < strings.ToLower(modelIDs[j])
			})

			models := make([]apiKeyAccessModelOption, 0, len(modelIDs))
			for _, modelID := range modelIDs {
				models = append(models, channel.models[modelID])
			}

			label := strings.TrimSpace(channel.label)
			if label == "" {
				label = channel.id
			}
			channels = append(channels, apiKeyAccessChannelOption{
				ID:     channel.id,
				Label:  label,
				Models: models,
			})
		}

		options = append(options, apiKeyAccessProviderOption{
			Provider: provider,
			Label:    apiKeyAccessProviderLabel(provider),
			Channels: channels,
		})
	}

	return options
}

func apiKeyAccessProviderLabel(provider string) string {
	provider = normalizeAccessOptionProvider(provider)
	if label, exists := apiKeyAccessProviderLabels[provider]; exists {
		return label
	}
	return provider
}

func normalizeAccessOptionProvider(provider string) string {
	return accessrestrictions.NormalizeProviderName(provider)
}

type apiKeyConfigEntry interface {
	GetAPIKey() string
	GetBaseURL() string
}

type apiKeyConfigModel interface {
	GetName() string
	GetAlias() string
}

func resolveAPIKeyAccessModels(cfg *config.Config, reg *registry.ModelRegistry, auth *coreauth.Auth) []*registry.ModelInfo {
	if auth == nil {
		return nil
	}

	channelID := strings.TrimSpace(accessrestrictions.ChannelIDForAuth(auth))
	if reg != nil && channelID != "" {
		if models := reg.GetModelsForClient(channelID); len(models) > 0 {
			return models
		}
	}

	models := resolveFallbackAPIKeyAccessModels(cfg, auth)
	models = applyAPIKeyAccessExcludedModels(models, auth)
	models = applyAPIKeyAccessModelPrefixes(models, effectiveAPIKeyAccessModelPrefix(auth), cfg != nil && cfg.ForceModelPrefix)
	return models
}

func resolveFallbackAPIKeyAccessModels(cfg *config.Config, auth *coreauth.Auth) []*registry.ModelInfo {
	if auth == nil {
		return nil
	}

	provider := accessrestrictions.ProviderFamilyForAuth(auth)
	switch provider {
	case "gemini":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.GeminiKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetGeminiModels()
	case "vertex":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.VertexCompatAPIKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetGeminiVertexModels()
	case "claude":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.ClaudeKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetClaudeModels()
	case "codex":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.CodexKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetOpenAIModels()
	case "codex-compat":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.CodexCompatKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetOpenAIModels()
	case "copilot-compat", "github-copilot":
		if cfg != nil {
			if entry := resolveAPIKeyAccessConfigEntry(cfg, auth, cfg.CopilotCompatKey); entry != nil && len(entry.Models) > 0 {
				return buildAPIKeyAccessConfigModels(entry.Models)
			}
		}
		return registry.GetOpenAIModels()
	case "openai-compatibility":
		return resolveAPIKeyAccessOpenAICompatibilityModels(cfg, auth)
	case "gemini-cli", "aistudio", "qwen", "iflow", "kimi", "antigravity":
		return registry.GetStaticModelDefinitionsByChannel(provider)
	default:
		return registry.GetStaticModelDefinitionsByChannel(strings.ToLower(strings.TrimSpace(auth.Provider)))
	}
}

func resolveAPIKeyAccessOpenAICompatibilityModels(cfg *config.Config, auth *coreauth.Auth) []*registry.ModelInfo {
	if cfg == nil || auth == nil {
		return nil
	}

	compatName := strings.TrimSpace(auth.Provider)
	providerKey := strings.ToLower(strings.TrimSpace(auth.Provider))
	if auth.Attributes != nil {
		if value := strings.TrimSpace(auth.Attributes["compat_name"]); value != "" {
			compatName = value
		}
		if value := strings.TrimSpace(auth.Attributes["provider_key"]); value != "" {
			providerKey = strings.ToLower(value)
		}
	}

	for i := range cfg.OpenAICompatibility {
		entry := &cfg.OpenAICompatibility[i]
		name := strings.TrimSpace(entry.Name)
		if compatName != "" && strings.EqualFold(name, compatName) {
			return buildAPIKeyAccessConfigModels(entry.Models)
		}
		if providerKey != "" && strings.EqualFold(strings.ToLower(name), providerKey) {
			return buildAPIKeyAccessConfigModels(entry.Models)
		}
	}

	return nil
}

func resolveAPIKeyAccessConfigEntry[T apiKeyConfigEntry](cfg *config.Config, auth *coreauth.Auth, entries []T) *T {
	if cfg == nil || auth == nil || len(entries) == 0 {
		return nil
	}

	attrKey, attrBase := "", ""
	if auth.Attributes != nil {
		attrKey = strings.TrimSpace(auth.Attributes["api_key"])
		attrBase = strings.TrimSpace(auth.Attributes["base_url"])
	}

	for i := range entries {
		entry := &entries[i]
		cfgKey := strings.TrimSpace((*entry).GetAPIKey())
		cfgBase := strings.TrimSpace((*entry).GetBaseURL())
		if attrKey != "" && attrBase != "" {
			if strings.EqualFold(cfgKey, attrKey) && strings.EqualFold(cfgBase, attrBase) {
				return entry
			}
			continue
		}
		if attrKey != "" && strings.EqualFold(cfgKey, attrKey) {
			if cfgBase == "" || strings.EqualFold(cfgBase, attrBase) {
				return entry
			}
			continue
		}
		if attrKey == "" && attrBase != "" && strings.EqualFold(cfgBase, attrBase) {
			return entry
		}
	}

	if attrKey != "" {
		for i := range entries {
			entry := &entries[i]
			if strings.EqualFold(strings.TrimSpace((*entry).GetAPIKey()), attrKey) {
				return entry
			}
		}
	}

	return nil
}

func buildAPIKeyAccessConfigModels[T apiKeyConfigModel](models []T) []*registry.ModelInfo {
	if len(models) == 0 {
		return nil
	}

	out := make([]*registry.ModelInfo, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for i := range models {
		model := models[i]
		name := strings.TrimSpace(model.GetName())
		alias := strings.TrimSpace(model.GetAlias())
		if alias == "" {
			alias = name
		}
		if alias == "" {
			continue
		}
		key := strings.ToLower(alias)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		displayName := name
		if displayName == "" {
			displayName = alias
		}
		out = append(out, &registry.ModelInfo{
			ID:          alias,
			DisplayName: displayName,
		})
	}
	return out
}

func applyAPIKeyAccessExcludedModels(models []*registry.ModelInfo, auth *coreauth.Auth) []*registry.ModelInfo {
	if len(models) == 0 || auth == nil || auth.Attributes == nil {
		return models
	}

	raw := strings.TrimSpace(auth.Attributes["excluded_models"])
	if raw == "" {
		return models
	}

	patterns := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if trimmed := strings.ToLower(strings.TrimSpace(item)); trimmed != "" {
			patterns = append(patterns, trimmed)
		}
	}
	if len(patterns) == 0 {
		return models
	}

	filtered := make([]*registry.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		modelID := strings.ToLower(strings.TrimSpace(model.ID))
		blocked := false
		for _, pattern := range patterns {
			if matchAPIKeyAccessModel(pattern, modelID) {
				blocked = true
				break
			}
		}
		if !blocked {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func applyAPIKeyAccessModelPrefixes(models []*registry.ModelInfo, prefix string, forceModelPrefix bool) []*registry.ModelInfo {
	trimmedPrefix := strings.TrimSpace(prefix)
	if trimmedPrefix == "" || len(models) == 0 {
		return models
	}

	out := make([]*registry.ModelInfo, 0, len(models)*2)
	seen := make(map[string]struct{}, len(models)*2)
	addModel := func(model *registry.ModelInfo) {
		if model == nil {
			return
		}
		modelID := strings.TrimSpace(model.ID)
		if modelID == "" {
			return
		}
		if _, exists := seen[modelID]; exists {
			return
		}
		seen[modelID] = struct{}{}
		out = append(out, model)
	}

	for _, model := range models {
		if model == nil {
			continue
		}
		baseID := strings.TrimSpace(model.ID)
		if baseID == "" {
			continue
		}
		if !forceModelPrefix || trimmedPrefix == baseID {
			addModel(model)
		}
		clone := *model
		clone.ID = trimmedPrefix + "/" + baseID
		addModel(&clone)
	}

	return out
}

func effectiveAPIKeyAccessModelPrefix(auth *coreauth.Auth) string {
	if auth == nil {
		return ""
	}

	prefix := strings.TrimSpace(auth.Prefix)
	if prefix == "" {
		switch strings.ToLower(strings.TrimSpace(auth.Provider)) {
		case "codex-compat":
			return config.DefaultCodexCompatPrefix
		case "copilot-compat", "github-copilot":
			return config.DefaultCopilotCompatPrefix
		}
	}
	return prefix
}

func matchAPIKeyAccessModel(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return pattern == value
	}

	parts := strings.Split(pattern, "*")
	if prefix := parts[0]; prefix != "" {
		if !strings.HasPrefix(value, prefix) {
			return false
		}
		value = value[len(prefix):]
	}
	if suffix := parts[len(parts)-1]; suffix != "" {
		if !strings.HasSuffix(value, suffix) {
			return false
		}
		value = value[:len(value)-len(suffix)]
	}
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		index := strings.Index(value, part)
		if index < 0 {
			return false
		}
		value = value[index+len(part):]
	}
	return true
}
