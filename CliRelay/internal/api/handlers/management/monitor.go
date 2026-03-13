package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) GetMonitorSummary(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	summary, err := usage.QueryDashboardSummary(days, apiKey, model, channelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "summary": summary})
}

func (h *Handler) GetMonitorModelDistribution(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	limit := intQueryDefault(c, "limit", 10)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	points, err := usage.QueryModelDistribution(days, limit, apiKey, model, channelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorDailyTrend(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	points, err := usage.QueryDailyTrend(days, apiKey, model, channelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorHourlySeries(c *gin.Context) {
	hours := intQueryDefault(c, "hours", 24)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	points, err := usage.QueryHourlySeries(hours, apiKey, model, channelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hours": hours, "items": points})
}

func (h *Handler) GetMonitorChannelStats(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	limit := intQueryDefault(c, "limit", 10)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	channels, models, err := usage.QueryChannelStats(days, apiKey, model, channelName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"days":     days,
		"channels": channels,
		"models":   models,
	})
}

func (h *Handler) GetMonitorFailureStats(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	limit := intQueryDefault(c, "limit", 10)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelName := strings.TrimSpace(c.Query("channel_name"))
	channels, models, err := usage.QueryFailureStats(days, apiKey, model, channelName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"days":     days,
		"channels": channels,
		"models":   models,
	})
}

func (h *Handler) GetMonitorFilters(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	channelName := strings.TrimSpace(c.Query("channel_name"))

	filters, err := usage.QueryMonitorFilters(days, apiKey, channelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if filters.APIKeys == nil {
		filters.APIKeys = []string{}
	}
	if filters.Models == nil {
		filters.Models = []string{}
	}
	if filters.Channels == nil {
		filters.Channels = []string{}
	}
	if filters.APIKeyNames == nil {
		filters.APIKeyNames = map[string]string{}
	}

	keyNameMap, _ := h.buildNameMaps()
	filters.APIKeyNames = make(map[string]string, len(filters.APIKeys))
	for _, key := range filters.APIKeys {
		if name, ok := keyNameMap[key]; ok {
			filters.APIKeyNames[key] = name
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"days":    days,
		"filters": filters,
	})
}
