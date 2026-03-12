package usage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	_ "modernc.org/sqlite"
)

// LogRow represents a single request log entry returned by QueryLogs.
type LogRow struct {
	ID              int64     `json:"id"`
	Timestamp       time.Time `json:"timestamp"`
	APIKey          string    `json:"api_key"`
	APIKeyName      string    `json:"api_key_name"`
	Model           string    `json:"model"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
	Source          string    `json:"source"`
	ChannelName     string    `json:"channel_name"`
	AuthIndex       string    `json:"auth_index"`
	Failed          bool      `json:"failed"`
	LatencyMs       int64     `json:"latency_ms"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	ReasoningTokens int64     `json:"reasoning_tokens"`
	CachedTokens    int64     `json:"cached_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	HasContent      bool      `json:"has_content"`
}

// LogQueryParams holds filter/pagination parameters for QueryLogs.
type LogQueryParams struct {
	Page   int    // 1-based
	Size   int    // rows per page
	Days   int    // time range in days
	APIKey string // exact match filter
	Model  string // exact match filter
	Status string // "success", "failed", or "" (all)
}

// LogQueryResult holds the paginated query result.
type LogQueryResult struct {
	Items []LogRow `json:"items"`
	Total int64    `json:"total"`
	Page  int      `json:"page"`
	Size  int      `json:"size"`
}

// FilterOptions holds the available filter values for the UI.
type FilterOptions struct {
	APIKeys     []string          `json:"api_keys"`
	APIKeyNames map[string]string `json:"api_key_names"`
	Models      []string          `json:"models"`
}

// LogStats holds aggregated stats over the filtered result set.
type LogStats struct {
	Total       int64   `json:"total"`
	SuccessRate float64 `json:"success_rate"`
	TotalTokens int64   `json:"total_tokens"`
}

// UsageSummary mirrors the high-level counters expected by management usage endpoints.
type UsageSummary struct {
	TotalRequests int64 `json:"total_requests"`
	SuccessCount  int64 `json:"success_count"`
	FailureCount  int64 `json:"failure_count"`
	TotalTokens   int64 `json:"total_tokens"`
}

var (
	usageDB     *sql.DB
	usageDBMu   sync.Mutex
	usageDBPath string
)

const createTableSQL = `
CREATE TABLE IF NOT EXISTS request_logs (
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
  total_tokens     INTEGER NOT NULL DEFAULT 0,
  input_content    TEXT NOT NULL DEFAULT '',
  output_content   TEXT NOT NULL DEFAULT '',
  request_meta     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON request_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_logs_api_key ON request_logs(api_key);
CREATE INDEX IF NOT EXISTS idx_logs_model ON request_logs(model);
CREATE INDEX IF NOT EXISTS idx_logs_failed ON request_logs(failed);
`

// migrateContentColumns adds input_content/output_content columns to an
// existing request_logs table that was created before this feature.
func migrateContentColumns(db *sql.DB) {
	for _, col := range []string{"input_content", "output_content", "request_meta"} {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE request_logs ADD COLUMN %s TEXT NOT NULL DEFAULT ''", col))
		if err != nil {
			// "duplicate column name" is expected when already migrated
			if !strings.Contains(err.Error(), "duplicate") {
				log.Warnf("usage: migrate column %s: %v", col, err)
			}
		}
	}
}

const maxContentBytes = 100 * 1024 // 100 KB per field

// InitDB opens (or creates) the SQLite database at the given path and creates
// the request_logs table if it doesn't exist.
func InitDB(dbPath string) error {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()

	if usageDB != nil {
		return nil // already initialised
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("usage: open sqlite: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite performs best with a single writer
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(createTableSQL); err != nil {
		_ = db.Close()
		return fmt.Errorf("usage: create table: %w", err)
	}

	usageDB = db
	usageDBPath = dbPath
	migrateContentColumns(db)
	log.Infof("usage: SQLite database initialised at %s", dbPath)
	return nil
}

// CloseDB closes the SQLite database gracefully.
func CloseDB() {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()

	if usageDB != nil {
		_ = usageDB.Close()
		usageDB = nil
		log.Info("usage: SQLite database closed")
	}
}

// InsertLog writes a single request log entry into the SQLite database.
// It is safe to call concurrently.
func InsertLog(apiKey, model, source, channelName, authIndex string,
	failed bool, timestamp time.Time, latencyMs int64, tokens TokenStats,
	inputContent, outputContent string, requestMeta map[string]any) {

	db := getDB()
	if db == nil {
		return
	}

	failedInt := 0
	if failed {
		failedInt = 1
	}

	// Truncate content to limit storage cost
	if len(inputContent) > maxContentBytes {
		inputContent = inputContent[:maxContentBytes] + "\n... (truncated)"
	}
	if len(outputContent) > maxContentBytes {
		outputContent = outputContent[:maxContentBytes] + "\n... (truncated)"
	}

	requestMetaJSON := ""
	if len(requestMeta) > 0 {
		if data, err := json.Marshal(requestMeta); err != nil {
			log.Warnf("usage: marshal request_meta: %v", err)
		} else {
			requestMetaJSON = string(data)
		}
	}

	_, err := db.Exec(
		`INSERT INTO request_logs
			(timestamp, api_key, model, source, channel_name, auth_index,
			 failed, latency_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens,
			 input_content, output_content, request_meta)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		timestamp.UTC().Format(time.RFC3339Nano),
		apiKey, model, source, channelName, authIndex,
		failedInt, latencyMs,
		tokens.InputTokens, tokens.OutputTokens, tokens.ReasoningTokens,
		tokens.CachedTokens, tokens.TotalTokens,
		inputContent, outputContent, requestMetaJSON,
	)
	if err != nil {
		log.Errorf("usage: insert log: %v", err)
	}
}

