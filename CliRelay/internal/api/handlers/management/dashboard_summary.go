package management

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetDashboardSummary is a lightweight endpoint that returns only the
// counts and KPIs needed by the frontend dashboard page, avoiding
// the transfer of the full usage / config payloads.
//
// GET /v0/management/dashboard-summary?days=7
func (h *Handler) GetDashboardSummary(c *gin.Context) {
	cfg := h.cfg

	// ── Provider key counts ──
	geminiCount := 0
	claudeCount := 0
	codexCount := 0
	vertexCount := 0
	openaiCount := 0
	authFileCount := 0
	apiKeyCount := 0

	if cfg != nil {
		geminiCount = len(cfg.GeminiKey)
		claudeCount = len(cfg.ClaudeKey)
		codexCount = len(cfg.CodexKey)
		vertexCount = len(cfg.VertexCompatAPIKey)
		openaiCount = len(cfg.OpenAICompatibility)
		apiKeyCount = len(cfg.APIKeyEntries)
	}

	if h.authManager != nil {
		for _, auth := range h.authManager.List() {
			if entry := h.buildAuthFileEntry(auth); entry != nil {
				authFileCount++
			}
		}
	}

	providerTotal := geminiCount + claudeCount + codexCount + vertexCount + openaiCount

	// ── Usage KPIs (time-filtered) ──
	daysStr := c.DefaultQuery("days", "7")
	days := 7
	if v, err := parsePositiveInt(daysStr); err == nil && v > 0 {
		days = v
	}

	usageSummary, err := usage.QueryDashboardSummary(days, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"kpi": gin.H{
			"total_requests":   usageSummary.TotalRequests,
			"success_requests": usageSummary.SuccessRequests,
			"failed_requests":  usageSummary.FailedRequests,
			"success_rate":     usageSummary.SuccessRate,
			"input_tokens":     usageSummary.InputTokens,
			"output_tokens":    usageSummary.OutputTokens,
			"reasoning_tokens": usageSummary.ReasoningTokens,
			"cached_tokens":    usageSummary.CachedTokens,
			"total_tokens":     usageSummary.TotalTokens,
		},
		"counts": gin.H{
			"api_keys":         apiKeyCount,
			"providers_total":  providerTotal,
			"gemini_keys":      geminiCount,
			"claude_keys":      claudeCount,
			"codex_keys":       codexCount,
			"vertex_keys":      vertexCount,
			"openai_providers": openaiCount,
			"auth_files":       authFileCount,
		},
		"days": days,
	})
}

func parsePositiveInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
