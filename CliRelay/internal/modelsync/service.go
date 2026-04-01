package modelsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
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
	targets []syncTarget
}

type syncTarget struct {
	kind       string
	match      entryMatcher
	codex      []config.CodexModel
	gemini     []config.GeminiModel
	claude     []config.ClaudeModel
	vertex     []config.VertexCompatModel
	openai     []config.OpenAICompatibilityModel
}

type entryMatcher struct {
	apiKey   string
	baseURL  string
	proxyURL string
	name     string
	headers  map[string]string
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

	targets := make([]syncTarget, 0)
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
	for _, entry := range cfg.GeminiKey {
		target, err := buildGeminiTarget(ctx, entry)
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	for _, entry := range cfg.ClaudeKey {
		target, err := buildClaudeTarget(ctx, entry)
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	for _, entry := range cfg.VertexCompatAPIKey {
		target, err := buildVertexTarget(ctx, entry)
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	for _, entry := range cfg.OpenAICompatibility {
		target, err := buildOpenAICompatTarget(ctx, entry)
		if err != nil {
			return nil, err
		}
		if target != nil {
			targets = append(targets, *target)
		}
	}
	return &providerSyncPlan{targets: targets}, nil
}

func buildCodexTarget(ctx context.Context, entry config.CodexKey, provider string) (*syncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}

	models, err := fetchOpenAICompatibleModels(ctx, buildAPIKeyAuth(provider, provider, entry.APIKey, entry.BaseURL, entry.ProxyURL, entry.Headers))
	if err != nil || len(models) == 0 {
		return nil, err
	}

	modelsToAdd := collectMissingCodexModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &syncTarget{
		kind: provider,
		match: entryMatcher{
			apiKey:   strings.TrimSpace(entry.APIKey),
			baseURL:  strings.TrimSpace(entry.BaseURL),
			proxyURL: strings.TrimSpace(entry.ProxyURL),
			headers:  cloneHeaders(entry.Headers),
		},
		codex: modelsToAdd,
	}, nil
}

func buildGeminiTarget(ctx context.Context, entry config.GeminiKey) (*syncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}

	models, err := fetchOpenAICompatibleModels(ctx, buildAPIKeyAuth("gemini", "gemini", entry.APIKey, entry.BaseURL, entry.ProxyURL, entry.Headers))
	if err != nil || len(models) == 0 {
		return nil, err
	}

	modelsToAdd := collectMissingGeminiModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &syncTarget{
		kind: "gemini",
		match: entryMatcher{
			apiKey:   strings.TrimSpace(entry.APIKey),
			baseURL:  strings.TrimSpace(entry.BaseURL),
			proxyURL: strings.TrimSpace(entry.ProxyURL),
			headers:  cloneHeaders(entry.Headers),
		},
		gemini: modelsToAdd,
	}, nil
}

func buildClaudeTarget(ctx context.Context, entry config.ClaudeKey) (*syncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}

	models, err := fetchOpenAICompatibleModels(ctx, buildAPIKeyAuth("claude", "claude", entry.APIKey, entry.BaseURL, entry.ProxyURL, entry.Headers))
	if err != nil || len(models) == 0 {
		return nil, err
	}

	modelsToAdd := collectMissingClaudeModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &syncTarget{
		kind: "claude",
		match: entryMatcher{
			apiKey:   strings.TrimSpace(entry.APIKey),
			baseURL:  strings.TrimSpace(entry.BaseURL),
			proxyURL: strings.TrimSpace(entry.ProxyURL),
			headers:  cloneHeaders(entry.Headers),
		},
		claude: modelsToAdd,
	}, nil
}

func buildVertexTarget(ctx context.Context, entry config.VertexCompatKey) (*syncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}

	models, err := fetchOpenAICompatibleModels(ctx, buildAPIKeyAuth("vertex", "vertex", entry.APIKey, entry.BaseURL, entry.ProxyURL, entry.Headers))
	if err != nil || len(models) == 0 {
		return nil, err
	}

	modelsToAdd := collectMissingVertexModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &syncTarget{
		kind: "vertex",
		match: entryMatcher{
			apiKey:   strings.TrimSpace(entry.APIKey),
			baseURL:  strings.TrimSpace(entry.BaseURL),
			proxyURL: strings.TrimSpace(entry.ProxyURL),
			headers:  cloneHeaders(entry.Headers),
		},
		vertex: modelsToAdd,
	}, nil
}

