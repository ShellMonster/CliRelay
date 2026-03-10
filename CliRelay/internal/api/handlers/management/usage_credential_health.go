package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) GetUsageCredentialHealth(c *gin.Context) {
	days := intQueryDefault(c, "days", 30)
	apiKey := strings.TrimSpace(c.Query("api_key"))

	items, err := usage.QueryUsageCredentialHealth(days, apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days":  days,
		"items": items,
	})
}
