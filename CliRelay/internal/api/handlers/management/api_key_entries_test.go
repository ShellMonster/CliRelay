package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func TestPutAPIKeyEntries_RemovesCoveredLegacyAPIKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	initial := "request-log: false\napi-keys:\n  - dup\n  - keep\n"
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
	req := httptest.NewRequest(http.MethodPut, "/v0/management/api-key-entries", strings.NewReader(`[{"key":"dup","name":"Dupe"}]`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	handler.PutAPIKeyEntries(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	reloaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	if len(reloaded.APIKeyEntries) != 1 || reloaded.APIKeyEntries[0].Key != "dup" {
		t.Fatalf("expected persisted api-key-entry for dup, got %#v", reloaded.APIKeyEntries)
	}
	if len(reloaded.APIKeys) != 1 || reloaded.APIKeys[0] != "keep" {
		t.Fatalf("expected only unmatched legacy api-key to remain, got %#v", reloaded.APIKeys)
	}
}

func TestDeleteAPIKeyEntry_RemovesMatchingLegacyAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	initial := "" +
		"request-log: false\n" +
		"api-keys:\n" +
		"  - dup\n" +
		"  - keep\n" +
		"api-key-entries:\n" +
		"  - key: dup\n" +
		"    name: Dupe\n"
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
	req := httptest.NewRequest(http.MethodDelete, "/v0/management/api-key-entries?index=0", nil)
	ctx.Request = req

	handler.DeleteAPIKeyEntry(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	reloaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	if len(reloaded.APIKeyEntries) != 0 {
		t.Fatalf("expected api-key-entries to be empty after delete, got %#v", reloaded.APIKeyEntries)
	}
	if len(reloaded.APIKeys) != 1 || reloaded.APIKeys[0] != "keep" {
		t.Fatalf("expected matching legacy api-key to be removed too, got %#v", reloaded.APIKeys)
	}
}