func buildOpenAICompatTarget(ctx context.Context, entry config.OpenAICompatibility) (*syncTarget, error) {
	if !entry.AutoSyncModels {
		return nil, nil
	}
	if len(entry.APIKeyEntries) == 0 {
		return nil, nil
	}

	var models []*registry.ModelInfo
	var err error
	for i := range entry.APIKeyEntries {
		apiKeyEntry := entry.APIKeyEntries[i]
		models, err = fetchOpenAICompatibleModels(ctx, buildAPIKeyAuth(
			"openai-compatibility",
			entry.Name,
			apiKeyEntry.APIKey,
			entry.BaseURL,
			apiKeyEntry.ProxyURL,
			mergeHeaders(entry.Headers, apiKeyEntry.Headers),
		))
		if len(models) > 0 {
			break
		}
	}
	if err != nil || len(models) == 0 {
		return nil, err
	}

	modelsToAdd := collectMissingOpenAICompatModels(entry.Models, models)
	if len(modelsToAdd) == 0 {
		return nil, nil
	}

	return &syncTarget{
		kind: "openai-compatibility",
		match: entryMatcher{
			name:    strings.TrimSpace(entry.Name),
			baseURL: strings.TrimSpace(entry.BaseURL),
		},
		openai: modelsToAdd,
	}, nil
}

func fetchOpenAICompatibleModels(ctx context.Context, auth *coreauth.Auth) ([]*registry.ModelInfo, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if auth == nil {
		return nil, nil
	}
	apiKey := strings.TrimSpace(auth.Attributes["api_key"])
	baseURL := strings.TrimSpace(auth.Attributes["base_url"])
	if apiKey == "" {
		return nil, nil
	}
	modelsEndpoint, ok := normalizeModelsEndpoint(baseURL)
	if !ok {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	for key, value := range auth.Attributes {
		if key == "" || value == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "api_key", "base_url", "provider_key", "compat_name":
			continue
		default:
			req.Header.Set(key, value)
		}
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	if transport := proxyTransport(auth.ProxyURL); transport != nil {
		client.Transport = transport
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		log.WithError(closeErr).Debug("model sync: failed to close upstream response body")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil
	}

	models, err := parseModelsResponse(body)
	if err != nil || len(models) == 0 {
		return nil, err
	}
	return models, nil
}

func proxyTransport(proxyURL string) *http.Transport {
	trimmed := strings.TrimSpace(proxyURL)
	if trimmed == "" {
		return nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil
	}
	return &http.Transport{Proxy: http.ProxyURL(parsed)}
}

func normalizeModelsEndpoint(baseURL string) (string, bool) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, "/models"):
		return parsed.String(), true
	case strings.HasSuffix(lowerPath, "/v1"):
		parsed.Path = path + "/models"
	case path == "":
		parsed.Path = "/v1/models"
	default:
		parsed.Path = path + "/models"
	}
	return parsed.String(), true
}

func parseModelsResponse(body []byte) ([]*registry.ModelInfo, error) {
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	out := make([]*registry.ModelInfo, 0, len(payload.Data))
	for _, item := range payload.Data {
		name := strings.TrimSpace(item.ID)
		if name == "" {
			continue
		}
		out = append(out, &registry.ModelInfo{ID: name, Name: name})
	}
	return out, nil
}

func buildAPIKeyAuth(providerKey, providerName, apiKey, baseURL, proxyURL string, headers map[string]string) *coreauth.Auth {
	apiKey = strings.TrimSpace(apiKey)
	baseURL = strings.TrimSpace(baseURL)
	proxyURL = strings.TrimSpace(proxyURL)
	attrs := map[string]string{
		"api_key":      apiKey,
		"base_url":     baseURL,
		"provider_key": strings.TrimSpace(providerKey),
	}
	if providerName != "" {
		attrs["compat_name"] = strings.TrimSpace(providerName)
	}
	for key, value := range headers {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		attrs[trimmedKey] = trimmedValue
	}

	sum := sha256.Sum256([]byte(providerKey + "|" + providerName + "|" + apiKey + "|" + baseURL))
	return &coreauth.Auth{
		ID:         "model-sync:" + hex.EncodeToString(sum[:8]),
		Provider:   providerKey,
		ProxyURL:   proxyURL,
		Attributes: attrs,
	}
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
		case "gemini":
			if appendToMatchingGeminiEntries(cfg.GeminiKey, target) {
				changed = true
			}
		case "claude":
			if appendToMatchingClaudeEntries(cfg.ClaudeKey, target) {
				changed = true
			}
		case "vertex":
			if appendToMatchingVertexEntries(cfg.VertexCompatAPIKey, target) {
				changed = true
			}
		case "openai-compatibility":
			if appendToMatchingOpenAICompatEntries(cfg.OpenAICompatibility, target) {
				changed = true
			}
		}
	}
	return changed
}

