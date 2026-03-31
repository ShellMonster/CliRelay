package modelsync

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/runtime/executor"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
)

const defaultSyncInterval = 30 * time.Minute

type ConfigManager interface {
	UpdateConfig(func(*config.Config) bool) error
}

type Service struct {
	manager  ConfigManager
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

func New(manager ConfigManager, interval time.Duration) *Service {
	if interval <= 0 {
		interval = defaultSyncInterval
	}
	return &Service{
		manager:  manager,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.manager == nil {
		return
	}
	go s.loop(ctx)
}

func (s *Service) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *Service) loop(ctx context.Context) {
	s.runOnce(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Service) runOnce(ctx context.Context) {
	if err := s.manager.UpdateConfig(func(cfg *config.Config) bool {
		changed, syncErr := syncCodexProviders(ctx, cfg)
		if syncErr != nil {
			log.WithError(syncErr).Warn("model sync failed")
			return false
		}
		return changed
	}); err != nil {
		log.WithError(err).Warn("failed to persist synced models")
	}
}

func syncCodexProviders(ctx context.Context, cfg *config.Config) (bool, error) {
	if cfg == nil {
		return false, nil
	}

	changed := false
	for i := range cfg.CodexKey {
		entryChanged, err := syncCodexKey(ctx, &cfg.CodexKey[i], "codex")
		if err != nil {
			return changed, err
		}
		changed = changed || entryChanged
	}
	for i := range cfg.CodexCompatKey {
		entryChanged, err := syncCodexKey(ctx, &cfg.CodexCompatKey[i], "codex-compat")
		if err != nil {
			return changed, err
		}
		changed = changed || entryChanged
	}
	for i := range cfg.CopilotCompatKey {
		entryChanged, err := syncCodexKey(ctx, &cfg.CopilotCompatKey[i], "copilot-compat")
		if err != nil {
			return changed, err
		}
		changed = changed || entryChanged
	}
	return changed, nil
}

func syncCodexKey(ctx context.Context, entry *config.CodexKey, provider string) (bool, error) {
	if entry == nil || !entry.AutoSyncModels {
		return false, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models := executor.FetchCodexModels(fetchCtx, buildCodexAuth(entry, provider), &config.Config{})
	if len(models) == 0 {
		return false, nil
	}
	return mergeCodexModels(entry, models), nil
}

func buildCodexAuth(entry *config.CodexKey, provider string) *coreauth.Auth {
	apiKey := strings.TrimSpace(entry.APIKey)
	baseURL := strings.TrimSpace(entry.BaseURL)
	proxyURL := strings.TrimSpace(entry.ProxyURL)
	attrs := map[string]string{
		"api_key":  apiKey,
		"base_url": baseURL,
	}
	for key, value := range entry.Headers {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		attrs[trimmedKey] = trimmedValue
	}

	idSource := provider + "|" + apiKey + "|" + baseURL
	return &coreauth.Auth{
		ID:         fmt.Sprintf("model-sync:%x", idSource),
		Provider:   provider,
		ProxyURL:   proxyURL,
		Attributes: attrs,
	}
}

func mergeCodexModels(entry *config.CodexKey, models []*registry.ModelInfo) bool {
	if entry == nil || len(models) == 0 {
		return false
	}

	seen := make(map[string]struct{}, len(entry.Models))
	for _, model := range entry.Models {
		name := strings.ToLower(strings.TrimSpace(model.Name))
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	changed := false
	for _, model := range models {
		if model == nil {
			continue
		}
		name := strings.TrimSpace(model.ID)
		if name == "" {
			name = strings.TrimSpace(model.Name)
		}
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		entry.Models = append(entry.Models, config.CodexModel{Name: name})
		seen[key] = struct{}{}
		changed = true
	}
	return changed
}
