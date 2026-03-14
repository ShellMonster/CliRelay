package codexcompat

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	codexresponses "github.com/router-for-me/CLIProxyAPI/v6/internal/translator/codex/openai/responses"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var compatDataTag = []byte("data:")

type codexCompatTranslationState struct {
	compat      *codexCompatResponsesState
	passthrough any
}

type codexCompatResponsesState struct {
	responseID string
	createdAt  int64
	itemIDs    map[int]string
	itemTypes  map[int]string
}

func ConvertCodexCompatResponseToOpenAIResponses(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) []string {
	state := ensureCodexCompatTranslationState(param)
	normalized := normalizeCodexCompatResponseIDs(rawJSON, state.compat)
	return codexresponses.ConvertCodexResponseToOpenAIResponses(ctx, modelName, originalRequestRawJSON, requestRawJSON, normalized, &state.passthrough)
}

func ConvertCodexCompatResponseToOpenAIResponsesNonStream(ctx context.Context, modelName string, originalRequestRawJSON, requestRawJSON, rawJSON []byte, param *any) string {
	state := ensureCodexCompatTranslationState(param)
	normalized := normalizeCodexCompatResponseIDs(rawJSON, state.compat)
	if bytes.HasPrefix(normalized, []byte("data:")) {
		normalized = bytes.TrimSpace(normalized[len(compatDataTag):])
	}
	return codexresponses.ConvertCodexResponseToOpenAIResponsesNonStream(ctx, modelName, originalRequestRawJSON, requestRawJSON, normalized, &state.passthrough)
}

func ensureCodexCompatTranslationState(param *any) *codexCompatTranslationState {
	if param == nil {
		return &codexCompatTranslationState{compat: newCodexCompatResponsesState()}
	}
	if *param == nil {
		*param = &codexCompatTranslationState{compat: newCodexCompatResponsesState()}
	}
	state, ok := (*param).(*codexCompatTranslationState)
	if !ok || state == nil {
		state = &codexCompatTranslationState{compat: newCodexCompatResponsesState()}
		*param = state
	}
	if state.compat == nil {
		state.compat = newCodexCompatResponsesState()
	}
	if state.compat.itemIDs == nil {
		state.compat.itemIDs = make(map[int]string)
	}
	if state.compat.itemTypes == nil {
		state.compat.itemTypes = make(map[int]string)
	}
	return state
}

func newCodexCompatResponsesState() *codexCompatResponsesState {
	return &codexCompatResponsesState{
		itemIDs:   make(map[int]string),
		itemTypes: make(map[int]string),
	}
}

func ensureCodexCompatResponsesState(param *any) *codexCompatResponsesState {
	state := ensureCodexCompatTranslationState(param)
	if state == nil {
		return newCodexCompatResponsesState()
	}
	return state.compat
}

func normalizeCodexCompatResponseIDs(rawJSON []byte, state *codexCompatResponsesState) []byte {
	if state == nil {
		return rawJSON
	}
	if state.itemIDs == nil {
		state.itemIDs = make(map[int]string)
	}
	if state.itemTypes == nil {
		state.itemTypes = make(map[int]string)
	}
	trimmed := bytes.TrimSpace(rawJSON)
	sseEncoded := bytes.HasPrefix(trimmed, compatDataTag)
	payload := trimmed
	if sseEncoded {
		payload = bytes.TrimSpace(trimmed[len(compatDataTag):])
	}
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return rawJSON
	}

	out := bytes.Clone(payload)
	eventType := strings.TrimSpace(gjson.GetBytes(out, "type").String())
	if eventType == "" {
		return rawJSON
	}

	if createdAt := gjson.GetBytes(out, "response.created_at"); createdAt.Exists() && state.createdAt == 0 {
		state.createdAt = createdAt.Int()
	}

	switch eventType {
	case "response.created", "response.in_progress", "response.completed", "response.done":
		out = rewriteCompatResponseEnvelope(out, state)
	}

	if outputIndexResult := gjson.GetBytes(out, "output_index"); outputIndexResult.Exists() {
		outputIndex := int(outputIndexResult.Int())
		itemType := compatItemTypeForEvent(eventType, out)
		stableItemID := state.ensureItemID(outputIndex, itemType, compatCandidateItemID(out))
		switch eventType {
		case "response.output_item.added", "response.output_item.done":
			out, _ = sjson.SetBytes(out, "item.id", stableItemID)
		case "response.content_part.added", "response.content_part.done",
			"response.output_text.delta", "response.output_text.done",
			"response.function_call_arguments.delta", "response.function_call_arguments.done",
			"response.reasoning_summary_part.added", "response.reasoning_summary_part.done",
			"response.reasoning_summary_text.delta", "response.reasoning_summary_text.done":
			out, _ = sjson.SetBytes(out, "item_id", stableItemID)
		}
	}

	if !sseEncoded {
		return out
	}

	line := make([]byte, 0, len("data: ")+len(out))
	line = append(line, []byte("data: ")...)
	line = append(line, out...)
	return line
}

