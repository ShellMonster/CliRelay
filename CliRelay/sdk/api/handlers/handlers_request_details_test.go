package handlers

import (
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	accessrestrictions "github.com/router-for-me/CLIProxyAPI/v6/internal/access/restrictions"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
	"golang.org/x/net/context"
)

func TestGetRequestDetails_PreservesSuffix(t *testing.T) {
	modelRegistry := registry.GetGlobalRegistry()
	now := time.Now().Unix()

	modelRegistry.RegisterClient("test-request-details-gemini", "gemini", []*registry.ModelInfo{
		{ID: "gemini-2.5-pro", Created: now + 30},
		{ID: "gemini-2.5-flash", Created: now + 25},
	})
	modelRegistry.RegisterClient("test-request-details-openai", "openai", []*registry.ModelInfo{
		{ID: "gpt-5.2", Created: now + 20},
	})
	modelRegistry.RegisterClient("test-request-details-openai-2", "openai", []*registry.ModelInfo{
		{ID: "gpt-5.2", Created: now + 15},
	})
	modelRegistry.RegisterClient("test-request-details-claude", "claude", []*registry.ModelInfo{
		{ID: "claude-sonnet-4-5", Created: now + 5},
	})

	// Ensure cleanup of all test registrations.
	clientIDs := []string{
		"test-request-details-gemini",
		"test-request-details-openai",
		"test-request-details-openai-2",
		"test-request-details-claude",
	}
	for _, clientID := range clientIDs {
		id := clientID
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(id)
		})
	}

	authManager := coreauth.NewManager(nil, nil, nil)
	for _, auth := range []*coreauth.Auth{
		{ID: "test-request-details-gemini", Provider: "gemini", Label: "Gemini A"},
		{ID: "test-request-details-openai", Provider: "openai", Label: "OpenAI A"},
		{ID: "test-request-details-openai-2", Provider: "openai", Label: "OpenAI B"},
		{ID: "test-request-details-claude", Provider: "claude", Label: "Claude A"},
	} {
		if _, err := authManager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth %s: %v", auth.ID, err)
		}
	}

	handler := NewBaseAPIHandlers(&sdkconfig.SDKConfig{}, authManager)

	tests := []struct {
		name          string
		inputModel    string
		metadata      map[string]string
		wantProviders []string
		wantAuthIDs   []string
		wantModel     string
		wantErr       bool
	}{
		{
			name:          "numeric suffix preserved",
			inputModel:    "gemini-2.5-pro(8192)",
			metadata:      nil,
			wantProviders: []string{"gemini"},
			wantAuthIDs:   nil,
			wantModel:     "gemini-2.5-pro(8192)",
			wantErr:       false,
		},
		{
			name:          "level suffix preserved",
			inputModel:    "gpt-5.2(high)",
			metadata:      nil,
			wantProviders: []string{"openai"},
			wantAuthIDs:   nil,
			wantModel:     "gpt-5.2(high)",
			wantErr:       false,
		},
		{
			name:          "no suffix unchanged",
			inputModel:    "claude-sonnet-4-5",
			metadata:      nil,
			wantProviders: []string{"claude"},
			wantAuthIDs:   nil,
			wantModel:     "claude-sonnet-4-5",
			wantErr:       false,
		},
		{
			name:          "unknown model with suffix",
			inputModel:    "unknown-model(8192)",
			metadata:      nil,
			wantProviders: nil,
			wantModel:     "",
			wantErr:       true,
		},
		{
			name:          "auto suffix resolved",
			inputModel:    "auto(high)",
			metadata:      nil,
			wantProviders: []string{"gemini"},
			wantAuthIDs:   nil,
			wantModel:     "gemini-2.5-pro(high)",
			wantErr:       false,
		},
		{
			name:          "special suffix none preserved",
			inputModel:    "gemini-2.5-flash(none)",
			metadata:      nil,
			wantProviders: []string{"gemini"},
			wantAuthIDs:   nil,
			wantModel:     "gemini-2.5-flash(none)",
			wantErr:       false,
		},
		{
			name:          "special suffix auto preserved",
			inputModel:    "claude-sonnet-4-5(auto)",
			metadata:      nil,
			wantProviders: []string{"claude"},
			wantAuthIDs:   nil,
			wantModel:     "claude-sonnet-4-5(auto)",
			wantErr:       false,
		},
		{
			name:       "provider restriction filters openai model",
			inputModel: "gpt-5.2(high)",
			metadata: func() map[string]string {
				meta := accessrestrictions.BuildRestrictionMetadata(nil, []sdkconfig.APIKeyProviderAccess{
					{Provider: "gemini"},
				})
				return meta
			}(),
			wantProviders: nil,
			wantAuthIDs:   nil,
			wantModel:     "",
			wantErr:       true,
		},
		{
			name:       "provider scoped model restriction allows suffix match",
			inputModel: "gpt-5.2(high)",
			metadata: func() map[string]string {
				meta := accessrestrictions.BuildRestrictionMetadata(nil, []sdkconfig.APIKeyProviderAccess{
					{Provider: "openai", Models: []string{"gpt-5.2"}},
				})
				return meta
			}(),
			wantProviders: []string{"openai"},
			wantAuthIDs:   []string{"test-request-details-openai", "test-request-details-openai-2"},
			wantModel:     "gpt-5.2(high)",
			wantErr:       false,
		},
		{
			name:       "channel restriction narrows auth ids",
			inputModel: "gpt-5.2(high)",
			metadata: func() map[string]string {
				meta := accessrestrictions.BuildRestrictionMetadata(nil, []sdkconfig.APIKeyProviderAccess{
					{Provider: "openai", Channels: []string{"test-request-details-openai"}},
				})
				return meta
			}(),
			wantProviders: []string{"openai"},
			wantAuthIDs:   []string{"test-request-details-openai"},
			wantModel:     "gpt-5.2(high)",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if len(tt.metadata) > 0 {
				gin.SetMode(gin.TestMode)
				recorder := httptest.NewRecorder()
				ginCtx, _ := gin.CreateTestContext(recorder)
				ginCtx.Set("accessMetadata", tt.metadata)
				ctx = context.WithValue(ctx, "gin", ginCtx)
			}

			providers, model, allowedAuthIDs, errMsg := handler.getRequestDetails(ctx, tt.inputModel)
			if (errMsg != nil) != tt.wantErr {
				t.Fatalf("getRequestDetails() error = %v, wantErr %v", errMsg, tt.wantErr)
			}
			if errMsg != nil {
				return
			}
			sort.Strings(allowedAuthIDs)
			if !reflect.DeepEqual(providers, tt.wantProviders) {
				t.Fatalf("getRequestDetails() providers = %v, want %v", providers, tt.wantProviders)
			}
			if !reflect.DeepEqual(allowedAuthIDs, tt.wantAuthIDs) {
				t.Fatalf("getRequestDetails() allowedAuthIDs = %v, want %v", allowedAuthIDs, tt.wantAuthIDs)
			}
			if model != tt.wantModel {
				t.Fatalf("getRequestDetails() model = %v, want %v", model, tt.wantModel)
			}
		})
	}
}
