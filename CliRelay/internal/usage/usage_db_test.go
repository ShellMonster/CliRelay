package usage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
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
