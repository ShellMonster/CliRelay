package modelsync

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
)

func TestMergeCodexModels_AppendsOnlyNewModels(t *testing.T) {
	entry := &config.CodexKey{
		Models: []config.CodexModel{
			{Name: "gpt-5", Alias: "g5"},
		},
	}

	changed := mergeCodexModels(entry, []*registry.ModelInfo{
		{ID: "gpt-5"},
		{ID: "gpt-5.4"},
		{Name: "o3"},
	})
	if !changed {
		t.Fatal("expected merge to report changes")
	}
	if len(entry.Models) != 3 {
		t.Fatalf("expected 3 models after merge, got %d", len(entry.Models))
	}
	if entry.Models[0].Alias != "g5" {
		t.Fatalf("expected existing alias to remain unchanged, got %q", entry.Models[0].Alias)
	}
	if entry.Models[1].Name != "gpt-5.4" || entry.Models[1].Alias != "" {
		t.Fatalf("unexpected second model: %#v", entry.Models[1])
	}
	if entry.Models[2].Name != "o3" {
		t.Fatalf("unexpected third model: %#v", entry.Models[2])
	}
}

