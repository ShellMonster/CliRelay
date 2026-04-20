package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestMonitorModelDistributionMissingParamsUseDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// nil usage service is safe: validation rejects bad params before reaching usage calls.
	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/monitor/model-distribution", nil)

	handler.GetMonitorModelDistribution(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for missing params, got %d: %s", recorder.Code, recorder.Body.String())
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

func TestMonitorModelDistributionMalformedDaysReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/monitor/model-distribution?days=abc&limit=10", nil)

	handler.GetMonitorModelDistribution(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed days, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMonitorModelDistributionMalformedLimitReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/monitor/model-distribution?days=7&limit=abc", nil)

	handler.GetMonitorModelDistribution(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed limit, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestMonitorModelDistributionUpperBound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/monitor/model-distribution?days=7&limit=1000000", nil)

	handler.GetMonitorModelDistribution(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for oversized limit, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUsageLogsMissingParamsUseDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for missing params, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Page int `json:"page"`
		Size int `json:"size"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.Page != 1 {
		t.Fatalf("expected missing page to default to 1, got %d", payload.Page)
	}
	if payload.Size != 50 {
		t.Fatalf("expected missing size to default to 50, got %d", payload.Size)
	}
}

func TestUsageLogsMalformedDaysReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=1&size=50&days=abc", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed days, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUsageLogsMalformedSizeReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=1&size=abc&days=7", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed size, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUsageLogsMalformedPageReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=abc&size=50&days=7", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for malformed page, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUsageLogsUpperBound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=1&size=1000000&days=7", nil)

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for oversized size, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
