package executor

import (
	"strings"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v6/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestSanitizeCopilotCompatResponsesPayload_PreservesPreviousResponseID(t *testing.T) {
	input := []byte(`{"model":"gpt-5.2","previous_response_id":"resp_prev","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	out := sanitizeCopilotCompatResponsesPayload(input, "gpt-5.2", false)
	if got := gjson.GetBytes(out, "previous_response_id").String(); got != "resp_prev" {
		t.Fatalf("previous_response_id = %q, want %q", got, "resp_prev")
	}
	if got := gjson.GetBytes(out, "reasoning.summary").String(); got != "auto" {
		t.Fatalf("reasoning.summary = %q, want %q", got, "auto")
	}
	include := gjson.GetBytes(out, "include")
	if !include.IsArray() || len(include.Array()) == 0 || include.Array()[0].String() != "reasoning.encrypted_content" {
		t.Fatalf("include missing reasoning.encrypted_content: %s", out)
	}
	if !gjson.GetBytes(out, "store").Exists() || !gjson.GetBytes(out, "store").Bool() {
		t.Fatalf("store should default to true: %s", out)
	}
}

func TestSanitizeCopilotCompatChatPayload_RestoresReasoningOpaque(t *testing.T) {
	input := []byte(`{
		"messages":[
			{
				"role":"assistant",
				"content":[
					{"type":"reasoning","text":"plan","reasoning_opaque":"opaque-token"},
					{"type":"text","text":"done"}
				]
			}
		]
	}`)
	out := sanitizeCopilotCompatChatPayload(input, "gpt-5.2")
	if got := gjson.GetBytes(out, "messages.0.reasoning_opaque").String(); got != "opaque-token" {
		t.Fatalf("messages.0.reasoning_opaque = %q, want %q", got, "opaque-token")
	}
	if got := gjson.GetBytes(out, "messages.0.reasoning_text").String(); got != "plan" {
		t.Fatalf("messages.0.reasoning_text = %q, want %q", got, "plan")
	}
	if gjson.GetBytes(out, "messages.0.content.0.type").String() == "reasoning" {
		t.Fatalf("reasoning part should be removed from chat content: %s", out)
	}
}

func TestSanitizeCopilotCompatResponsesPayload_RestoresReasoningItem(t *testing.T) {
	input := []byte(`{
		"model":"gpt-5.2",
		"input":[
			{
				"type":"message",
				"role":"assistant",
				"content":[
					{"type":"reasoning","text":"think","item_id":"rs_1","reasoning_opaque":"enc_1"}
				]
			}
		]
	}`)
	out := sanitizeCopilotCompatResponsesPayload(input, "gpt-5.2", false)
	if got := gjson.GetBytes(out, "input.0.content.0.type").String(); got != "reasoning" {
		t.Fatalf("input.0.content.0.type = %q, want reasoning", got)
	}
	if got := gjson.GetBytes(out, "input.0.content.0.id").String(); got != "rs_1" {
		t.Fatalf("input.0.content.0.id = %q, want %q", got, "rs_1")
	}
	if got := gjson.GetBytes(out, "input.0.content.0.encrypted_content").String(); got != "enc_1" {
		t.Fatalf("input.0.content.0.encrypted_content = %q, want %q", got, "enc_1")
	}
	if got := gjson.GetBytes(out, "input.0.content.0.summary.0.text").String(); got != "think" {
		t.Fatalf("input.0.content.0.summary.0.text = %q, want %q", got, "think")
	}
}

func TestAnnotateCopilotCompatTranslatedJSON_AttachesProviderOptions(t *testing.T) {
	raw := `{"messages":[{"role":"assistant","reasoning_opaque":"opaque-1","content":[{"type":"reasoning","text":"r","item_id":"rs_1","encrypted_content":"enc_1"},{"type":"text","text":"hello"}]}]}`
	out := annotateCopilotCompatTranslatedJSON(raw)
	if got := gjson.Get(out, "messages.0.content.0.providerOptions.copilot.itemId").String(); got != "rs_1" {
		t.Fatalf("reasoning itemId = %q, want %q", got, "rs_1")
	}
	if got := gjson.Get(out, "messages.0.content.0.providerOptions.copilot.reasoningOpaque").String(); got != "enc_1" {
		t.Fatalf("reasoning opaque = %q, want %q", got, "enc_1")
	}
	if got := gjson.Get(out, "messages.0.content.1.providerOptions.copilot.reasoningOpaque").String(); got != "opaque-1" {
		t.Fatalf("text opaque = %q, want %q", got, "opaque-1")
	}
}

func TestSanitizeCopilotCompatResponsesPayload_ReadsProviderOptionsMetadata(t *testing.T) {
	input := []byte(`{
		"model":"gpt-5.2",
		"input":[
			{
				"type":"message",
				"role":"assistant",
				"content":[
					{
						"type":"reasoning",
						"providerOptions":{"copilot":{"itemId":"rs_meta","reasoningOpaque":"enc_meta"}},
						"text":"meta-think"
					}
				]
			}
		]
	}`)
	out := sanitizeCopilotCompatResponsesPayload(input, "gpt-5.2", false)
	if got := gjson.GetBytes(out, "input.0.content.0.id").String(); got != "rs_meta" {
		t.Fatalf("input.0.content.0.id = %q, want %q", got, "rs_meta")
	}
	if got := gjson.GetBytes(out, "input.0.content.0.encrypted_content").String(); got != "enc_meta" {
		t.Fatalf("input.0.content.0.encrypted_content = %q, want %q", got, "enc_meta")
	}
	if gjson.GetBytes(out, "input.0.content.0.providerOptions").Exists() {
		t.Fatalf("providerOptions should be stripped from upstream payload: %s", out)
	}
}

func TestSanitizeCopilotCompatChatPayload_MapsResponsesReasoningEffort(t *testing.T) {
	input := []byte(`{"model":"claude-sonnet-4-5","reasoning":{"effort":"high"},"messages":[{"role":"user","content":"hi"}]}`)
	out := sanitizeCopilotCompatChatPayload(input, "claude-sonnet-4-5")
	if got := gjson.GetBytes(out, "reasoning_effort").String(); got != "high" {
		t.Fatalf("reasoning_effort = %q, want %q", got, "high")
	}
	if gjson.GetBytes(out, "reasoning").Exists() {
		t.Fatalf("responses reasoning object should be removed from chat payload: %s", out)
	}
}

func TestAnnotateCopilotCompatTranslatedJSON_AttachesChoiceMessageMetadata(t *testing.T) {
	raw := `{"choices":[{"message":{"role":"assistant","reasoning_opaque":"opaque-choice","content":"hello"}}]}`
	out := annotateCopilotCompatTranslatedJSON(raw)
	if got := gjson.Get(out, "choices.0.message.providerOptions.copilot.reasoningOpaque").String(); got != "opaque-choice" {
		t.Fatalf("choice message reasoning opaque = %q, want %q", got, "opaque-choice")
	}
}

func TestIsCopilotResponsesFinishedPayload(t *testing.T) {
	for _, raw := range []string{
		`{"type":"response.completed"}`,
		`{"type":"response.incomplete"}`,
		`{"type":"response.done"}`,
	} {
		if !isCopilotResponsesFinishedPayload([]byte(raw)) {
			t.Fatalf("expected finished payload for %s", raw)
		}
	}
	if isCopilotResponsesFinishedPayload([]byte(`{"type":"response.in_progress"}`)) {
		t.Fatal("unexpected finished payload for in_progress")
	}
}

func TestCopilotResponsesTerminalError(t *testing.T) {
	err := copilotResponsesTerminalError([]byte(`{"type":"error","message":"bad stream"}`))
	if err == nil || !strings.Contains(err.Error(), "bad stream") {
		t.Fatalf("expected terminal error, got %v", err)
	}
}

func TestIsCopilotResponsesTerminalPayload(t *testing.T) {
	if !isCopilotResponsesTerminalPayload([]byte(`{"choices":[{"finish_reason":"stop"}]}`)) {
		t.Fatal("expected finish_reason chunk to be terminal")
	}
	if isCopilotResponsesTerminalPayload([]byte(`{"choices":[{"delta":{"content":"hi"}}]}`)) {
		t.Fatal("unexpected terminal payload for delta-only chunk")
	}
}

func TestCopilotChatTerminalError(t *testing.T) {
	err := copilotChatTerminalError([]byte(`{"error":{"message":"chat failed"}}`))
	if err == nil || !strings.Contains(err.Error(), "chat failed") {
		t.Fatalf("expected chat terminal error, got %v", err)
	}
}

func TestShouldUseResponsesAPI_DisablesResponsesForCodexSource(t *testing.T) {
	executor := &CopilotCompatExecutor{}
	if executor.shouldUseResponsesAPI("gpt-5.4", cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatCodex}) {
		t.Fatal("codex source should not use responses api")
	}
	if executor.shouldUseResponsesAPI("gpt-5.4", cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatCodexCompat}) {
		t.Fatal("codex-compat source should not use responses api")
	}
	if !executor.shouldUseResponsesAPI("gpt-5.4", cliproxyexecutor.Options{SourceFormat: sdktranslator.FormatOpenAIResponse}) {
		t.Fatal("responses source should continue using responses api")
	}
}

func TestCopilotCompatFallbackModels_IncludesOpenAIClaudeGemini(t *testing.T) {
	models := CopilotCompatFallbackModels()
	if len(models) == 0 {
		t.Fatal("expected fallback models")
	}
	var hasGPT, hasClaude, hasGemini bool
	for _, model := range models {
		if model == nil {
			continue
		}
		switch {
		case !hasGPT && strings.HasPrefix(model.ID, "gpt-"):
			hasGPT = true
		case !hasClaude && strings.HasPrefix(model.ID, "claude-"):
			hasClaude = true
		case !hasGemini && strings.HasPrefix(model.ID, "gemini-"):
			hasGemini = true
		}
	}
	if !hasGPT || !hasClaude || !hasGemini {
		t.Fatalf("expected GPT/Claude/Gemini fallback coverage, got gpt=%t claude=%t gemini=%t", hasGPT, hasClaude, hasGemini)
	}
}
