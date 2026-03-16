package executor

import (
	"encoding/json"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

func sanitizeCopilotCompatGeminiTools(body []byte) []byte {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body
	}

	tools, ok := root["tools"].([]any)
	if !ok || len(tools) == 0 {
		return body
	}

	changed := false
	for i, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok {
			continue
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			continue
		}
		parameters, ok := function["parameters"].(map[string]any)
		if !ok {
			continue
		}
		parametersJSON, err := json.Marshal(parameters)
		if err != nil {
			continue
		}
		cleanedJSON := util.CleanJSONSchemaForGemini(string(parametersJSON))
		var cleaned map[string]any
		if err := json.Unmarshal([]byte(cleanedJSON), &cleaned); err != nil {
			continue
		}
		function["parameters"] = cleaned
		tool["function"] = function
		tools[i] = tool
		changed = true
	}
	if !changed {
		return body
	}

	root["tools"] = tools
	out, err := json.Marshal(root)
	if err != nil {
		return body
	}
	return out
}
