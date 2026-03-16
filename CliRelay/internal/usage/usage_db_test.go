package usage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildWhereClauseIncludesResolvedChannelFilter(t *testing.T) {
	where, args := buildWhereClause(LogQueryParams{
		Days: 1,
		ChannelFilter: ChannelFilter{
			AuthIDs:      []string{"auth-id-1"},
			AuthIndexes:  []string{"auth-1"},
			ChannelNames: []string{"legacy-name"},
			Sources:      []string{"source-1"},
		},
	})

	if !strings.Contains(where, "auth_id IN (?)") {
		t.Fatalf("expected auth_id clause, got %q", where)
	}
	if !strings.Contains(where, "auth_index IN (?)") {
		t.Fatalf("expected auth_index clause, got %q", where)
	}
	if !strings.Contains(where, "channel_name IN (?)") {
		t.Fatalf("expected channel_name clause, got %q", where)
	}
	if !strings.Contains(where, "source IN (?)") {
		t.Fatalf("expected source clause, got %q", where)
	}
	if !strings.Contains(where, " OR ") {
		t.Fatalf("expected OR-combined channel filter, got %q", where)
	}
	if len(args) != 5 {
		t.Fatalf("expected cutoff + 4 filter args, got %d (%+v)", len(args), args)
	}
}

func TestInitDBMigratesExistingRequestLogsTableWithAuthIDColumn(t *testing.T) {
	CloseDB()
	t.Cleanup(CloseDB)

	dbPath := filepath.Join(t.TempDir(), "usage.db")
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer legacyDB.Close()

	_, err = legacyDB.Exec(`
CREATE TABLE request_logs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp        DATETIME NOT NULL,
  api_key          TEXT NOT NULL DEFAULT '',
  model            TEXT NOT NULL DEFAULT '',
  source           TEXT NOT NULL DEFAULT '',
  channel_name     TEXT NOT NULL DEFAULT '',
  auth_index       TEXT NOT NULL DEFAULT '',
  failed           INTEGER NOT NULL DEFAULT 0,
  latency_ms       INTEGER NOT NULL DEFAULT 0,
  input_tokens     INTEGER NOT NULL DEFAULT 0,
  output_tokens    INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  cached_tokens    INTEGER NOT NULL DEFAULT 0,
  total_tokens     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_logs_timestamp ON request_logs(timestamp DESC);
CREATE INDEX idx_logs_api_key ON request_logs(api_key);
CREATE INDEX idx_logs_model ON request_logs(model);
CREATE INDEX idx_logs_failed ON request_logs(failed);
`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB migrate legacy schema: %v", err)
	}

	db := getDB()
	if db == nil {
		t.Fatal("expected usage db to be initialized")
	}

	var authIDColumns int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('request_logs') WHERE name = 'auth_id'").Scan(&authIDColumns); err != nil {
		t.Fatalf("query auth_id column: %v", err)
	}
	if authIDColumns != 1 {
		t.Fatalf("expected auth_id column to exist once, got %d", authIDColumns)
	}

	var authIDIndexes int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_index_list('request_logs') WHERE name = 'idx_logs_auth_id'").Scan(&authIDIndexes); err != nil {
		t.Fatalf("query auth_id index: %v", err)
	}
	if authIDIndexes != 1 {
		t.Fatalf("expected idx_logs_auth_id to exist once, got %d", authIDIndexes)
	}
}

func withTempUsageDB(t *testing.T, fn func()) {
	t.Helper()

	CloseDB()
	t.Cleanup(CloseDB)

	dbPath := filepath.Join(t.TempDir(), "usage.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB(%q): %v", dbPath, err)
	}

	fn()
}

