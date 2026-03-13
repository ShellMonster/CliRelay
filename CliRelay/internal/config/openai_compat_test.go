package config

import "testing"

func TestSanitizeOpenAICompatibility_NormalizesAPIKeyEntryHeaders(t *testing.T) {
	cfg := &Config{
		OpenAICompatibility: []OpenAICompatibility{
			{
				Name:    " custom ",
				Prefix:  " team-a ",
				BaseURL: " https://example.com ",
				Headers: map[string]string{
					" X-Provider ": " provider ",
				},
				APIKeyEntries: []OpenAICompatibilityAPIKey{
					{
						APIKey:   " key-1 ",
						ProxyURL: " https://proxy.example.com ",
						Headers: map[string]string{
							" X-Key ": " value ",
							"Empty":   "   ",
						},
					},
				},
			},
		},
	}

	cfg.SanitizeOpenAICompatibility()

	if len(cfg.OpenAICompatibility) != 1 {
		t.Fatalf("expected 1 openai-compat entry, got %d", len(cfg.OpenAICompatibility))
	}
	entry := cfg.OpenAICompatibility[0]
	if entry.Name != "custom" {
		t.Fatalf("expected sanitized provider name, got %q", entry.Name)
	}
	if entry.Prefix != "team-a" {
		t.Fatalf("expected sanitized prefix, got %q", entry.Prefix)
	}
	if entry.BaseURL != "https://example.com" {
		t.Fatalf("expected sanitized base url, got %q", entry.BaseURL)
	}
	if got := entry.Headers["X-Provider"]; got != "provider" {
		t.Fatalf("expected sanitized provider header, got %q", got)
	}
	if len(entry.APIKeyEntries) != 1 {
		t.Fatalf("expected 1 api key entry, got %d", len(entry.APIKeyEntries))
	}
	keyEntry := entry.APIKeyEntries[0]
	if keyEntry.APIKey != "key-1" {
		t.Fatalf("expected trimmed api key, got %q", keyEntry.APIKey)
	}
	if keyEntry.ProxyURL != "https://proxy.example.com" {
		t.Fatalf("expected trimmed proxy url, got %q", keyEntry.ProxyURL)
	}
	if got := keyEntry.Headers["X-Key"]; got != "value" {
		t.Fatalf("expected sanitized key header, got %q", got)
	}
	if _, ok := keyEntry.Headers["Empty"]; ok {
		t.Fatalf("expected empty header to be dropped")
	}
}

func TestSanitizeGeminiKeys_PreservesModels(t *testing.T) {
	cfg := &Config{
		GeminiKey: []GeminiKey{
			{
				APIKey: " gem-key ",
				Models: []GeminiModel{
					{Name: " gemini-2.5-pro ", Alias: " gemini-2.5-pro "},
					{Name: " gemini-2.5-flash ", Alias: " flash "},
					{Name: "   ", Alias: "   "},
				},
			},
		},
	}

	cfg.SanitizeGeminiKeys()

	if len(cfg.GeminiKey) != 1 {
		t.Fatalf("expected 1 gemini key, got %d", len(cfg.GeminiKey))
	}
	models := cfg.GeminiKey[0].Models
	if len(models) != 2 {
		t.Fatalf("expected 2 sanitized gemini models, got %d", len(models))
	}
	if models[0].Name != "gemini-2.5-pro" || models[0].Alias != "gemini-2.5-pro" {
		t.Fatalf("unexpected first model after sanitize: %+v", models[0])
	}
	if models[1].Name != "gemini-2.5-flash" || models[1].Alias != "flash" {
		t.Fatalf("unexpected second model after sanitize: %+v", models[1])
	}
}
