package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const (
	codexModelsDefaultEndpoint = "https://api.openai.com/v1/models"
	codexModelsDevEndpoint     = "https://models.dev/api.json"
	codexModelsCacheTTL        = 2 * time.Minute
)

var codexModelsDevAPIEndpoint = codexModelsDevEndpoint

type codexModelsCacheEntry struct {
	fingerprint string
	expiresAt   time.Time
	models      []*registry.ModelInfo
}

var codexModelsCache sync.Map // key: auth id/fingerprint, value: codexModelsCacheEntry

// FetchCodexModels retrieves model definitions from the upstream models endpoint.
// It returns nil when fetching fails so callers can fallback to static model lists.
func FetchCodexModels(ctx context.Context, auth *cliproxyauth.Auth, cfg *config.Config) []*registry.ModelInfo {
	if auth == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	apiKey, baseURL := codexCreds(auth)
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil
	}

	fingerprint := codexModelsFingerprint(apiKey, baseURL, auth.ProxyURL)
	cacheKey := strings.TrimSpace(auth.ID)
	if cacheKey == "" {
		cacheKey = fingerprint
	}

	if cached := getCachedCodexModels(cacheKey, fingerprint); len(cached) > 0 {
		return cached
	}

	modelsURL := normalizeCodexModelsEndpoint(baseURL)
	httpClient := newProxyAwareHTTPClient(ctx, cfg, auth, 15*time.Second)
	models := fetchCodexModelsFromOpenAI(ctx, httpClient, auth, apiKey, modelsURL)
	if len(models) == 0 {
		models = fetchCodexModelsFromModelsDev(ctx, httpClient)
	}
	if len(models) == 0 {
		log.Debugf("codex executor: all dynamic model sources returned empty model list")
		return nil
	}

	codexModelsCache.Store(cacheKey, codexModelsCacheEntry{
		fingerprint: fingerprint,
		expiresAt:   time.Now().Add(codexModelsCacheTTL),
		models:      cloneModelInfos(models),
	})
	return models
}

func fetchCodexModelsFromOpenAI(ctx context.Context, httpClient *http.Client, auth *cliproxyauth.Auth, apiKey, modelsURL string) []*registry.ModelInfo {
	httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if errReq != nil {
		log.Debugf("codex executor: failed to build models request: %v", errReq)
		return nil
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", codexUserAgent)
	util.ApplyCustomHeadersFromAttrs(httpReq, auth.Attributes)

	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		log.Debugf("codex executor: models request failed: %v", errDo)
		return nil
	}
	bodyBytes, errRead := io.ReadAll(httpResp.Body)
	if errClose := httpResp.Body.Close(); errClose != nil {
		log.Errorf("codex executor: close models response body error: %v", errClose)
	}
	if errRead != nil {
		log.Debugf("codex executor: models response read failed: %v", errRead)
		return nil
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		log.Debugf("codex executor: models endpoint returned status %d", httpResp.StatusCode)
		return nil
	}

	return parseCodexModelsResponse(bodyBytes)
}

func fetchCodexModelsFromModelsDev(ctx context.Context, httpClient *http.Client) []*registry.ModelInfo {
	httpReq, errReq := http.NewRequestWithContext(ctx, http.MethodGet, codexModelsDevAPIEndpoint, nil)
	if errReq != nil {
		log.Debugf("codex executor: failed to build models.dev request: %v", errReq)
		return nil
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", codexUserAgent)

	httpResp, errDo := httpClient.Do(httpReq)
	if errDo != nil {
		log.Debugf("codex executor: models.dev request failed: %v", errDo)
		return nil
	}
	bodyBytes, errRead := io.ReadAll(httpResp.Body)
	if errClose := httpResp.Body.Close(); errClose != nil {
		log.Errorf("codex executor: close models.dev response body error: %v", errClose)
	}
	if errRead != nil {
		log.Debugf("codex executor: models.dev response read failed: %v", errRead)
		return nil
	}
	if httpResp.StatusCode < http.StatusOK || httpResp.StatusCode >= http.StatusMultipleChoices {
		log.Debugf("codex executor: models.dev endpoint returned status %d", httpResp.StatusCode)
		return nil
	}

	return parseCodexModelsDevResponse(bodyBytes)
}

func normalizeCodexModelsEndpoint(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return codexModelsDefaultEndpoint
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return codexModelsDefaultEndpoint
	}

	path := strings.TrimSuffix(parsed.Path, "/")
	lowerPath := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowerPath, "/models"):
		return parsed.String()
	case strings.Contains(lowerPath, "/backend-api/codex"):
		return codexModelsDefaultEndpoint
	case strings.HasSuffix(lowerPath, "/v1"):
		parsed.Path = path + "/models"
	case path == "":
		parsed.Path = "/v1/models"
	default:
		parsed.Path = path + "/models"
	}
	return parsed.String()
}