// QueryLogs returns a paginated, filtered list of log entries.
func QueryLogs(params LogQueryParams) (LogQueryResult, error) {
	db := getDB()
	if db == nil {
		return LogQueryResult{Page: params.Page, Size: params.Size}, nil
	}

	// Normalise parameters
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Size < 1 {
		params.Size = 50
	}
	if params.Size > 200 {
		params.Size = 200
	}
	if params.Days < 1 {
		params.Days = 7
	}

	where, args := buildWhereClause(params)

	// Count total
	var total int64
	countSQL := "SELECT COUNT(*) FROM request_logs" + where
	if err := db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return LogQueryResult{}, fmt.Errorf("usage: count query: %w", err)
	}

	// Fetch page
	offset := (params.Page - 1) * params.Size
	querySQL := "SELECT id, timestamp, api_key, model, source, channel_name, auth_index, " +
		"failed, latency_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens, " +
		"(CASE WHEN length(input_content) > 0 OR length(output_content) > 0 THEN 1 ELSE 0 END) as has_content, request_meta " +
		"FROM request_logs" + where +
		" ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	queryArgs := append(args, params.Size, offset)

	rows, err := db.Query(querySQL, queryArgs...)
	if err != nil {
		return LogQueryResult{}, fmt.Errorf("usage: query logs: %w", err)
	}
	defer rows.Close()

	items := make([]LogRow, 0, params.Size)
	for rows.Next() {
		var row LogRow
		var ts, requestMetaJSON string
		var failedInt, hasContentInt int
		if err := rows.Scan(
			&row.ID, &ts, &row.APIKey, &row.Model, &row.Source, &row.ChannelName,
			&row.AuthIndex, &failedInt, &row.LatencyMs,
			&row.InputTokens, &row.OutputTokens, &row.ReasoningTokens,
			&row.CachedTokens, &row.TotalTokens, &hasContentInt, &requestMetaJSON,
		); err != nil {
			return LogQueryResult{}, fmt.Errorf("usage: scan row: %w", err)
		}
		row.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		row.Failed = failedInt != 0
		row.HasContent = hasContentInt != 0
		if strings.TrimSpace(requestMetaJSON) != "" {
			var requestMeta map[string]any
			if err := json.Unmarshal([]byte(requestMetaJSON), &requestMeta); err != nil {
				log.Warnf("usage: unmarshal request_meta for log %d: %v", row.ID, err)
			} else if value, ok := requestMeta["reasoning_effort"].(string); ok {
				row.ReasoningEffort = strings.TrimSpace(value)
			}
		}
		items = append(items, row)
	}

	return LogQueryResult{
		Items: items,
		Total: total,
		Page:  params.Page,
		Size:  params.Size,
	}, nil
}

// QueryFilters returns the distinct API keys and models within the time range.
func QueryFilters(days int) (FilterOptions, error) {
	db := getDB()
	if db == nil {
		return FilterOptions{}, nil
	}
	if days < 1 {
		days = 7
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -(days - 1)).Format(time.RFC3339)

	keys, err := queryDistinct(db, "api_key", cutoff)
	if err != nil {
		return FilterOptions{}, err
	}
	models, err := queryDistinct(db, "model", cutoff)
	if err != nil {
		return FilterOptions{}, err
	}

	return FilterOptions{APIKeys: keys, Models: models}, nil
}

// QueryStats returns aggregated statistics over the filtered dataset.
func QueryStats(params LogQueryParams) (LogStats, error) {
	db := getDB()
	if db == nil {
		return LogStats{}, nil
	}
	if params.Days < 1 {
		params.Days = 7
	}

	where, args := buildWhereClause(params)

	var total, successCount, totalTokens int64
	statsSQL := "SELECT COUNT(*), COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END),0), COALESCE(SUM(total_tokens),0) " +
		"FROM request_logs" + where
	if err := db.QueryRow(statsSQL, args...).Scan(&total, &successCount, &totalTokens); err != nil {
		return LogStats{}, fmt.Errorf("usage: stats query: %w", err)
	}

	var successRate float64
	if total > 0 {
		successRate = float64(successCount) / float64(total) * 100
	}

	return LogStats{
		Total:       total,
		SuccessRate: successRate,
		TotalTokens: totalTokens,
	}, nil
}

// QueryUsageSummary returns aggregate usage counters from SQLite.
func QueryUsageSummary() (UsageSummary, error) {
	db := getDB()
	if db == nil {
		return UsageSummary{}, nil
	}

	var total, successCount, failureCount, totalTokens int64
	const sqlText = `
SELECT
	COUNT(*),
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(total_tokens), 0)
FROM request_logs`
	if err := db.QueryRow(sqlText).Scan(&total, &successCount, &failureCount, &totalTokens); err != nil {
		return UsageSummary{}, fmt.Errorf("usage: summary query: %w", err)
	}

	return UsageSummary{
		TotalRequests: total,
		SuccessCount:  successCount,
		FailureCount:  failureCount,
		TotalTokens:   totalTokens,
	}, nil
}

