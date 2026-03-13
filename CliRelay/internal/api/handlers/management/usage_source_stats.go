package management

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func (h *Handler) GetUsageSourceStats(c *gin.Context) {
	days := intQueryDefault(c, "days", 30)
	recentMinutes := intQueryDefault(c, "recent_minutes", 200)
	blockMinutes := intQueryDefault(c, "block_minutes", 10)

	items, err := usage.QueryUsageSourceStats(days, recentMinutes, blockMinutes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"days":           days,
		"recent_minutes": recentMinutes,
		"block_minutes":  blockMinutes,
		"items":          items,
	})
}