func rewriteCompatResponseEnvelope(payload []byte, state *codexCompatResponsesState) []byte {
	stableResponseID := state.ensureResponseID(strings.TrimSpace(gjson.GetBytes(payload, "response.id").String()))
	if stableResponseID != "" {
		payload, _ = sjson.SetBytes(payload, "response.id", stableResponseID)
	}
	if state.createdAt != 0 && gjson.GetBytes(payload, "response.created_at").Exists() {
		payload, _ = sjson.SetBytes(payload, "response.created_at", state.createdAt)
	}

	outputs := gjson.GetBytes(payload, "response.output")
	if !outputs.Exists() || !outputs.IsArray() {
		return payload
	}

	items := outputs.Array()
	for i := range items {
		item := items[i]
		itemType := strings.TrimSpace(item.Get("type").String())
		stableItemID := state.ensureItemID(i, itemType, strings.TrimSpace(item.Get("id").String()))
		if stableItemID == "" {
			continue
		}
		payload, _ = sjson.SetBytes(payload, fmt.Sprintf("response.output.%d.id", i), stableItemID)
	}
	return payload
}

func compatCandidateItemID(payload []byte) string {
	if itemID := strings.TrimSpace(gjson.GetBytes(payload, "item.id").String()); itemID != "" {
		return itemID
	}
	return strings.TrimSpace(gjson.GetBytes(payload, "item_id").String())
}

func compatItemTypeForEvent(eventType string, payload []byte) string {
	if itemType := strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()); itemType != "" {
		return itemType
	}
	switch {
	case strings.Contains(eventType, "reasoning_summary"):
		return "reasoning"
	case strings.Contains(eventType, "function_call_arguments"):
		return "function_call"
	case strings.Contains(eventType, "output_text"), strings.Contains(eventType, "content_part"):
		return "message"
	default:
		return "item"
	}
}

func compatItemIDPrefix(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "message":
		return "msg"
	case "reasoning":
		return "rs"
	case "function_call":
		return "fc"
	default:
		return "item"
	}
}

func (s *codexCompatResponsesState) ensureResponseID(candidate string) string {
	if s == nil {
		return candidate
	}
	if strings.TrimSpace(s.responseID) != "" {
		return s.responseID
	}
	if candidate != "" {
		s.responseID = candidate
		return s.responseID
	}
	s.responseID = "response_codex_compat"
	return s.responseID
}

func (s *codexCompatResponsesState) ensureItemID(outputIndex int, itemType string, candidate string) string {
	if s == nil {
		return candidate
	}
	if stable, ok := s.itemIDs[outputIndex]; ok && strings.TrimSpace(stable) != "" {
		if _, exists := s.itemTypes[outputIndex]; !exists && strings.TrimSpace(itemType) != "" {
			s.itemTypes[outputIndex] = strings.TrimSpace(itemType)
		}
		return stable
	}

	if strings.TrimSpace(itemType) != "" {
		s.itemTypes[outputIndex] = strings.TrimSpace(itemType)
	} else if existingType := strings.TrimSpace(s.itemTypes[outputIndex]); existingType != "" {
		itemType = existingType
	}

	if candidate != "" {
		s.itemIDs[outputIndex] = candidate
		return candidate
	}

	responseID := s.ensureResponseID("")
	stable := fmt.Sprintf("%s_%s_%d", compatItemIDPrefix(itemType), responseID, outputIndex)
	s.itemIDs[outputIndex] = stable
	return stable
}
