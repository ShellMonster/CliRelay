package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestGetUsageLogs_EmptyDatabaseReturnsStableShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=1&size=50&days=7", nil)
	ctx.Request = req

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Items   []any `json:"items"`
		Total   int64 `json:"total"`
		Page    int   `json:"page"`
		Size    int   `json:"size"`
		Filters struct {
			APIKeys        []string          `json:"api_keys"`
			APIKeyNames    map[string]string `json:"api_key_names"`
			Models         []string          `json:"models"`
			Channels       []string          `json:"channels"`
			ChannelOptions []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"channel_options"`
		} `json:"filters"`
		Stats struct {
			Total       int64   `json:"total"`
			SuccessRate float64 `json:"success_rate"`
			TotalTokens int64   `json:"total_tokens"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if payload.Items == nil {
		t.Fatal("expected items to be an empty array, got null")
	}
	if payload.Filters.APIKeys == nil {
		t.Fatal("expected filters.api_keys to be an empty array, got null")
	}
	if payload.Filters.Models == nil {
		t.Fatal("expected filters.models to be an empty array, got null")
	}
	if payload.Filters.Channels == nil {
		t.Fatal("expected filters.channels to be an empty array, got null")
	}
	if payload.Filters.ChannelOptions == nil {
		t.Fatal("expected filters.channel_options to be an empty array, got null")
	}
	if payload.Filters.APIKeyNames == nil {
		t.Fatal("expected filters.api_key_names to be an empty object, got null")
	}
	if payload.Total != 0 || payload.Stats.Total != 0 || payload.Stats.SuccessRate != 0 || payload.Stats.TotalTokens != 0 {
		t.Fatalf("expected zero-value stats for empty database, got %+v total=%d", payload.Stats, payload.Total)
	}
}

func TestGetUsageLogs_EmptyDatabaseIncludesConfiguredChannelOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(&config.Config{
		CodexCompatKey: []config.CodexKey{
			{
				APIKey:  "sk-compat",
				Name:    "Github_Compat",
				BaseURL: "https://api.githubcopilot.com",
			},
		},
	}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs?page=1&size=50&days=7", nil)
	ctx.Request = req

	handler.GetUsageLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Filters struct {
			Channels       []string `json:"channels"`
			ChannelOptions []struct {
				Value string `json:"value"`
				Label string `json:"label"`
			} `json:"channel_options"`
		} `json:"filters"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(payload.Filters.ChannelOptions) != 1 {
		t.Fatalf("expected configured channel option in empty db response, got %+v", payload.Filters.ChannelOptions)
	}
	if payload.Filters.ChannelOptions[0].Label != "Github_Compat" {
		t.Fatalf("expected configured channel label, got %+v", payload.Filters.ChannelOptions)
	}
	if len(payload.Filters.Channels) != 1 || payload.Filters.Channels[0] != "Github_Compat" {
		t.Fatalf("expected configured channel label in compatibility field, got %+v", payload.Filters.Channels)
	}
}
