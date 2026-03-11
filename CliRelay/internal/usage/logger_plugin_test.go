package usage

import (
	"path/filepath"
	"testing"
	"time"

	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestRecord_DropsContentWhenDisabled(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(CloseDB)

	SetStatisticsEnabled(true)
	SetLogContentEnabled(false)
	t.Cleanup(func() {
		SetStatisticsEnabled(true)
		SetLogContentEnabled(true)
	})

	stats := NewRequestStatistics()
	stats.Record(nil, coreusage.Record{
		APIKey:      "key",
		Model:       "model",
		Source:      "source",
		RequestedAt: time.Now(),
		Detail: coreusage.Detail{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
		InputContent:  "request-body",
		OutputContent: "response-body",
	})

	time.Sleep(50 * time.Millisecond)

	got, err := QueryLogContent(1)
	if err != nil {
		t.Fatalf("QueryLogContent: %v", err)
	}
	if got.InputContent != "" || got.OutputContent != "" {
		t.Fatalf("expected content to be dropped, got input=%q output=%q", got.InputContent, got.OutputContent)
	}
	if got.HasContent {
		t.Fatal("expected has_content=false when content logging is disabled")
	}
}