// DashboardSummary aggregates SQLite-backed usage counters for a selected time window.
type DashboardSummary struct {
	TotalRequests   int64
	SuccessRequests int64
	FailedRequests  int64
	SuccessRate     float64
	InputTokens     int64
	OutputTokens    int64
	ReasoningTokens int64
	CachedTokens    int64
	TotalTokens     int64
}

type ModelDistributionPoint struct {
	Model    string `json:"model"`
	Requests int64  `json:"requests"`
	Tokens   int64  `json:"tokens"`
}

type DailyTrendPoint struct {
	Day             string `json:"day"`
	Requests        int64  `json:"requests"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
}

type HourlySeriesPoint struct {
	Hour            string `json:"hour"`
	Model           string `json:"model"`
	Requests        int64  `json:"requests"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
}

type ChannelStatsPoint struct {
	Source          string  `json:"source"`
	Requests        int64   `json:"requests"`
	SuccessRequests int64   `json:"success_requests"`
	FailedRequests  int64   `json:"failed_requests"`
	SuccessRate     float64 `json:"success_rate"`
	LastRequestAt   string  `json:"last_request_at"`
}

type ChannelModelStatsPoint struct {
	Source          string  `json:"source"`
	Model           string  `json:"model"`
	Requests        int64   `json:"requests"`
	SuccessRequests int64   `json:"success_requests"`
	FailedRequests  int64   `json:"failed_requests"`
	SuccessRate     float64 `json:"success_rate"`
	LastRequestAt   string  `json:"last_request_at"`
}

type FailureChannelPoint struct {
	Source         string `json:"source"`
	FailedRequests int64  `json:"failed_requests"`
	LastFailedAt   string `json:"last_failed_at"`
}

type FailureModelPoint struct {
	Source          string  `json:"source"`
	Model           string  `json:"model"`
	Requests        int64   `json:"requests"`
	SuccessRequests int64   `json:"success_requests"`
	FailedRequests  int64   `json:"failed_requests"`
	SuccessRate     float64 `json:"success_rate"`
	LastRequestAt   string  `json:"last_request_at"`
}

type UsageSeriesPoint struct {
	Bucket          string `json:"bucket"`
	Requests        int64  `json:"requests"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	ReasoningTokens int64  `json:"reasoning_tokens"`
	CachedTokens    int64  `json:"cached_tokens"`
	TotalTokens     int64  `json:"total_tokens"`
}

type UsageAPIModelStats struct {
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

type UsageAPIStats struct {
	Endpoint      string               `json:"endpoint"`
	TotalRequests int64                `json:"total_requests"`
	SuccessCount  int64                `json:"success_count"`
	FailureCount  int64                `json:"failure_count"`
	TotalTokens   int64                `json:"total_tokens"`
	Models        []UsageAPIModelStats `json:"models"`
}

type UsageModelStats struct {
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

type UsageCredentialStats struct {
	Source       string `json:"source"`
	AuthIndex    string `json:"auth_index"`
	Requests     int64  `json:"requests"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
}

type UsageCredentialHealthPoint struct {
	AuthIndex    string `json:"auth_index"`
	Source       string `json:"source"`
	Bucket       string `json:"bucket"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
}

type UsageServiceHealthPoint struct {
	Bucket       string `json:"bucket"`
	SuccessCount int64  `json:"success_count"`
	FailureCount int64  `json:"failure_count"`
}

type UsageOverview struct {
	Days           int                       `json:"days"`
	Summary        DashboardSummary          `json:"summary"`
	RequestTrend   []UsageSeriesPoint        `json:"request_trend"`
	TokenBreakdown []UsageSeriesPoint        `json:"token_breakdown"`
	APIs           []UsageAPIStats           `json:"apis"`
	Models         []UsageModelStats         `json:"models"`
	Credentials    []UsageCredentialStats    `json:"credentials"`
	ServiceHealth  []UsageServiceHealthPoint `json:"service_health"`
}

func buildAggregateWhere(days int, apiKey string) (string, []any) {
	if days < 1 {
		days = 7
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -(days - 1)).Format(time.RFC3339)

	clauses := []string{"timestamp >= ?"}
	args := []any{cutoff}
	if strings.TrimSpace(apiKey) != "" {
		clauses = append(clauses, "api_key = ?")
		args = append(args, strings.TrimSpace(apiKey))
	}

	return " WHERE " + strings.Join(clauses, " AND "), args
}

// QueryDashboardSummary returns time-filtered dashboard KPI data from SQLite.
func QueryDashboardSummary(days int, apiKey string) (DashboardSummary, error) {
	db := getDB()
	if db == nil {
		return DashboardSummary{}, nil
	}
	where, args := buildAggregateWhere(days, apiKey)

	sqlText := `
SELECT
	COUNT(*),
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0),
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(reasoning_tokens), 0),
	COALESCE(SUM(cached_tokens), 0),
	COALESCE(SUM(total_tokens), 0)