func TestTokenAggregatesKeepRawTotalsAndAdjustClaudeProcessedTokens(t *testing.T) {
	withTempUsageDB(t, func() {
		loc := analyticsLocation()
		now := time.Now().In(loc)
		startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		todayTime := startOfToday.Add(30 * time.Minute)

		InsertLog(
			"key-codex",
			"gpt-5.4",
			"codex",
			"Codex",
			"auth-codex",
			"0",
			false,
			todayTime,
			120,
			TokenStats{InputTokens: 10, OutputTokens: 2, CachedTokens: 9, TotalTokens: 12},
			"",
			"",
			map[string]any{"provider": "codex"},
		)
		InsertLog(
			"key-claude-compat",
			"claude-sonnet-4-5",
			"openai",
			"Claude Compat",
			"auth-claude-compat",
			"2",
			false,
			todayTime.Add(5*time.Minute),
			140,
			TokenStats{InputTokens: 3, OutputTokens: 1, CachedTokens: 20, TotalTokens: 4},
			"",
			"",
			map[string]any{"provider": "openai-compat", "requested_model": "claude-sonnet-4-5"},
		)
		InsertLog(
			"key-failed",
			"claude-opus-4-6",
			"claude",
			"Claude",
			"auth-claude",
			"1",
			true,
			todayTime.Add(10*time.Minute),
			90,
			TokenStats{InputTokens: 50, OutputTokens: 10, CachedTokens: 40, TotalTokens: 60},
			"",
			"",
			map[string]any{"provider": "claude"},
		)

		stats, err := QueryStats(LogQueryParams{Days: 1})
		if err != nil {
			t.Fatalf("QueryStats() error = %v", err)
		}
		if stats.TotalTokens != 76 {
			t.Fatalf("QueryStats TotalTokens = %d, want 76", stats.TotalTokens)
		}

		usageSummary, err := QueryUsageSummary()
		if err != nil {
			t.Fatalf("QueryUsageSummary() error = %v", err)
		}
		if usageSummary.TotalTokens != 76 {
			t.Fatalf("QueryUsageSummary TotalTokens = %d, want 76", usageSummary.TotalTokens)
		}

		summary, err := QueryDashboardSummary(1, "", "", ChannelFilter{})
		if err != nil {
			t.Fatalf("QueryDashboardSummary() error = %v", err)
		}

		if summary.TotalRequests != 3 {
			t.Fatalf("TotalRequests = %d, want 3", summary.TotalRequests)
		}
		if summary.SuccessRequests != 2 {
			t.Fatalf("SuccessRequests = %d, want 2", summary.SuccessRequests)
		}
		if summary.FailedRequests != 1 {
			t.Fatalf("FailedRequests = %d, want 1", summary.FailedRequests)
		}
		if summary.InputTokens != 63 {
			t.Fatalf("InputTokens = %d, want 63", summary.InputTokens)
		}
		if summary.OutputTokens != 13 {
			t.Fatalf("OutputTokens = %d, want 13", summary.OutputTokens)
		}
		if summary.CachedTokens != 69 {
			t.Fatalf("CachedTokens = %d, want 69", summary.CachedTokens)
		}
		if summary.TotalTokens != 76 {
			t.Fatalf("TotalTokens = %d, want 76", summary.TotalTokens)
		}
		if summary.ProcessedTokens != 136 {
			t.Fatalf("ProcessedTokens = %d, want 136", summary.ProcessedTokens)
		}

		points, err := QueryModelDistribution(1, 10, "", "", ChannelFilter{})
		if err != nil {
			t.Fatalf("QueryModelDistribution() error = %v", err)
		}
		if len(points) != 3 {
			t.Fatalf("len(points) = %d, want 3", len(points))
		}

		byModel := make(map[string]ModelDistributionPoint, len(points))
		for _, point := range points {
			byModel[point.Model] = point
		}

		compatPoint := byModel["claude-sonnet-4-5"]
		if compatPoint.TotalTokens != 4 {
			t.Fatalf("compat TotalTokens = %d, want 4", compatPoint.TotalTokens)
		}
		if compatPoint.CachedTokens != 20 {
			t.Fatalf("compat CachedTokens = %d, want 20", compatPoint.CachedTokens)
		}
		if compatPoint.ProcessedTokens != 24 {
			t.Fatalf("compat ProcessedTokens = %d, want 24", compatPoint.ProcessedTokens)
		}
		if compatPoint.Tokens != compatPoint.ProcessedTokens {
			t.Fatalf("compat Tokens = %d, want processed %d", compatPoint.Tokens, compatPoint.ProcessedTokens)
		}

		codexPoint := byModel["gpt-5.4"]
		if codexPoint.ProcessedTokens != 12 {
			t.Fatalf("codex ProcessedTokens = %d, want 12", codexPoint.ProcessedTokens)
		}

		detailStats, err := QueryUsageModelDetailStats(1, 10)
		if err != nil {
			t.Fatalf("QueryUsageModelDetailStats() error = %v", err)
		}
		detailByModel := make(map[string]UsageModelDetailStats, len(detailStats))
		for _, item := range detailStats {
			detailByModel[item.Model] = item
		}
		compatDetail := detailByModel["claude-sonnet-4-5"]
		if compatDetail.TotalTokens != 4 {
			t.Fatalf("compat detail TotalTokens = %d, want 4", compatDetail.TotalTokens)
		}
		if compatDetail.ProcessedTokens != 24 {
			t.Fatalf("compat detail ProcessedTokens = %d, want 24", compatDetail.ProcessedTokens)
		}
	})
}

