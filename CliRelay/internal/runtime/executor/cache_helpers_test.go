package executor

import (
	"context"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func resetCodexCompatibleCacheForTest() {
	codexCacheMu.Lock()
	defer codexCacheMu.Unlock()
	codexCacheMap = make(map[string]codexCache)
}

func TestPrepareCodexCompatiblePayload_OpenAIResponsesPromptCacheHeaders(t *testing.T) {
	rawJSON := []byte(`{"model":"gpt-5.4","input":[]}`)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","prompt_cache_key":"cache-openai"}`),
	}

	prepared, cacheID := prepareCodexCompatiblePayload(sdktranslator.FormatOpenAIResponse, req, rawJSON, false)
	if cacheID != "cache-openai" {
		t.Fatalf("cacheID = %q, want %q", cacheID, "cache-openai")
	}
	if gjson.GetBytes(prepared, "prompt_cache_key").Exists() {
		t.Fatalf("unexpected prompt_cache_key in body when allowPromptCache is false: %s", prepared)
	}

	httpReq, err := buildCodexCompatibleRequest(context.Background(), "https://example.com/responses", prepared, cacheID)
	if err != nil {
		t.Fatalf("buildCodexCompatibleRequest error: %v", err)
	}
	if got := httpReq.Header.Get("Conversation_id"); got != "cache-openai" {
		t.Fatalf("Conversation_id = %q, want %q", got, "cache-openai")
	}
	if got := httpReq.Header.Get("Session_id"); got != "cache-openai" {
		t.Fatalf("Session_id = %q, want %q", got, "cache-openai")
	}
}

func TestPrepareCodexCompatiblePayload_ClaudeReusesCacheAcrossModes(t *testing.T) {
	resetCodexCompatibleCacheForTest()
	t.Cleanup(resetCodexCompatibleCacheForTest)

	req := cliproxyexecutor.Request{
		Model:   "claude-sonnet-4-5",
		Payload: []byte(`{"metadata":{"user_id":"user-123"}}`),
	}

	firstBody, firstCacheID := prepareCodexCompatiblePayload(sdktranslator.FormatClaude, req, []byte(`{"messages":[]}`), true)
	if firstCacheID == "" {
		t.Fatal("expected Claude metadata.user_id to create a cache id")
	}
	if got := gjson.GetBytes(firstBody, "prompt_cache_key").String(); got != firstCacheID {
		t.Fatalf("prompt_cache_key = %q, want %q", got, firstCacheID)
	}

	secondBody, secondCacheID := prepareCodexCompatiblePayload(sdktranslator.FormatClaude, req, []byte(`{"messages":[]}`), false)
	if secondCacheID != firstCacheID {
		t.Fatalf("cache id should be reused, got %q want %q", secondCacheID, firstCacheID)
	}
	if gjson.GetBytes(secondBody, "prompt_cache_key").Exists() {
		t.Fatalf("unexpected prompt_cache_key in chat-mode body: %s", secondBody)
	}

	httpReq, err := buildCodexCompatibleRequest(context.Background(), "https://example.com/chat/completions", secondBody, secondCacheID)
	if err != nil {
		t.Fatalf("buildCodexCompatibleRequest error: %v", err)
	}
	if got := httpReq.Header.Get("Conversation_id"); got != firstCacheID {
		t.Fatalf("Conversation_id = %q, want %q", got, firstCacheID)
	}
	if got := httpReq.Header.Get("Session_id"); got != firstCacheID {
		t.Fatalf("Session_id = %q, want %q", got, firstCacheID)
	}
}