FROM request_logs
` + where

	var total, successRequests, failedRequests int64
	var inputTokens, outputTokens, reasoningTokens, cachedTokens, totalTokens int64
	if err := db.QueryRow(sqlText, args...).Scan(
		&total,
		&successRequests,
		&failedRequests,
		&inputTokens,
		&outputTokens,
		&reasoningTokens,
		&cachedTokens,
		&totalTokens,
	); err != nil {
		return DashboardSummary{}, fmt.Errorf("usage: dashboard summary query: %w", err)
	}

	var successRate float64
	if total > 0 {
		successRate = float64(successRequests) / float64(total) * 100
	}

	return DashboardSummary{
		TotalRequests:   total,
		SuccessRequests: successRequests,
		FailedRequests:  failedRequests,
		SuccessRate:     successRate,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		ReasoningTokens: reasoningTokens,
		CachedTokens:    cachedTokens,
		TotalTokens:     totalTokens,
	}, nil
}

func QueryModelDistribution(days int, limit int, apiKey string) ([]ModelDistributionPoint, error) {
	db := getDB()
	if db == nil {
		return []ModelDistributionPoint{}, nil
	}
	if limit < 1 {
		limit = 10
	}
	where, args := buildAggregateWhere(days, apiKey)

	query := `
SELECT model, COUNT(*) AS requests, COALESCE(SUM(total_tokens), 0) AS tokens
FROM request_logs` + where + `
GROUP BY model
ORDER BY requests DESC, tokens DESC, model ASC
LIMIT ?`
	args = append(args, limit)
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: model distribution query: %w", err)
	}
	defer rows.Close()

	points := make([]ModelDistributionPoint, 0, limit)
	for rows.Next() {
		var p ModelDistributionPoint
		if err := rows.Scan(&p.Model, &p.Requests, &p.Tokens); err != nil {
			return nil, fmt.Errorf("usage: model distribution scan: %w", err)
		}
		points = append(points, p)
	}
	return points, nil
}

func QueryDailyTrend(days int, apiKey string) ([]DailyTrendPoint, error) {
	db := getDB()
	if db == nil {
		return []DailyTrendPoint{}, nil
	}
	where, args := buildAggregateWhere(days, apiKey)

	query := `
SELECT
	strftime('%Y-%m-%d', timestamp) AS day,
	COUNT(*) AS requests,
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(reasoning_tokens), 0),
	COALESCE(SUM(cached_tokens), 0),
	COALESCE(SUM(total_tokens), 0)
FROM request_logs` + where + `
GROUP BY day
ORDER BY day ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: daily trend query: %w", err)
	}
	defer rows.Close()

	points := make([]DailyTrendPoint, 0, days)
	for rows.Next() {
		var p DailyTrendPoint
		if err := rows.Scan(
			&p.Day,
			&p.Requests,
			&p.InputTokens,
			&p.OutputTokens,
			&p.ReasoningTokens,
			&p.CachedTokens,
			&p.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: daily trend scan: %w", err)
		}
		points = append(points, p)
	}
	return points, nil
}

func QueryHourlySeries(hours int, apiKey string) ([]HourlySeriesPoint, error) {
	db := getDB()
	if db == nil {
		return []HourlySeriesPoint{}, nil
	}
	if hours < 1 {
		hours = 24
	}

	cutoff := time.Now().UTC().Add(-time.Duration(hours-1) * time.Hour).Format(time.RFC3339)
	clauses := []string{"timestamp >= ?"}
	args := []any{cutoff}
	if strings.TrimSpace(apiKey) != "" {
		clauses = append(clauses, "api_key = ?")
		args = append(args, strings.TrimSpace(apiKey))
	}

	query := `
SELECT
	strftime('%Y-%m-%dT%H:00:00Z', timestamp) AS hour,
	model,
	COUNT(*) AS requests,
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(reasoning_tokens), 0),
	COALESCE(SUM(cached_tokens), 0),
	COALESCE(SUM(total_tokens), 0)
FROM request_logs
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY hour, model
ORDER BY hour ASC, model ASC`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: hourly series query: %w", err)
	}
	defer rows.Close()

	points := make([]HourlySeriesPoint, 0, hours)
	for rows.Next() {
		var p HourlySeriesPoint
		if err := rows.Scan(
			&p.Hour,
			&p.Model,
			&p.Requests,
			&p.InputTokens,
			&p.OutputTokens,
			&p.ReasoningTokens,
			&p.CachedTokens,
			&p.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: hourly series scan: %w", err)
		}
		points = append(points, p)
	}
	return points, nil
}

