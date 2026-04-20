package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func multiQueryValues(c *gin.Context, key string) []string {
	raw := c.QueryArray(key)
	if len(raw) == 0 {
		if single := strings.TrimSpace(c.Query(key)); single != "" {
			raw = []string{single}
		}
	}
	seen := make(map[string]struct{}, len(raw))
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			values = append(values, trimmed)
		}
	}
	return values
}

func (h *Handler) GetMonitorSummary(c *gin.Context) {
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	summary, err := usage.QueryDashboardSummary(days, apiKey, model, channelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "summary": summary})
}

func (h *Handler) GetMonitorModelDistribution(c *gin.Context) {
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseManagementIntQuery(c, "limit", 10, managementMaxLimit, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	points, err := usage.QueryModelDistribution(days, limit, apiKey, model, channelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorDailyTrend(c *gin.Context) {
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	points, err := usage.QueryDailyTrend(days, apiKey, model, channelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorHourlySeries(c *gin.Context) {
	hours, err := parseManagementIntQuery(c, "hours", 24, managementMaxHours, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	days := (hours + 23) / 24
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	points, err := usage.QueryHourlySeries(hours, apiKey, model, channelFilter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"hours": hours, "items": points})
}

func (h *Handler) GetMonitorChannelStats(c *gin.Context) {
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseManagementIntQuery(c, "limit", 10, managementMaxLimit, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	channels, models, err := usage.QueryChannelStats(days, apiKey, model, channelFilter, limit)
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
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit, err := parseManagementIntQuery(c, "limit", 10, managementMaxLimit, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	model := strings.TrimSpace(c.Query("model"))
	channelFilter, err := h.resolveMonitorChannelFilter(days, apiKey, multiQueryValues(c, "channel_name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	channels, models, err := usage.QueryFailureStats(days, apiKey, model, channelFilter, limit)
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
	days, err := parseManagementIntQuery(c, "days", 7, managementMaxDays, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	apiKey := strings.TrimSpace(c.Query("api_key"))
	selectedChannels := multiQueryValues(c, "channel_name")
	channelResolver, err := h.buildUsageChannelResolver(usage.LogQueryParams{
		Days:   days,
		APIKey: apiKey,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	channelFilter := channelResolver.ResolveChannelFilter(selectedChannels)

	filters, err := usage.QueryMonitorFilters(days, apiKey, channelFilter)
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
	if filters.ChannelOptions == nil {
		filters.ChannelOptions = []usage.ChannelOption{}
	}
	if filters.APIKeyNames == nil {
		filters.APIKeyNames = map[string]string{}
	}
	filters.Channels = channelResolver.displayChannelNames
	filters.ChannelOptions = channelResolver.channelOptions

	filters.APIKeyNames = make(map[string]string, len(filters.APIKeys))
	for _, key := range filters.APIKeys {
		if name := channelResolver.ResolveAPIKeyName(key); name != "" {
			filters.APIKeyNames[key] = name
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"days":    days,
		"filters": filters,
	})
}

func (h *Handler) resolveMonitorChannelFilter(days int, apiKey string, selected []string) (usage.ChannelFilter, error) {
	if !usageChannelSelectionNeedsRefs(selected) {
		return h.newUsageChannelResolver(nil).ResolveChannelFilter(selected), nil
	}
	channelResolver, err := h.buildUsageChannelResolver(usage.LogQueryParams{
		Days:   days,
		APIKey: apiKey,
	})
	if err != nil {
		return usage.ChannelFilter{}, err
	}
	return channelResolver.ResolveChannelFilter(selected), nil
}

func usageChannelSelectionNeedsRefs(selected []string) bool {
	for _, item := range selected {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, ok := parseUsageAuthChannelToken(value); ok {
			continue
		}
		if _, ok := parseUsageLegacyAuthChannelToken(value); ok {
			continue
		}
		if _, ok := parseUsageLegacyNameToken(value); ok {
			continue
		}
		if _, ok := parseUsageSourceChannelToken(value); ok {
			continue
		}
		return true
	}
	return false
}
