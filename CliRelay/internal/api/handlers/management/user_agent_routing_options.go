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

	providerSeen := make(map[string]userAgentRoutingProviderOption)
	modelSeen := make(map[string]userAgentRoutingModelOption)
	reg := registry.GetGlobalRegistry()

	for _, auth := range manager.List() {
		if auth == nil {
			continue
		}

		providerID := normalizeAccessOptionProvider(accessrestrictions.ProviderFamilyForAuth(auth))
		if providerID != "" {
			if _, exists := providerSeen[providerID]; !exists {
				providerSeen[providerID] = userAgentRoutingProviderOption{
					ID:    providerID,
					Label: apiKeyAccessProviderLabel(providerID),
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

	providers := make([]userAgentRoutingProviderOption, 0, len(providerSeen))
	for _, option := range providerSeen {
		providers = append(providers, option)
	}
	sort.Slice(providers, func(i, j int) bool {
		left, right := providers[i], providers[j]
		leftOrder, leftExists := apiKeyAccessProviderOrder[left.ID]
		rightOrder, rightExists := apiKeyAccessProviderOrder[right.ID]
		switch {
		case leftExists && rightExists && leftOrder != rightOrder:
			return leftOrder < rightOrder
		case leftExists != rightExists:
			return leftExists
		default:
			return strings.ToLower(left.Label) < strings.ToLower(right.Label)
		}
	})

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