func QueryChannelStats(days int, apiKey string, limit int) ([]ChannelStatsPoint, []ChannelModelStatsPoint, error) {
	db := getDB()
	if db == nil {
		return []ChannelStatsPoint{}, []ChannelModelStatsPoint{}, nil
	}
	if limit < 1 {
		limit = 10
	}
	where, args := buildAggregateWhere(days, apiKey)

	channelQuery := `
SELECT
	source,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_requests,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failed_requests,
	MAX(timestamp) AS last_request_at
FROM request_logs` + where + `
GROUP BY source
HAVING source != ''
ORDER BY requests DESC, last_request_at DESC, source ASC
LIMIT ?`

	channelArgs := append(append([]any{}, args...), limit)
	channelRows, err := db.Query(channelQuery, channelArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("usage: channel stats query: %w", err)
	}
	defer channelRows.Close()

	channels := make([]ChannelStatsPoint, 0, limit)
	sources := make([]string, 0, limit)
	for channelRows.Next() {
		var point ChannelStatsPoint
		if err := channelRows.Scan(
			&point.Source,
			&point.Requests,
			&point.SuccessRequests,
			&point.FailedRequests,
			&point.LastRequestAt,
		); err != nil {
			return nil, nil, fmt.Errorf("usage: channel stats scan: %w", err)
		}
		if point.Requests > 0 {
			point.SuccessRate = float64(point.SuccessRequests) / float64(point.Requests) * 100
		}
		channels = append(channels, point)
		sources = append(sources, point.Source)
	}

	if len(sources) == 0 {
		return channels, []ChannelModelStatsPoint{}, nil
	}

	modelArgs := append([]any{}, args...)
	sourcePlaceholders := make([]string, 0, len(sources))
	for _, source := range sources {
		sourcePlaceholders = append(sourcePlaceholders, "?")
		modelArgs = append(modelArgs, source)
	}

	modelQuery := `
SELECT
	source,
	model,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_requests,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failed_requests,
	MAX(timestamp) AS last_request_at
FROM request_logs` + where + `
AND source IN (` + strings.Join(sourcePlaceholders, ",") + `)
GROUP BY source, model
HAVING model != ''
ORDER BY source ASC, requests DESC, last_request_at DESC, model ASC`

	modelRows, err := db.Query(modelQuery, modelArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("usage: channel model stats query: %w", err)
	}
	defer modelRows.Close()

	models := make([]ChannelModelStatsPoint, 0, len(sources)*4)
	for modelRows.Next() {
		var point ChannelModelStatsPoint
		if err := modelRows.Scan(
			&point.Source,
			&point.Model,
			&point.Requests,
			&point.SuccessRequests,
			&point.FailedRequests,
			&point.LastRequestAt,
		); err != nil {
			return nil, nil, fmt.Errorf("usage: channel model stats scan: %w", err)
		}
		if point.Requests > 0 {
			point.SuccessRate = float64(point.SuccessRequests) / float64(point.Requests) * 100
		}
		models = append(models, point)
	}

	return channels, models, nil
}

func QueryFailureStats(days int, apiKey string, limit int) ([]FailureChannelPoint, []FailureModelPoint, error) {
	db := getDB()
	if db == nil {
		return []FailureChannelPoint{}, []FailureModelPoint{}, nil
	}
	if limit < 1 {
		limit = 10
	}
	where, args := buildAggregateWhere(days, apiKey)

	channelQuery := `
SELECT
	source,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failed_requests,
	MAX(CASE WHEN failed=1 THEN timestamp ELSE NULL END) AS last_failed_at
FROM request_logs` + where + `
GROUP BY source
HAVING source != '' AND failed_requests > 0
ORDER BY failed_requests DESC, last_failed_at DESC, source ASC
LIMIT ?`

	channelArgs := append(append([]any{}, args...), limit)
	channelRows, err := db.Query(channelQuery, channelArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("usage: failure channel stats query: %w", err)
	}
	defer channelRows.Close()

	channels := make([]FailureChannelPoint, 0, limit)
	sources := make([]string, 0, limit)
	for channelRows.Next() {
		var point FailureChannelPoint
		if err := channelRows.Scan(&point.Source, &point.FailedRequests, &point.LastFailedAt); err != nil {
			return nil, nil, fmt.Errorf("usage: failure channel stats scan: %w", err)
		}
		channels = append(channels, point)
		sources = append(sources, point.Source)
	}

	if len(sources) == 0 {
		return channels, []FailureModelPoint{}, nil
	}

	modelArgs := append([]any{}, args...)
	sourcePlaceholders := make([]string, 0, len(sources))
	for _, source := range sources {
		sourcePlaceholders = append(sourcePlaceholders, "?")
		modelArgs = append(modelArgs, source)
	}

	modelQuery := `
SELECT
	source,
	model,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_requests,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failed_requests,
	MAX(timestamp) AS last_request_at
FROM request_logs` + where + `
AND source IN (` + strings.Join(sourcePlaceholders, ",") + `)
GROUP BY source, model
HAVING model != '' AND failed_requests > 0
ORDER BY source ASC, failed_requests DESC, last_request_at DESC, model ASC`

	modelRows, err := db.Query(modelQuery, modelArgs...)
	if err != nil {
		return nil, nil, fmt.Errorf("usage: failure model stats query: %w", err)
	}
	defer modelRows.Close()

	models := make([]FailureModelPoint, 0, len(sources)*4)
	for modelRows.Next() {
		var point FailureModelPoint
		if err := modelRows.Scan(
			&point.Source,
			&point.Model,
			&point.Requests,
			&point.SuccessRequests,
			&point.FailedRequests,
			&point.LastRequestAt,
		); err != nil {
			return nil, nil, fmt.Errorf("usage: failure model stats scan: %w", err)
		}
		if point.Requests > 0 {
			point.SuccessRate = float64(point.SuccessRequests) / float64(point.Requests) * 100
		}
		models = append(models, point)
	}

	return channels, models, nil
}

