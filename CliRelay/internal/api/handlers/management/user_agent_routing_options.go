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

type userAgentRoutingProviderOption struct {
	ID       string                          `json:"id"`
	Label    string                          `json:"label"`
	Channels []userAgentRoutingChannelOption `json:"channels"`
}

type userAgentRoutingChannelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type userAgentRoutingModelOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type userAgentRoutingOptions struct {
	Providers []userAgentRoutingProviderOption `json:"providers"`
	Models    []userAgentRoutingModelOption    `json:"models"`
}

// GetUserAgentRoutingOptions returns provider family and model options for UA routing rule editing.
func (h *Handler) GetUserAgentRoutingOptions(c *gin.Context) {
	options := buildUserAgentRoutingOptions(h.cfg, h.authManager)
	c.JSON(http.StatusOK, options)
}

func buildUserAgentRoutingOptions(cfg *config.Config, manager *coreauth.Manager) userAgentRoutingOptions {
	if manager == nil {
		return userAgentRoutingOptions{}
	}

	type providerState struct {
		id       string
		label    string
		channels map[string]userAgentRoutingChannelOption
	}

	providerStateByID := make(map[string]*providerState)
	providerOrder := make([]string, 0)
	modelSeen := make(map[string]userAgentRoutingModelOption)
	reg := registry.GetGlobalRegistry()

	ensureProvider := func(providerID string) *providerState {
		providerID = normalizeAccessOptionProvider(providerID)
		if providerID == "" {
			return nil
		}
		if state := providerStateByID[providerID]; state != nil {
			return state
		}
		state := &providerState{
			id:       providerID,
			label:    apiKeyAccessProviderLabel(providerID),
			channels: make(map[string]userAgentRoutingChannelOption),
		}
		providerStateByID[providerID] = state
		providerOrder = append(providerOrder, providerID)
		return state
	}

	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}

		providerID := normalizeAccessOptionProvider(accessrestrictions.ProviderFamilyForAuth(auth))
		state := ensureProvider(providerID)
		if state != nil {
			channelID := strings.TrimSpace(accessrestrictions.ChannelIDForAuth(auth))
			if channelID != "" {
				label := strings.TrimSpace(accessrestrictions.ChannelLabelForAuth(auth))
				if label == "" {
					label = channelID
				}
				state.channels[channelID] = userAgentRoutingChannelOption{
					ID:    channelID,
					Label: label,
				}
			}
		}

		for _, model := range resolveAPIKeyAccessModels(cfg, reg, auth) {
			if model == nil {
				continue
			}
			modelID := normalizeUserAgentRoutingOptionModelID(auth, model.ID)
			if modelID == "" {
				continue
			}
			key := strings.ToLower(modelID)
			if _, exists := modelSeen[key]; exists {
				continue
			}
			label := strings.TrimSpace(model.DisplayName)
			if label == "" {
				label = modelID
			}
			modelSeen[key] = userAgentRoutingModelOption{
				ID:    modelID,
				Label: label,
			}
		}
	}

	sort.Slice(providerOrder, func(i, j int) bool {
		left, right := providerOrder[i], providerOrder[j]
		leftOrder, leftExists := apiKeyAccessProviderOrder[left]
		rightOrder, rightExists := apiKeyAccessProviderOrder[right]
		switch {
		case leftExists && rightExists && leftOrder != rightOrder:
			return leftOrder < rightOrder
		case leftExists != rightExists:
			return leftExists
		default:
			return strings.ToLower(apiKeyAccessProviderLabel(left)) < strings.ToLower(apiKeyAccessProviderLabel(right))
		}
	})

	providers := make([]userAgentRoutingProviderOption, 0, len(providerOrder))
	for _, providerID := range providerOrder {
		state := providerStateByID[providerID]
		if state == nil {
			continue
		}

		channelIDs := make([]string, 0, len(state.channels))
		for channelID := range state.channels {
			channelIDs = append(channelIDs, channelID)
		}
		sort.Slice(channelIDs, func(i, j int) bool {
			left := state.channels[channelIDs[i]]
			right := state.channels[channelIDs[j]]
			leftLabel := strings.ToLower(strings.TrimSpace(left.Label))
			rightLabel := strings.ToLower(strings.TrimSpace(right.Label))
			if leftLabel == rightLabel {
				return channelIDs[i] < channelIDs[j]
			}
			return leftLabel < rightLabel
		})

		channels := make([]userAgentRoutingChannelOption, 0, len(channelIDs))
		for _, channelID := range channelIDs {
			channels = append(channels, state.channels[channelID])
		}

		providers = append(providers, userAgentRoutingProviderOption{
			ID:       state.id,
			Label:    state.label,
			Channels: channels,
		})
	}

	models := make([]userAgentRoutingModelOption, 0, len(modelSeen))
	for _, option := range modelSeen {
		models = append(models, option)
	}
	sort.Slice(models, func(i, j int) bool {
		leftLabel := strings.ToLower(strings.TrimSpace(models[i].Label))
		rightLabel := strings.ToLower(strings.TrimSpace(models[j].Label))
		if leftLabel == rightLabel {
			return strings.ToLower(models[i].ID) < strings.ToLower(models[j].ID)
		}
		return leftLabel < rightLabel
	})

	return userAgentRoutingOptions{
		Providers: providers,
		Models:    models,
	}
}

func normalizeUserAgentRoutingOptionModelID(auth *coreauth.Auth, modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if trimmed == "" {
		return ""
	}

	prefix := strings.TrimSpace(effectiveAPIKeyAccessModelPrefix(auth))
	if prefix == "" {
		return trimmed
	}

	needle := prefix + "/"
	if len(trimmed) <= len(needle) || !strings.EqualFold(trimmed[:len(needle)], needle) {
		return trimmed
	}

	return strings.TrimSpace(trimmed[len(needle):])
}
