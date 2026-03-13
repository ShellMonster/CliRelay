package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) GetUsageModelStats(c *gin.Context) {
	days := intQueryDefault(c, "days", 30)
	limit := intQueryDefault(c, "limit", 500)

	items, err := usage.QueryUsageModelDetailStats(days, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days":  days,
		"items": items,
	})
}
