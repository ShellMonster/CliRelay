package thinking_test

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/thinking"
	_ "github.com/router-for-me/CLIProxyAPI/v6/internal/thinking/provider/codex"
	"github.com/tidwall/gjson"
)

func TestApplyThinking_OpenAIResponseUnsupportedModelStripsReasoningEffort(t *testing.T) {
	reg := registry.GetGlobalRegistry()
	clientID := "test-copilot-compat-thinking-strip"
	modelID := "test-openai-response-no-thinking"
	reg.RegisterClient(clientID, "copilot-compat", []*registry.ModelInfo{
		{ID: modelID},
	})
	t.Cleanup(func() {
		reg.UnregisterClient(clientID)
	})

	body := []byte(`{"model":"test-openai-response-no-thinking","reasoning":{"effort":"xhigh"},"input":[{"role":"user","content":"hi"}]}`)
	out, err := thinking.ApplyThinking(body, modelID, "openai-response", "openai-response", "copilot-compat")
	if err != nil {
		t.Fatalf("ApplyThinking error: %v", err)
	}
	if gjson.GetBytes(out, "reasoning.effort").Exists() {
		t.Fatalf("reasoning.effort should be stripped for unsupported models: %s", out)
	}
	if !gjson.GetBytes(out, "input").Exists() {
		t.Fatalf("expected request payload to remain intact: %s", out)
	}
}
