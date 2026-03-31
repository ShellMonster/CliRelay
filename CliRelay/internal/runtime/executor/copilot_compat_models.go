package executor

import (
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func CopilotCompatFallbackModels() []*registry.ModelInfo {
	merged := make(map[string]*registry.ModelInfo)
	appendModels := func(models []*registry.ModelInfo, owner string) {
		for _, model := range models {
			if model == nil || strings.TrimSpace(model.ID) == "" {
				continue
			}
			id := strings.TrimSpace(model.ID)
			if _, exists := merged[id]; exists {
				continue
			}
			cloned := cloneRegistryModelInfo(model)
			if strings.TrimSpace(cloned.OwnedBy) == "" {
				cloned.OwnedBy = owner
			}
			if strings.TrimSpace(cloned.DisplayName) == "" {
				cloned.DisplayName = cloned.ID
			}
			merged[id] = cloned
		}
	}

	appendModels(registry.GetOpenAIModels(), "github-copilot")
	appendModels(registry.GetClaudeModels(), "github-copilot")
	appendModels(registry.GetGeminiModels(), "github-copilot")

	ids := make([]string, 0, len(merged))
	for id := range merged {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return strings.ToLower(ids[i]) < strings.ToLower(ids[j])
	})

	out := make([]*registry.ModelInfo, 0, len(ids))
	for _, id := range ids {
		out = append(out, merged[id])
	}
	return out
}

func cloneRegistryModelInfo(model *registry.ModelInfo) *registry.ModelInfo {
	if model == nil {
		return nil
	}
	copyModel := *model
	if len(model.SupportedGenerationMethods) > 0 {
		copyModel.SupportedGenerationMethods = append([]string(nil), model.SupportedGenerationMethods...)
	}
	if len(model.SupportedParameters) > 0 {
		copyModel.SupportedParameters = append([]string(nil), model.SupportedParameters...)
	}
	return &copyModel
}