func parseCodexModelsResponse(body []byte) []*registry.ModelInfo {
	data := gjson.GetBytes(body, "data")
	if !data.Exists() || !data.IsArray() {
		return nil
	}

	now := time.Now().Unix()
	models := make([]*registry.ModelInfo, 0, len(data.Array()))
	for _, item := range data.Array() {
		modelID := strings.TrimSpace(item.Get("id").String())
		if modelID == "" {
			continue
		}

		model := &registry.ModelInfo{
			ID:          modelID,
			Object:      "model",
			Created:     item.Get("created").Int(),
			OwnedBy:     strings.TrimSpace(item.Get("owned_by").String()),
			Type:        "openai",
			DisplayName: modelID,
			Name:        strings.TrimSpace(item.Get("name").String()),
			Version:     strings.TrimSpace(item.Get("version").String()),
		}
		if object := strings.TrimSpace(item.Get("object").String()); object != "" {
			model.Object = object
		}
		if model.Created <= 0 {
			model.Created = now
		}
		if model.OwnedBy == "" {
			model.OwnedBy = "openai"
		}

		if staticInfo := registry.LookupStaticModelInfo(modelID); staticInfo != nil {
			merged := *staticInfo
			merged.ID = model.ID
			merged.Object = model.Object
			merged.Created = model.Created
			merged.OwnedBy = model.OwnedBy
			if model.Type != "" {
				merged.Type = model.Type
			}
			if model.Name != "" {
				merged.Name = model.Name
			}
			if model.Version != "" {
				merged.Version = model.Version
			}
			model = &merged
		}

		models = append(models, model)
	}
	return models
}

func parseCodexModelsDevResponse(body []byte) []*registry.ModelInfo {
	root := gjson.ParseBytes(body)
	if !root.Exists() || !root.IsObject() {
		return nil
	}

	now := time.Now().Unix()
	modelMap := make(map[string]*registry.ModelInfo)
	root.ForEach(func(providerID, providerValue gjson.Result) bool {
		modelsNode := providerValue.Get("models")
		if !modelsNode.Exists() || !modelsNode.IsObject() {
			return true
		}

		provider := strings.ToLower(strings.TrimSpace(providerID.String()))
		modelsNode.ForEach(func(modelKey, modelValue gjson.Result) bool {
			rawID := strings.TrimSpace(modelKey.String())
			modelID, ok := normalizeModelsDevModelID(provider, rawID)
			if !ok || strings.TrimSpace(modelID) == "" {
				return true
			}
			if _, exists := modelMap[modelID]; exists {
				return true
			}

			model := &registry.ModelInfo{
				ID:          modelID,
				Object:      "model",
				Created:     now,
				OwnedBy:     "openai",
				Type:        "openai",
				DisplayName: modelID,
				Name:        strings.TrimSpace(modelValue.Get("name").String()),
				Version:     strings.TrimSpace(modelValue.Get("version").String()),
			}

			if staticInfo := registry.LookupStaticModelInfo(modelID); staticInfo != nil {
				merged := *staticInfo
				merged.ID = model.ID
				merged.Object = model.Object
				merged.Created = model.Created
				merged.OwnedBy = model.OwnedBy
				if model.Type != "" {
					merged.Type = model.Type
				}
				if model.Name != "" {
					merged.Name = model.Name
				}
				if model.Version != "" {
					merged.Version = model.Version
				}
				model = &merged
			}

			modelMap[modelID] = model
			return true
		})
		return true
	})

	if len(modelMap) == 0 {
		return nil
	}

	ids := make([]string, 0, len(modelMap))
	for modelID := range modelMap {
		ids = append(ids, modelID)
	}
	sort.Strings(ids)

	models := make([]*registry.ModelInfo, 0, len(ids))
	for _, modelID := range ids {
		models = append(models, modelMap[modelID])
	}
	return models
}

func normalizeModelsDevModelID(provider, rawID string) (string, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	rawID = strings.TrimSpace(rawID)
	if rawID == "" {
		return "", false
	}

	lowerID := strings.ToLower(rawID)
	if strings.HasPrefix(lowerID, "openai/") {
		modelID := strings.TrimSpace(rawID[len("openai/"):])
		if modelID == "" {
			return "", false
		}
		return modelID, true
	}
	if provider == "openai" && !strings.Contains(rawID, "/") {
		return rawID, true
	}
	return "", false
}

func getCachedCodexModels(cacheKey, fingerprint string) []*registry.ModelInfo {
	if cacheKey == "" {
		return nil
	}
	value, ok := codexModelsCache.Load(cacheKey)
	if !ok {
		return nil
	}
	entry, ok := value.(codexModelsCacheEntry)
	if !ok {
		return nil
	}
	if entry.fingerprint != fingerprint || time.Now().After(entry.expiresAt) {
		return nil
	}
	return cloneModelInfos(entry.models)
}

func cloneModelInfos(models []*registry.ModelInfo) []*registry.ModelInfo {
	if len(models) == 0 {
		return nil
	}
	cloned := make([]*registry.ModelInfo, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		copyModel := *model
		if model.Thinking != nil {
			thinkingCopy := *model.Thinking
			if len(model.Thinking.Levels) > 0 {
				thinkingCopy.Levels = append([]string(nil), model.Thinking.Levels...)
			}
			copyModel.Thinking = &thinkingCopy
		}
		if len(model.SupportedGenerationMethods) > 0 {
			copyModel.SupportedGenerationMethods = append([]string(nil), model.SupportedGenerationMethods...)
		}
		if len(model.SupportedParameters) > 0 {
			copyModel.SupportedParameters = append([]string(nil), model.SupportedParameters...)
		}
		cloned = append(cloned, &copyModel)
	}
	return cloned
}

func codexModelsFingerprint(apiKey, baseURL, proxyURL string) string {
	payload := strings.TrimSpace(apiKey) + "|" + strings.TrimSpace(baseURL) + "|" + strings.TrimSpace(proxyURL)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
