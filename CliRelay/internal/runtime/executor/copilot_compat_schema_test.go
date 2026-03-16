package executor

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestSanitizeCopilotCompatChatPayload_GeminiSanitizesTools(t *testing.T) {
	input := []byte(`{
		"model":"gemini-3-pro-preview",
		"stream":true,
		"stream_options":{"include_usage":true},
		"reasoning_effort":"medium",
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"question",
					"description":"Ask question",
					"parameters":{
						"$schema":"https://json-schema.org/draft/2020-12/schema",
						"definitions":{
							"User":{
								"type":"object",
								"properties":{
									"name":{"type":"string"}
								}
							}
						},
						"type":"object",
						"properties":{
							"$id":{"type":"string"},
							"dateStart":{"type":["string","number"],"default":"now"},
							"arguments":{
								"anyOf":[
									{"type":"string"},
									{
										"type":"object",
										"properties":{
											"count":{"type":"integer"}
										},
										"required":["count"]
									}
								]
							},
							"customer":{"$ref":"#/definitions/User"}
						},
						"required":["dateStart"],
						"additionalProperties":false
					}
				}
			}
		]
	}`)

	out := sanitizeCopilotCompatChatPayload(input, "gemini-3-pro-preview")

	if gjson.GetBytes(out, "stream_options").Exists() {
		t.Fatalf("stream_options should be removed")
	}
	if gjson.GetBytes(out, "reasoning_effort").Exists() {
		t.Fatalf("reasoning_effort should be removed for gemini models")
	}
	if gjson.GetBytes(out, "tools.0.function.parameters.$schema").Exists() {
		t.Fatalf("$schema should be removed from tool schema")
	}
	if gjson.GetBytes(out, "tools.0.function.parameters.definitions").Exists() {
		t.Fatalf("definitions should be removed from cleaned tool schema")
	}
	if gjson.GetBytes(out, "tools.0.function.parameters.properties.dateStart.default").Exists() {
		t.Fatalf("default should be removed from nested tool schema")
	}
	if got := gjson.GetBytes(out, "tools.0.function.parameters.properties.dateStart.type").String(); got != "string" {
		t.Fatalf("dateStart.type = %q, want %q", got, "string")
	}
	dateStartDesc := gjson.GetBytes(out, "tools.0.function.parameters.properties.dateStart.description").String()
	if got, want := dateStartDesc, "Accepts: string | number"; !containsSubstring(got, want) {
		t.Fatalf("dateStart.description = %q, want substring %q", got, want)
	}
	if got, want := dateStartDesc, "default: now"; !containsSubstring(got, want) {
		t.Fatalf("dateStart.description = %q, want substring %q", got, want)
	}
	if got := gjson.GetBytes(out, "tools.0.function.parameters.properties.arguments.type").String(); got != "object" {
		t.Fatalf("arguments.type = %q, want %q", got, "object")
	}
	argumentsDesc := gjson.GetBytes(out, "tools.0.function.parameters.properties.arguments.description").String()
	if got, want := argumentsDesc, "Accepts: string | object"; !containsSubstring(got, want) {
		t.Fatalf("arguments.description = %q, want substring %q", got, want)
	}
	if got := gjson.GetBytes(out, "tools.0.function.parameters.properties.customer.type").String(); got != "object" {
		t.Fatalf("customer.type = %q, want %q", got, "object")
	}
	if got, want := gjson.GetBytes(out, "tools.0.function.parameters.properties.customer.description").String(), "See: User"; !containsSubstring(got, want) {
		t.Fatalf("customer.description = %q, want substring %q", got, want)
	}
	if !gjson.GetBytes(out, "tools.0.function.parameters.properties.$id").Exists() {
		t.Fatalf("properties.$id should be preserved as a user-facing field")
	}
}

func TestSanitizeCopilotCompatChatPayload_NonGeminiKeepsToolSchema(t *testing.T) {
	input := []byte(`{
		"model":"gpt-5.4",
		"stream_options":{"include_usage":true},
		"reasoning_effort":"medium",
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"question",
					"parameters":{
						"$schema":"https://json-schema.org/draft/2020-12/schema",
						"type":"object"
					}
				}
			}
		]
	}`)

	out := sanitizeCopilotCompatChatPayload(input, "gpt-5.4")

	if gjson.GetBytes(out, "stream_options").Exists() {
		t.Fatalf("stream_options should be removed for all copilot chat payloads")
	}
	if got := gjson.GetBytes(out, "reasoning_effort").String(); got != "medium" {
		t.Fatalf("reasoning_effort = %q, want %q", got, "medium")
	}
	if !gjson.GetBytes(out, "tools.0.function.parameters.$schema").Exists() {
		t.Fatalf("non-gemini payloads should keep tool schema untouched")
	}
}

func containsSubstring(got, want string) bool {
	return strings.Contains(got, want)
}
