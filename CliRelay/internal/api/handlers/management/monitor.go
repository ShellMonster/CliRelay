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
	summary, err := usage.QueryDashboardSummary(days, apiKey)
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
	points, err := usage.QueryModelDistribution(days, limit, apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorDailyTrend(c *gin.Context) {
	days := intQueryDefault(c, "days", 7)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	points, err := usage.QueryDailyTrend(days, apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days, "items": points})
}

func (h *Handler) GetMonitorHourlySeries(c *gin.Context) {
	hours := intQueryDefault(c, "hours", 24)
	apiKey := strings.TrimSpace(c.Query("api_key"))
	points, err := usage.QueryHourlySeries(hours, apiKey)
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
	channels, models, err := usage.QueryChannelStats(days, apiKey, limit)
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
	channels, models, err := usage.QueryFailureStats(days, apiKey, limit)
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
