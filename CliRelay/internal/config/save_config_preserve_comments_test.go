package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveConfigPreserveComments_PersistsParticipateInDefaultRoutingFalse(t *testing.T) {
	t.Run("codex", func(t *testing.T) {
		configPath := writeConfigFixture(t, `
codex-api-key:
  - api-key: sk-test
    base-url: https://example.com
`)

		cfg := &Config{
			CodexKey: []CodexKey{
				{
					APIKey:                      "sk-test",
					BaseURL:                     "https://example.com",
					ParticipateInDefaultRouting: boolPtr(false),
				},
			},
		}

		if err := SaveConfigPreserveComments(configPath, cfg); err != nil {
			t.Fatalf("SaveConfigPreserveComments() error = %v", err)
		}

		assertConfigContains(t, configPath, "participate-in-default-routing: false")

		loaded, err := LoadConfig(configPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if len(loaded.CodexKey) != 1 {
			t.Fatalf("expected 1 codex key, got %d", len(loaded.CodexKey))
		}
		if loaded.CodexKey[0].ParticipateInDefaultRouting == nil || *loaded.CodexKey[0].ParticipateInDefaultRouting {
			t.Fatalf("expected participate-in-default-routing=false after reload, got %+v", loaded.CodexKey[0].ParticipateInDefaultRouting)
		}
	})

	t.Run("openai-compatibility", func(t *testing.T) {
		configPath := writeConfigFixture(t, `
openai-compatibility:
  - name: compat
    base-url: https://example.com/v1
    api-key-entries:
      - api-key: sk-test
`)

		cfg := &Config{
			OpenAICompatibility: []OpenAICompatibility{
				{
					Name:                        "compat",
					BaseURL:                     "https://example.com/v1",
					ParticipateInDefaultRouting: boolPtr(false),
					APIKeyEntries: []OpenAICompatibilityAPIKey{
						{APIKey: "sk-test"},
					},
				},
			},
		}

		if err := SaveConfigPreserveComments(configPath, cfg); err != nil {
			t.Fatalf("SaveConfigPreserveComments() error = %v", err)
		}

		assertConfigContains(t, configPath, "participate-in-default-routing: false")

		loaded, err := LoadConfig(configPath)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if len(loaded.OpenAICompatibility) != 1 {
			t.Fatalf("expected 1 openai-compat entry, got %d", len(loaded.OpenAICompatibility))
		}
		if loaded.OpenAICompatibility[0].ParticipateInDefaultRouting == nil || *loaded.OpenAICompatibility[0].ParticipateInDefaultRouting {
			t.Fatalf("expected participate-in-default-routing=false after reload, got %+v", loaded.OpenAICompatibility[0].ParticipateInDefaultRouting)
		}
	})
}

func writeConfigFixture(t *testing.T, contents string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return configPath
}

func assertConfigContains(t *testing.T, configPath, expected string) {
	t.Helper()

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, expected) {
		t.Fatalf("config %s does not contain %q:\n%s", configPath, expected, text)
	}
}

func boolPtr(v bool) *bool {
	return &v
}