func TestUsageBucketsUseChinaTimezone(t *testing.T) {
	withTempUsageDB(t, func() {
		loc := analyticsLocation()
		now := time.Now().In(loc)
		startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

		yesterdayLocal := startOfToday.Add(-30 * time.Minute)
		todayLocal := startOfToday.Add(30 * time.Minute)
		currentHourStart := now.Truncate(time.Hour)
		hourLocal := currentHourStart.Add(15 * time.Minute)

		InsertLog(
			"key-yesterday",
			"claude-opus-4-6",
			"claude",
			"Claude",
			"auth-claude",
			"1",
			false,
			yesterdayLocal,
			100,
			TokenStats{InputTokens: 2, OutputTokens: 1, CachedTokens: 7, TotalTokens: 3},
			"",
			"",
			map[string]any{"provider": "claude"},
		)
		InsertLog(
			"key-today",
			"claude-opus-4-6",
			"claude",
			"Claude",
			"auth-claude",
			"1",
			false,
			todayLocal,
			100,
			TokenStats{InputTokens: 3, OutputTokens: 1, CachedTokens: 20, TotalTokens: 4},
			"",
			"",
			map[string]any{"provider": "claude"},
		)
		InsertLog(
			"key-hour",
			"gpt-5.4",
			"codex",
			"Codex",
			"auth-codex",
			"0",
			false,
			hourLocal,
			80,
			TokenStats{InputTokens: 10, OutputTokens: 2, CachedTokens: 9, TotalTokens: 12},
			"",
			"",
			map[string]any{"provider": "codex"},
		)

		daily, err := QueryDailyTrend(2, "", "", ChannelFilter{})
		if err != nil {
			t.Fatalf("QueryDailyTrend() error = %v", err)
		}
		if len(daily) != 2 {
			t.Fatalf("len(daily) = %d, want 2", len(daily))
		}

		wantYesterday := yesterdayLocal.Format("2006-01-02")
		wantToday := todayLocal.Format("2006-01-02")
		if daily[0].Day != wantYesterday {
			t.Fatalf("daily[0].Day = %q, want %q", daily[0].Day, wantYesterday)
		}
		if daily[1].Day != wantToday {
			t.Fatalf("daily[1].Day = %q, want %q", daily[1].Day, wantToday)
		}
		if daily[0].TotalTokens != 3 {
			t.Fatalf("daily[0].TotalTokens = %d, want 3", daily[0].TotalTokens)
		}
		if daily[1].TotalTokens != 16 {
			t.Fatalf("daily[1].TotalTokens = %d, want 16", daily[1].TotalTokens)
		}
		if daily[0].ProcessedTokens != 10 {
			t.Fatalf("daily[0].ProcessedTokens = %d, want 10", daily[0].ProcessedTokens)
		}
		if daily[1].ProcessedTokens != 36 {
			t.Fatalf("daily[1].ProcessedTokens = %d, want 36", daily[1].ProcessedTokens)
		}

		requestTrend, err := QueryUsageRequestTrend(2, "")
		if err != nil {
			t.Fatalf("QueryUsageRequestTrend() error = %v", err)
		}
		if len(requestTrend) != 2 {
			t.Fatalf("len(requestTrend) = %d, want 2", len(requestTrend))
		}
		if requestTrend[0].Bucket != wantYesterday {
			t.Fatalf("requestTrend[0].Bucket = %q, want %q", requestTrend[0].Bucket, wantYesterday)
		}
		if requestTrend[1].Bucket != wantToday {
			t.Fatalf("requestTrend[1].Bucket = %q, want %q", requestTrend[1].Bucket, wantToday)
		}
		if requestTrend[0].TotalTokens != 3 {
			t.Fatalf("requestTrend[0].TotalTokens = %d, want 3", requestTrend[0].TotalTokens)
		}
		if requestTrend[1].TotalTokens != 16 {
			t.Fatalf("requestTrend[1].TotalTokens = %d, want 16", requestTrend[1].TotalTokens)
		}

		hourly, err := QueryHourlySeries(2, "", "", ChannelFilter{})
		if err != nil {
			t.Fatalf("QueryHourlySeries() error = %v", err)
		}
		if len(hourly) == 0 {
			t.Fatal("len(hourly) = 0, want >= 1")
		}

		expectedHour := currentHourStart.Format("2006-01-02T15:00:00-07:00")
		var matched *HourlySeriesPoint
		for i := range hourly {
			if hourly[i].Hour == expectedHour && hourly[i].Model == "gpt-5.4" {
				matched = &hourly[i]
				break
			}
		}
		if matched == nil {
			t.Fatalf("expected hourly bucket %q for gpt-5.4", expectedHour)
		}
		if matched.ProcessedTokens != 12 {
			t.Fatalf("matched.ProcessedTokens = %d, want 12", matched.ProcessedTokens)
		}
	})
}
