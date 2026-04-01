package modelsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Snapshot() (*config.Config, error)
	UpdateConfig(func(*config.Config) bool) error
}

type Service struct {
	manager  ConfigManager
	interval time.Duration
	stopCh   chan struct{}
	stopOnce sync.Once
}

type providerSyncPlan struct {
	targets []codexSyncTarget
}

type codexSyncTarget struct {
	kind        string
	apiKey      string
	baseURL     string
	proxyURL    string
	headers     map[string]string
	modelsToAdd []config.CodexModel
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
	snapshot, err := s.manager.Snapshot()
	if err != nil {
		log.WithError(err).Warn("failed to snapshot config for model sync")
		return
	}
	plan, err := buildSyncPlan(ctx, snapshot)
	if err != nil {
		log.WithError(err).Warn("model sync failed")
		return
	}
	if len(plan.targets) == 0 {
		return
	}

	if err := s.manager.UpdateConfig(func(cfg *config.Config) bool {
		return applySyncPlan(cfg, plan)
	}); err != nil {
		log.WithError(err).Warn("failed to persist synced models")
	}
}

func buildSyncPlan(ctx context.Context, cfg *config.Config) (*providerSyncPlan, error) {
	if cfg == nil {
		return &providerSyncPlan{}, nil
	}

	targets := make([]codexSyncTarget, 0)
	for _, entry := range cfg.CodexKey {
		target, err := buildCodexTarget(ctx, entry, "codex")
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	for _, entry := range cfg.CodexCompatKey {
		target, err := buildCodexTarget(ctx, entry, "codex-compat")
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	for _, entry := range cfg.CopilotCompatKey {
		target, err := buildCodexTarget(ctx, entry, "copilot-compat")
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	return &providerSyncPlan{targets: targets}, nil
}

func buildCodexTarget(ctx context.Context, entry config.CodexKey, provider string) (*codexSyncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	models := executor.FetchCodexModels(fetchCtx, buildCodexAuth(entry, provider), &config.Config{})
	if len(models) == 0 {
		return nil, nil
	}

	modelsToAdd := collectMissingCodexModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &codexSyncTarget{
		kind:        provider,
		apiKey:      strings.TrimSpace(entry.APIKey),
		baseURL:     strings.TrimSpace(entry.BaseURL),
		proxyURL:    strings.TrimSpace(entry.ProxyURL),
		headers:     cloneHeaders(entry.Headers),
		modelsToAdd: modelsToAdd,
	}, nil
}

func buildCodexAuth(entry config.CodexKey, provider string) *coreauth.Auth {
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

	sum := sha256.Sum256([]byte(provider + "|" + apiKey + "|" + baseURL))
	return &coreauth.Auth{
		ID:         "model-sync:" + hex.EncodeToString(sum[:8]),
		Provider:   provider,
		ProxyURL:   proxyURL,
		Attributes: attrs,
	}
}

func collectMissingCodexModels(existing []config.CodexModel, models []*registry.ModelInfo) []config.CodexModel {
	if len(models) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		name := strings.ToLower(strings.TrimSpace(model.Name))
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	out := make([]config.CodexModel, 0)
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
		seen[key] = struct{}{}
		out = append(out, config.CodexModel{Name: name})
	}
	return out
}

func applySyncPlan(cfg *config.Config, plan *providerSyncPlan) bool {
	if cfg == nil || plan == nil || len(plan.targets) == 0 {
		return false
	}

	changed := false
	for _, target := range plan.targets {
		switch target.kind {
		case "codex":
			if appendToMatchingCodexEntries(cfg.CodexKey, target) {
				changed = true
			}
		case "codex-compat":
			if appendToMatchingCodexEntries(cfg.CodexCompatKey, target) {
				changed = true
			}
		case "copilot-compat":
			if appendToMatchingCodexEntries(cfg.CopilotCompatKey, target) {
				changed = true
			}
		}
	}
	return changed
}

func appendToMatchingCodexEntries(entries []config.CodexKey, target codexSyncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameCodexEntry(*entry, target) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingCodexModels(entry.Models, target.modelsToAdd)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func appendMissingCodexModels(existing []config.CodexModel, additions []config.CodexModel) []config.CodexModel {
	if len(additions) == 0 {
		return existing
	}

	seen := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		name := strings.ToLower(strings.TrimSpace(model.Name))
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	out := existing
	for _, model := range additions {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model)
	}
	return out
}

func sameCodexEntry(entry config.CodexKey, target codexSyncTarget) bool {
	if strings.TrimSpace(entry.APIKey) != target.apiKey {
		return false
	}
	if strings.TrimSpace(entry.BaseURL) != target.baseURL {
		return false
	}
	if strings.TrimSpace(entry.ProxyURL) != target.proxyURL {
		return false
	}
	return headersEqual(entry.Headers, target.headers)
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}

func headersEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
