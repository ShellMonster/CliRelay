package management

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) GetUsageOverview(c *gin.Context) {
	days := intQueryDefault(c, "days", 30)
	apiKey := strings.TrimSpace(c.Query("api_key"))

	overview, err := usage.QueryUsageOverview(days, apiKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, overview)
}