func QueryUsageRequestTrend(days int, apiKey string) ([]UsageSeriesPoint, error) {
	db := getDB()
	if db == nil {
		return []UsageSeriesPoint{}, nil
	}
	where, args := buildAggregateWhere(days, apiKey)

	query := `
SELECT
	strftime('%Y-%m-%d', timestamp) AS bucket,
	COUNT(*) AS requests,
	COALESCE(SUM(input_tokens), 0),
	COALESCE(SUM(output_tokens), 0),
	COALESCE(SUM(reasoning_tokens), 0),
	COALESCE(SUM(cached_tokens), 0),
	COALESCE(SUM(total_tokens), 0)
FROM request_logs` + where + `
GROUP BY bucket
ORDER BY bucket ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: request trend query: %w", err)
	}
	defer rows.Close()

	points := make([]UsageSeriesPoint, 0, days)
	for rows.Next() {
		var point UsageSeriesPoint
		if err := rows.Scan(
			&point.Bucket,
			&point.Requests,
			&point.InputTokens,
			&point.OutputTokens,
			&point.ReasoningTokens,
			&point.CachedTokens,
			&point.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: request trend scan: %w", err)
		}
		points = append(points, point)
	}
	return points, nil
}

func QueryUsageAPIStats(days int, apiKey string, limit int) ([]UsageAPIStats, error) {
	db := getDB()
	if db == nil {
		return []UsageAPIStats{}, nil
	}
	if limit < 1 {
		limit = 20
	}
	where, args := buildAggregateWhere(days, apiKey)

	apiQuery := `
SELECT
	api_key,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count,
	COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM request_logs` + where + `
GROUP BY api_key
HAVING api_key != ''
ORDER BY requests DESC, total_tokens DESC, api_key ASC
LIMIT ?`

	apiRows, err := db.Query(apiQuery, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("usage: api stats query: %w", err)
	}
	defer apiRows.Close()

	apiStats := make([]UsageAPIStats, 0, limit)
	apiKeys := make([]string, 0, limit)
	for apiRows.Next() {
		var item UsageAPIStats
		if err := apiRows.Scan(
			&item.Endpoint,
			&item.TotalRequests,
			&item.SuccessCount,
			&item.FailureCount,
			&item.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: api stats scan: %w", err)
		}
		item.Models = []UsageAPIModelStats{}
		apiStats = append(apiStats, item)
		apiKeys = append(apiKeys, item.Endpoint)
	}

	if len(apiKeys) == 0 {
		return apiStats, nil
	}

	modelArgs := append([]any{}, args...)
	placeholders := make([]string, 0, len(apiKeys))
	for _, key := range apiKeys {
		placeholders = append(placeholders, "?")
		modelArgs = append(modelArgs, key)
	}

	modelQuery := `
SELECT
	api_key,
	model,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count,
	COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM request_logs` + where + `
AND api_key IN (` + strings.Join(placeholders, ",") + `)
GROUP BY api_key, model
HAVING model != ''
ORDER BY api_key ASC, requests DESC, total_tokens DESC, model ASC`

	modelRows, err := db.Query(modelQuery, modelArgs...)
	if err != nil {
		return nil, fmt.Errorf("usage: api model stats query: %w", err)
	}
	defer modelRows.Close()

	modelMap := make(map[string][]UsageAPIModelStats, len(apiKeys))
	for modelRows.Next() {
		var apiKeyValue string
		var modelItem UsageAPIModelStats
		if err := modelRows.Scan(
			&apiKeyValue,
			&modelItem.Model,
			&modelItem.Requests,
			&modelItem.SuccessCount,
			&modelItem.FailureCount,
			&modelItem.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: api model stats scan: %w", err)
		}
		modelMap[apiKeyValue] = append(modelMap[apiKeyValue], modelItem)
	}

	for i := range apiStats {
		apiStats[i].Models = modelMap[apiStats[i].Endpoint]
		if apiStats[i].Models == nil {
			apiStats[i].Models = []UsageAPIModelStats{}
		}
	}

	return apiStats, nil
}

func QueryUsageModelStats(days int, apiKey string, limit int) ([]UsageModelStats, error) {
	db := getDB()
	if db == nil {
		return []UsageModelStats{}, nil
	}
	if limit < 1 {
		limit = 50
	}
	where, args := buildAggregateWhere(days, apiKey)

	query := `
SELECT
	model,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count,
	COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM request_logs` + where + `
GROUP BY model
HAVING model != ''
ORDER BY requests DESC, total_tokens DESC, model ASC
LIMIT ?`

	rows, err := db.Query(query, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("usage: model stats query: %w", err)
	}
	defer rows.Close()

	items := make([]UsageModelStats, 0, limit)
	for rows.Next() {
		var item UsageModelStats
		if err := rows.Scan(
			&item.Model,
			&item.Requests,
			&item.SuccessCount,
			&item.FailureCount,
			&item.TotalTokens,
		); err != nil {
			return nil, fmt.Errorf("usage: model stats scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func QueryUsageCredentialStats(days int, apiKey string, limit int) ([]UsageCredentialStats, error) {
	db := getDB()
	if db == nil {
		return []UsageCredentialStats{}, nil
	}
	if limit < 1 {
		limit = 200
	}
	where, args := buildAggregateWhere(days, apiKey)

	query := `
SELECT
	source,
	auth_index,
	COUNT(*) AS requests,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count
FROM request_logs` + where + `
GROUP BY source, auth_index
HAVING source != '' OR auth_index != ''
ORDER BY requests DESC, success_count DESC, source ASC, auth_index ASC
LIMIT ?`

	rows, err := db.Query(query, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("usage: credential stats query: %w", err)
	}
	defer rows.Close()

	items := make([]UsageCredentialStats, 0, limit)
	for rows.Next() {
		var item UsageCredentialStats
		if err := rows.Scan(
			&item.Source,
			&item.AuthIndex,
			&item.Requests,
			&item.SuccessCount,
			&item.FailureCount,
		); err != nil {
			return nil, fmt.Errorf("usage: credential stats scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func QueryUsageServiceHealth(days int, apiKey string) ([]UsageServiceHealthPoint, error) {
	db := getDB()
	if db == nil {
		return []UsageServiceHealthPoint{}, nil
	}
	if days < 1 {
		days = 7
	}

	cutoff := time.Now().UTC().Add(-time.Duration(days*24) * time.Hour).Format(time.RFC3339)
	clauses := []string{"timestamp >= ?"}
	args := []any{cutoff}
	if strings.TrimSpace(apiKey) != "" {
		clauses = append(clauses, "api_key = ?")
		args = append(args, strings.TrimSpace(apiKey))
	}

	query := `
SELECT
	strftime('%Y-%m-%dT%H:%M:00Z', datetime((strftime('%s', timestamp) / 900) * 900, 'unixepoch')) AS bucket,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count
FROM request_logs
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY bucket
ORDER BY bucket ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: service health query: %w", err)
	}
	defer rows.Close()

	items := make([]UsageServiceHealthPoint, 0, days*96)
	for rows.Next() {
		var item UsageServiceHealthPoint
		if err := rows.Scan(&item.Bucket, &item.SuccessCount, &item.FailureCount); err != nil {
			return nil, fmt.Errorf("usage: service health scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func QueryUsageCredentialHealth(days int, apiKey string) ([]UsageCredentialHealthPoint, error) {
	db := getDB()
	if db == nil {
		return []UsageCredentialHealthPoint{}, nil
	}
	if days < 1 {
		days = 7
	}

	cutoff := time.Now().UTC().Add(-time.Duration(days*24) * time.Hour).Format(time.RFC3339)
	clauses := []string{"timestamp >= ?", "auth_index != ''"}
	args := []any{cutoff}
	if strings.TrimSpace(apiKey) != "" {
		clauses = append(clauses, "api_key = ?")
		args = append(args, strings.TrimSpace(apiKey))
	}

	query := `
SELECT
	auth_index,
	source,
	strftime('%Y-%m-%dT%H:%M:00Z', datetime((strftime('%s', timestamp) / 600) * 600, 'unixepoch')) AS bucket,
	COALESCE(SUM(CASE WHEN failed=0 THEN 1 ELSE 0 END), 0) AS success_count,
	COALESCE(SUM(CASE WHEN failed=1 THEN 1 ELSE 0 END), 0) AS failure_count
FROM request_logs
WHERE ` + strings.Join(clauses, " AND ") + `
GROUP BY auth_index, source, bucket
ORDER BY auth_index ASC, bucket ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("usage: credential health query: %w", err)
	}
	defer rows.Close()

	items := make([]UsageCredentialHealthPoint, 0, days*96)
	for rows.Next() {
		var item UsageCredentialHealthPoint
		if err := rows.Scan(
			&item.AuthIndex,
			&item.Source,
			&item.Bucket,
			&item.SuccessCount,
			&item.FailureCount,
		); err != nil {
			return nil, fmt.Errorf("usage: credential health scan: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func QueryUsageOverview(days int, apiKey string) (UsageOverview, error) {
	summary, err := QueryDashboardSummary(days, apiKey)
	if err != nil {
		return UsageOverview{}, err
	}
	requestTrend, err := QueryUsageRequestTrend(days, apiKey)
	if err != nil {
		return UsageOverview{}, err
	}
	apiStats, err := QueryUsageAPIStats(days, apiKey, 20)
	if err != nil {
		return UsageOverview{}, err
	}
	modelStats, err := QueryUsageModelStats(days, apiKey, 100)
	if err != nil {
		return UsageOverview{}, err
	}
	credentialStats, err := QueryUsageCredentialStats(days, apiKey, 500)
	if err != nil {
		return UsageOverview{}, err
	}
	serviceHealth, err := QueryUsageServiceHealth(days, apiKey)
	if err != nil {
		return UsageOverview{}, err
	}

	return UsageOverview{
		Days:           days,
		Summary:        summary,
		RequestTrend:   requestTrend,
		TokenBreakdown: requestTrend,
		APIs:           apiStats,
		Models:         modelStats,
		Credentials:    credentialStats,
		ServiceHealth:  serviceHealth,
	}, nil
}

// MigrateFromSnapshot imports all request details from an existing
// StatisticsSnapshot into SQLite. It skips rows that already exist
// (based on a count check to avoid duplicating on restart).
func MigrateFromSnapshot(snapshot StatisticsSnapshot) (int64, error) {
	db := getDB()
	if db == nil {
		return 0, nil
	}

	// Check if data already exists
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM request_logs").Scan(&count); err != nil {
		return 0, fmt.Errorf("usage: migration count: %w", err)
	}
	if count > 0 {
		log.Infof("usage: SQLite already has %d rows, skipping migration", count)
		return 0, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("usage: begin migration tx: %w", err)
	}

	stmt, err := tx.Prepare(`INSERT INTO request_logs
		(timestamp, api_key, model, source, channel_name, auth_index,
		 failed, latency_ms, input_tokens, output_tokens, reasoning_tokens, cached_tokens, total_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		return 0, fmt.Errorf("usage: prepare migration stmt: %w", err)
	}
	defer stmt.Close()

	var imported int64
	for apiKey, apiData := range snapshot.APIs {
		for model, modelData := range apiData.Models {
			for _, detail := range modelData.Details {
				failedInt := 0
				if detail.Failed {
					failedInt = 1
				}
				_, err := stmt.Exec(
					detail.Timestamp.UTC().Format(time.RFC3339Nano),
					apiKey, model, detail.Source, detail.ChannelName, detail.AuthIndex,
					failedInt, detail.LatencyMs,
					detail.Tokens.InputTokens, detail.Tokens.OutputTokens,
					detail.Tokens.ReasoningTokens, detail.Tokens.CachedTokens,
					detail.Tokens.TotalTokens,
				)
				if err != nil {
					_ = tx.Rollback()
					return imported, fmt.Errorf("usage: migration insert: %w", err)
				}
				imported++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return imported, fmt.Errorf("usage: commit migration: %w", err)
	}

	log.Infof("usage: migrated %d request logs from snapshot to SQLite", imported)
	return imported, nil
}

// --- internal helpers ---

func getDB() *sql.DB {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()
	return usageDB
}

func buildWhereClause(params LogQueryParams) (string, []interface{}) {
	conditions := make([]string, 0, 4)
	args := make([]interface{}, 0, 4)

	// Time range: days=1 means "today", days=7 means "last 7 days", etc.
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -(params.Days - 1))
	conditions = append(conditions, "timestamp >= ?")
	args = append(args, cutoff.Format(time.RFC3339))

	if params.APIKey != "" {
		conditions = append(conditions, "api_key = ?")
		args = append(args, params.APIKey)
	}
	if params.Model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, params.Model)
	}
	if params.Status == "success" {
		conditions = append(conditions, "failed = 0")
	} else if params.Status == "failed" {
		conditions = append(conditions, "failed = 1")
	}

	if len(conditions) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func queryDistinct(db *sql.DB, column, cutoff string) ([]string, error) {
	q := fmt.Sprintf("SELECT DISTINCT %s FROM request_logs WHERE timestamp >= ? ORDER BY %s", column, column)
	rows, err := db.Query(q, cutoff)
	if err != nil {
		return nil, fmt.Errorf("usage: distinct %s: %w", column, err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v != "" {
			result = append(result, v)
		}
	}
	return result, nil
}

// QueryModelsForKey returns the distinct models used by a specific API key within the time range.
func QueryModelsForKey(apiKey string, days int) ([]string, error) {
	db := getDB()
	if db == nil {
		return nil, nil
	}
	if days < 1 {
		days = 7
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	cutoff := today.AddDate(0, 0, -(days - 1)).Format(time.RFC3339)

	rows, err := db.Query(
		"SELECT DISTINCT model FROM request_logs WHERE api_key = ? AND timestamp >= ? AND model != '' ORDER BY model",
		apiKey, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("usage: distinct models for key: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

// LogContentResult holds the content detail for a single log entry.
type LogContentResult struct {
	ID              int64          `json:"id"`
	InputContent    string         `json:"input_content"`
	OutputContent   string         `json:"output_content"`
	Model           string         `json:"model"`
	HasContent      bool           `json:"has_content"`
	ContentDisabled bool           `json:"content_disabled"`
	RequestMeta     map[string]any `json:"request_meta,omitempty"`
}

// QueryLogContent retrieves the stored request/response content for a single log entry.
func QueryLogContent(id int64) (LogContentResult, error) {
	db := getDB()
	if db == nil {
		return LogContentResult{}, fmt.Errorf("usage: database not initialised")
	}

	var result LogContentResult
	var requestMetaJSON string
	err := db.QueryRow(
		"SELECT id, model, input_content, output_content, request_meta FROM request_logs WHERE id = ?", id,
	).Scan(&result.ID, &result.Model, &result.InputContent, &result.OutputContent, &requestMetaJSON)
	if err != nil {
		return LogContentResult{}, fmt.Errorf("usage: query log content: %w", err)
	}
	result.HasContent = result.InputContent != "" || result.OutputContent != ""
	if strings.TrimSpace(requestMetaJSON) != "" {
		if err := json.Unmarshal([]byte(requestMetaJSON), &result.RequestMeta); err != nil {
			log.Warnf("usage: unmarshal request_meta for log %d: %v", id, err)
		}
	}
	return result, nil
}

// GetDBPath returns the file path of the SQLite database, or empty if not initialised.
func GetDBPath() string {
	usageDBMu.Lock()
	defer usageDBMu.Unlock()
	return usageDBPath
}

// ChannelLatency holds the average latency stats for a single channel (source).
type ChannelLatency struct {
	Source string  `json:"source"`
	Count  int64   `json:"count"`
	AvgMs  float64 `json:"avg_ms"`
}

// GetChannelAvgLatency returns average request latency grouped by source (channel)
// for the last N days.
func GetChannelAvgLatency(days int) ([]ChannelLatency, error) {
	db := getDB()
	if db == nil {
		return nil, fmt.Errorf("usage: database not initialised")
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	rows, err := db.Query(`
		SELECT source, COUNT(*) as cnt, AVG(latency_ms) as avg_lat
		FROM request_logs
		WHERE timestamp > ? AND source != ''
		GROUP BY source
		ORDER BY avg_lat DESC
		LIMIT 5
	`, cutoff.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("usage: query channel latency: %w", err)
	}
	defer rows.Close()

	var result []ChannelLatency
	for rows.Next() {
		var cl ChannelLatency
		if err := rows.Scan(&cl.Source, &cl.Count, &cl.AvgMs); err != nil {
			return nil, fmt.Errorf("usage: scan channel latency: %w", err)
		}
		result = append(result, cl)
	}
	return result, rows.Err()
}
