package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestDashboardSummaryMissingDaysUsesDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/dashboard-summary", nil)

	handler.GetDashboardSummary(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for missing days, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Days int `json:"days"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.Days != 7 {
		t.Fatalf("expected missing days to default to 7, got %d", payload.Days)
	}
}

func TestDashboardSummaryMalformedDaysReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/dashboard-summary?days=abc", nil)

	handler.GetDashboardSummary(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed days, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestDashboardSummaryNegativeDays(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/dashboard-summary?days=-1", nil)

	handler.GetDashboardSummary(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for negative days, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