func appendToMatchingCodexEntries(entries []config.CodexKey, target syncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameEntry(target.match, strings.TrimSpace(entry.APIKey), strings.TrimSpace(entry.BaseURL), strings.TrimSpace(entry.ProxyURL), "", entry.Headers) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingCodexModels(entry.Models, target.codex)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func appendToMatchingGeminiEntries(entries []config.GeminiKey, target syncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameEntry(target.match, strings.TrimSpace(entry.APIKey), strings.TrimSpace(entry.BaseURL), strings.TrimSpace(entry.ProxyURL), "", entry.Headers) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingGeminiModels(entry.Models, target.gemini)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func appendToMatchingClaudeEntries(entries []config.ClaudeKey, target syncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameEntry(target.match, strings.TrimSpace(entry.APIKey), strings.TrimSpace(entry.BaseURL), strings.TrimSpace(entry.ProxyURL), "", entry.Headers) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingClaudeModels(entry.Models, target.claude)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func appendToMatchingVertexEntries(entries []config.VertexCompatKey, target syncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameEntry(target.match, strings.TrimSpace(entry.APIKey), strings.TrimSpace(entry.BaseURL), strings.TrimSpace(entry.ProxyURL), "", entry.Headers) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingVertexModels(entry.Models, target.vertex)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func appendToMatchingOpenAICompatEntries(entries []config.OpenAICompatibility, target syncTarget) bool {
	changed := false
	for i := range entries {
		entry := &entries[i]
		if !sameEntry(target.match, "", strings.TrimSpace(entry.BaseURL), "", strings.TrimSpace(entry.Name), nil) {
			continue
		}
		before := len(entry.Models)
		entry.Models = appendMissingOpenAICompatModels(entry.Models, target.openai)
		if len(entry.Models) != before {
			changed = true
		}
	}
	return changed
}

func sameEntry(match entryMatcher, apiKey, baseURL, proxyURL, name string, headers map[string]string) bool {
	if match.apiKey != "" && apiKey != match.apiKey {
		return false
	}
	if match.baseURL != baseURL {
		return false
	}
	if match.proxyURL != proxyURL {
		return false
	}
	if match.name != "" && name != match.name {
		return false
	}
	if match.headers != nil && !headersEqual(headers, match.headers) {
		return false
	}
	return true
}

func collectMissingCodexModels(existing []config.CodexModel, models []*registry.ModelInfo) []config.CodexModel {
	return collectMissingModels(existing, models, func(name string) config.CodexModel { return config.CodexModel{Name: name} })
}

func collectMissingGeminiModels(existing []config.GeminiModel, models []*registry.ModelInfo) []config.GeminiModel {
	return collectMissingModels(existing, models, func(name string) config.GeminiModel { return config.GeminiModel{Name: name} })
}

func collectMissingClaudeModels(existing []config.ClaudeModel, models []*registry.ModelInfo) []config.ClaudeModel {
	return collectMissingModels(existing, models, func(name string) config.ClaudeModel { return config.ClaudeModel{Name: name} })
}

func collectMissingVertexModels(existing []config.VertexCompatModel, models []*registry.ModelInfo) []config.VertexCompatModel {
	return collectMissingModels(existing, models, func(name string) config.VertexCompatModel {
		return config.VertexCompatModel{Name: name, Alias: name}
	})
}

func collectMissingOpenAICompatModels(existing []config.OpenAICompatibilityModel, models []*registry.ModelInfo) []config.OpenAICompatibilityModel {
	return collectMissingModels(existing, models, func(name string) config.OpenAICompatibilityModel { return config.OpenAICompatibilityModel{Name: name} })
}

func collectMissingModels[T interface{ GetName() string }](existing []T, models []*registry.ModelInfo, makeModel func(string) T) []T {
	if len(models) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		name := strings.ToLower(strings.TrimSpace(model.GetName()))
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	out := make([]T, 0)
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
		out = append(out, makeModel(name))
	}
	return out
}

func appendMissingCodexModels(existing []config.CodexModel, additions []config.CodexModel) []config.CodexModel {
	return appendMissingModels(existing, additions, func(model config.CodexModel) string { return model.Name })
}

func appendMissingGeminiModels(existing []config.GeminiModel, additions []config.GeminiModel) []config.GeminiModel {
	return appendMissingModels(existing, additions, func(model config.GeminiModel) string { return model.Name })
}

func appendMissingClaudeModels(existing []config.ClaudeModel, additions []config.ClaudeModel) []config.ClaudeModel {
	return appendMissingModels(existing, additions, func(model config.ClaudeModel) string { return model.Name })
}

func appendMissingVertexModels(existing []config.VertexCompatModel, additions []config.VertexCompatModel) []config.VertexCompatModel {
	return appendMissingModels(existing, additions, func(model config.VertexCompatModel) string { return model.Name })
}

func appendMissingOpenAICompatModels(existing []config.OpenAICompatibilityModel, additions []config.OpenAICompatibilityModel) []config.OpenAICompatibilityModel {
	return appendMissingModels(existing, additions, func(model config.OpenAICompatibilityModel) string { return model.Name })
}

func appendMissingModels[T any](existing []T, additions []T, nameFn func(T) string) []T {
	if len(additions) == 0 {
		return existing
	}

	seen := make(map[string]struct{}, len(existing))
	for _, model := range existing {
		name := strings.ToLower(strings.TrimSpace(nameFn(model)))
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}

	out := existing
	for _, model := range additions {
		name := strings.TrimSpace(nameFn(model))
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

func mergeHeaders(left, right map[string]string) map[string]string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	out := cloneHeaders(left)
	if out == nil {
		out = make(map[string]string, len(right))
	}
	for key, value := range right {
		out[key] = value
	}
	return out
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
