package codexcompat

import (
	"context"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func parseCompatSSEJSON(t *testing.T, chunk string) gjson.Result {
	t.Helper()

	line := strings.TrimSpace(chunk)
	line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if !gjson.Valid(line) {
		t.Fatalf("invalid SSE JSON payload: %q", chunk)
	}
	return gjson.Parse(line)
}

func TestConvertCodexCompatResponseToOpenAIResponses_StabilizesResponseAndItemIDs(t *testing.T) {
	t.Helper()

	originalReq := []byte(`{"instructions":"be precise"}`)
	inputs := []string{
		`data: {"type":"response.created","response":{"id":"resp_initial","created_at":111,"model":"gpt-5.4","instructions":""}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_rotated","created_at":222,"model":"gpt-5.4"}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_initial","type":"message","role":"assistant"}}`,
		`data: {"type":"response.content_part.added","output_index":0,"item_id":"msg_rotated_1","part":{"type":"output_text","text":""}}`,
		`data: {"type":"response.output_text.delta","output_index":0,"item_id":"msg_rotated_2","delta":"Hel"}`,
		`data: {"type":"response.output_text.done","output_index":0,"item_id":"msg_rotated_3","text":"Hello"}`,
		`data: {"type":"response.content_part.done","output_index":0,"item_id":"msg_rotated_4","part":{"type":"output_text","text":"Hello"}}`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_rotated_5","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}}`,
		`data: {"type":"response.completed","response":{"id":"resp_completed","created_at":333,"model":"gpt-5.4","output":[{"id":"msg_completed","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}],"usage":{"input_tokens":1,"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":2}}}`,
	}

	var param any
	outputs := make([]gjson.Result, 0, len(inputs))
	for _, input := range inputs {
		chunks := ConvertCodexCompatResponseToOpenAIResponses(context.Background(), "gpt-5.4", originalReq, nil, []byte(input), &param)
		if len(chunks) != 1 {
			t.Fatalf("expected a single translated chunk, got %d for %q", len(chunks), input)
		}
		outputs = append(outputs, parseCompatSSEJSON(t, chunks[0]))
	}

	if got := outputs[0].Get("response.id").String(); got != "resp_initial" {
		t.Fatalf("unexpected response.created id: got %q", got)
	}
	if got := outputs[0].Get("response.instructions").String(); got != "be precise" {
		t.Fatalf("unexpected response.created instructions: got %q", got)
	}
	if got := outputs[1].Get("response.id").String(); got != "resp_initial" {
		t.Fatalf("unexpected response.in_progress id: got %q", got)
	}
	if got := outputs[1].Get("response.created_at").Int(); got != 111 {
		t.Fatalf("unexpected response.in_progress created_at: got %d", got)
	}

	expectedItemID := "msg_initial"
	for idx, path := range []string{
		"item.id",
		"item_id",
		"item_id",
		"item_id",
		"item_id",
		"item.id",
	} {
		got := outputs[idx+2].Get(path).String()
		if got != expectedItemID {
			t.Fatalf("unexpected stable item id at output[%d]: got %q want %q", idx+2, got, expectedItemID)
		}
	}

	completed := outputs[len(outputs)-1]
	if got := completed.Get("response.id").String(); got != "resp_initial" {
		t.Fatalf("unexpected response.completed id: got %q", got)
	}
	if got := completed.Get("response.output.0.id").String(); got != expectedItemID {
		t.Fatalf("unexpected response.completed output id: got %q want %q", got, expectedItemID)
	}
}

func TestConvertCodexCompatResponseToOpenAIResponsesNonStream_UsesStableStateForRawJSON(t *testing.T) {
	var param any

	ConvertCodexCompatResponseToOpenAIResponses(
		context.Background(),
		"gpt-5.4",
		nil,
		nil,
		[]byte(`data: {"type":"response.created","response":{"id":"resp_initial","created_at":111,"model":"gpt-5.4"}}`),
		&param,
	)
	ConvertCodexCompatResponseToOpenAIResponses(
		context.Background(),
		"gpt-5.4",
		nil,
		nil,
		[]byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_initial","type":"message","role":"assistant"}}`),
		&param,
	)

	result := ConvertCodexCompatResponseToOpenAIResponsesNonStream(
		context.Background(),
		"gpt-5.4",
		nil,
		nil,
		[]byte(`{"type":"response.completed","response":{"id":"resp_rotated","created_at":222,"model":"gpt-5.4","output":[{"id":"msg_rotated","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}}`),
		&param,
	)

	if !gjson.Valid(result) {
		t.Fatalf("invalid non-stream JSON: %q", result)
	}

	parsed := gjson.Parse(result)
	if got := parsed.Get("id").String(); got != "resp_initial" {
		t.Fatalf("unexpected non-stream response id: got %q", got)
	}
	if got := parsed.Get("output.0.id").String(); got != "msg_initial" {
		t.Fatalf("unexpected non-stream output id: got %q", got)
	}
}
