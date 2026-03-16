package executor

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func parseCopilotCompatSSEJSON(t *testing.T, chunk []byte) gjson.Result {
	t.Helper()

	line := strings.TrimSpace(string(chunk))
	line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if !gjson.Valid(line) {
		t.Fatalf("invalid SSE JSON payload: %q", chunk)
	}
	return gjson.Parse(line)
}

func TestNormalizeCopilotCompatResponseIDs_StreamStabilizesIDs(t *testing.T) {
	state := newCopilotCompatResponsesState()

	created := normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.created","response":{"id":"resp_initial","created_at":111,"model":"gpt-5.4"}}`), state)
	inProgress := normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.in_progress","response":{"id":"resp_rotated","created_at":222,"model":"gpt-5.4"}}`), state)
	itemAdded := normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_initial","type":"message","role":"assistant"}}`), state)
	textDelta := normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.output_text.delta","output_index":0,"item_id":"msg_rotated","delta":"hi"}`), state)

	if got := parseCopilotCompatSSEJSON(t, created).Get("response.id").String(); got != "resp_initial" {
		t.Fatalf("response.created id = %q, want %q", got, "resp_initial")
	}
	inProgressJSON := parseCopilotCompatSSEJSON(t, inProgress)
	if got := inProgressJSON.Get("response.id").String(); got != "resp_initial" {
		t.Fatalf("response.in_progress id = %q, want %q", got, "resp_initial")
	}
	if got := inProgressJSON.Get("response.created_at").Int(); got != 111 {
		t.Fatalf("response.in_progress created_at = %d, want %d", got, 111)
	}
	if got := parseCopilotCompatSSEJSON(t, itemAdded).Get("item.id").String(); got != "msg_initial" {
		t.Fatalf("response.output_item.added item.id = %q, want %q", got, "msg_initial")
	}
	if got := parseCopilotCompatSSEJSON(t, textDelta).Get("item_id").String(); got != "msg_initial" {
		t.Fatalf("response.output_text.delta item_id = %q, want %q", got, "msg_initial")
	}
}

func TestNormalizeCopilotCompatResponseIDs_NonStreamUsesExistingState(t *testing.T) {
	state := newCopilotCompatResponsesState()

	_ = normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.created","response":{"id":"resp_initial","created_at":111,"model":"gpt-5.4"}}`), state)
	_ = normalizeCopilotCompatResponseIDs([]byte(`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"msg_initial","type":"message","role":"assistant"}}`), state)

	completed := normalizeCopilotCompatResponseIDs([]byte(`{"id":"resp_rotated","created_at":222,"model":"gpt-5.4","output":[{"id":"msg_rotated","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello"}]}]}`), state)
	if !gjson.ValidBytes(completed) {
		t.Fatalf("invalid normalized JSON: %s", completed)
	}

	parsed := gjson.ParseBytes(completed)
	if got := parsed.Get("id").String(); got != "resp_initial" {
		t.Fatalf("response id = %q, want %q", got, "resp_initial")
	}
	if got := parsed.Get("created_at").Int(); got != 111 {
		t.Fatalf("created_at = %d, want %d", got, 111)
	}
	if got := parsed.Get("output.0.id").String(); got != "msg_initial" {
		t.Fatalf("output.0.id = %q, want %q", got, "msg_initial")
	}
}
