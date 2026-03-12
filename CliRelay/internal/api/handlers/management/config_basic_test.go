package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

func TestPutUsageLogContentEnabled_PersistsConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	initial := "usage-log-content-enabled: true\nrequest-log: false\n"
	if err := os.WriteFile(configPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	handler := NewHandler(cfg, configPath, nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPut, "/v0/management/usage-log-content-enabled", strings.NewReader(`{"value":false}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	handler.PutUsageLogContentEnabled(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	reloaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if reloaded.UsageLogContentEnabled {
		t.Fatal("expected usage-log-content-enabled to persist as false")
	}
}

func TestGetLogContent_ExposesContentDisabledFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usageDBDir := t.TempDir()
	dbPath := filepath.Join(usageDBDir, "usage.db")
	if err := usage.InitDB(dbPath); err != nil {
		t.Fatalf("init usage db: %v", err)
	}
	t.Cleanup(usage.CloseDB)

	usage.SetLogContentEnabled(false)
	t.Cleanup(func() { usage.SetLogContentEnabled(true) })

	usage.InsertLog("k", "m", "s", "c", "a", false, time.Now(), 12, usage.TokenStats{}, "", "", nil)

	handler := NewHandler(&config.Config{}, "", nil)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/v0/management/usage/logs/1/content", nil)
	ctx.Request = req
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}

	handler.GetLogContent(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		ContentDisabled bool `json:"content_disabled"`
		HasContent      bool `json:"has_content"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.ContentDisabled {
		t.Fatal("expected content_disabled=true when content logging is off")
	}
	if payload.HasContent {
		t.Fatal("expected has_content=false for empty stored content")
	}
}
