package executor

import "testing"

func TestParseOpenAIUsageChatCompletions(t *testing.T) {
	data := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":5}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 1 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 1)
	}
	if detail.OutputTokens != 2 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 2)
	}
	if detail.TotalTokens != 3 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 3)
	}
	if detail.CachedTokens != 4 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 4)
	}
	if detail.ReasoningTokens != 5 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 5)
	}
}

func TestParseOpenAIUsageResponses(t *testing.T) {
	data := []byte(`{"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30,"input_tokens_details":{"cached_tokens":7},"output_tokens_details":{"reasoning_tokens":9}}}`)
	detail := parseOpenAIUsage(data)
	if detail.InputTokens != 10 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 10)
	}
	if detail.OutputTokens != 20 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 20)
	}
	if detail.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 30)
	}
	if detail.CachedTokens != 7 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 7)
	}
	if detail.ReasoningTokens != 9 {
		t.Fatalf("reasoning tokens = %d, want %d", detail.ReasoningTokens, 9)
	}
}

func TestParseClaudeStreamUsageReadsMessageUsage(t *testing.T) {
	line := []byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":123,"output_tokens":0,"cache_creation_input_tokens":45}}}`)
	detail, ok := parseClaudeStreamUsage(line)
	if !ok {
		t.Fatal("parseClaudeStreamUsage() ok = false, want true")
	}
	if detail.InputTokens != 168 {
		t.Fatalf("input tokens = %d, want %d", detail.InputTokens, 168)
	}
	if detail.OutputTokens != 0 {
		t.Fatalf("output tokens = %d, want %d", detail.OutputTokens, 0)
	}
	if detail.CachedTokens != 45 {
		t.Fatalf("cached tokens = %d, want %d", detail.CachedTokens, 45)
	}
	if detail.TotalTokens != 168 {
		t.Fatalf("total tokens = %d, want %d", detail.TotalTokens, 168)
	}
}

func TestMergeUsageDetailsCombinesClaudeStreamEvents(t *testing.T) {
	lines := [][]byte{
		[]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":321,"cache_creation_input_tokens":12}}}`),
		[]byte(`data: {"type":"message_delta","usage":{"output_tokens":654,"cache_read_input_tokens":77}}`),
	}

	var detail claudeUsageFields
	for _, line := range lines {
		parsed, ok := parseClaudeStreamUsageFields(line)
		if !ok {
			t.Fatalf("parseClaudeStreamUsageFields(%q) ok = false, want true", string(line))
		}
		detail = mergeClaudeUsageFields(detail, parsed)
	}
	merged := detail.detail()

	if merged.InputTokens != 333 {
		t.Fatalf("input tokens = %d, want %d", merged.InputTokens, 333)
	}
	if merged.OutputTokens != 654 {
		t.Fatalf("output tokens = %d, want %d", merged.OutputTokens, 654)
	}
	if merged.CachedTokens != 77 {
		t.Fatalf("cached tokens = %d, want %d", merged.CachedTokens, 77)
	}
	if merged.TotalTokens != 987 {
		t.Fatalf("total tokens = %d, want %d", merged.TotalTokens, 987)
	}
}

func TestMergeClaudeUsageFieldsPrefersCacheReadOverCreation(t *testing.T) {
	lines := [][]byte{
		[]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":3,"cache_creation_input_tokens":74122}}}`),
		[]byte(`data: {"type":"message_delta","usage":{"output_tokens":333,"cache_read_input_tokens":53359}}`),
	}

	var detail claudeUsageFields
	for _, line := range lines {
		parsed, ok := parseClaudeStreamUsageFields(line)
		if !ok {
			t.Fatalf("parseClaudeStreamUsageFields(%q) ok = false, want true", string(line))
		}
		detail = mergeClaudeUsageFields(detail, parsed)
	}
	merged := detail.detail()

	if merged.InputTokens != 74125 {
		t.Fatalf("input tokens = %d, want %d", merged.InputTokens, 74125)
	}
	if merged.OutputTokens != 333 {
		t.Fatalf("output tokens = %d, want %d", merged.OutputTokens, 333)
	}
	if merged.CachedTokens != 53359 {
		t.Fatalf("cached tokens = %d, want %d", merged.CachedTokens, 53359)
	}
	if merged.TotalTokens != 74458 {
		t.Fatalf("total tokens = %d, want %d", merged.TotalTokens, 74458)
	}
}
