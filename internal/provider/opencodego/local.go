package opencodego

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"

	"leeway/internal/model"

	_ "modernc.org/sqlite"
)

// Local usage history mirrors OpenCodeGoLocalUsageReader: it reads the
// opencode CLI database (~/.local/share/opencode/opencode.db) read-only and
// sums assistant message costs attributed to the 'opencode-go' provider.
//
// The period semantics mirror the Swift daily cost buckets: rows are grouped
// into device-local calendar days and the most recent day with any data is
// reported as the machine-level local cost.
type localCostReader struct {
	dbPath string
	now    func() time.Time
}

func defaultLocalDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "opencode", "opencode.db")
}

// messageUsageSQL mirrors the Swift messageUsageSQL (databases without a
// part table).
const messageUsageSQL = `
SELECT
  CAST(COALESCE(json_extract(data, '$.time.created'), time_created) AS INTEGER) AS createdMs,
  CAST(json_extract(data, '$.cost') AS REAL) AS cost,
  1 AS requestCount,
  COALESCE(json_extract(data, '$.modelID'), '') AS modelID
FROM message
WHERE json_valid(data)
  AND json_extract(data, '$.providerID') = 'opencode-go'
  AND json_extract(data, '$.role') = 'assistant'
  AND json_type(data, '$.cost') IN ('integer', 'real')
`

// messageAndPartUsageSQL mirrors the Swift messageAndPartUsageSQL (databases
// with a part table): step-finish part costs win; message costs are only used
// when no step-finish part carries one.
const messageAndPartUsageSQL = `
WITH provider_messages AS (
  SELECT
    id AS messageID,
    CAST(COALESCE(json_extract(data, '$.time.created'), time_created) AS INTEGER) AS createdMs,
    CAST(json_extract(data, '$.cost') AS REAL) AS cost,
    json_type(data, '$.cost') IN ('integer', 'real') AS hasCost,
    COALESCE(json_extract(data, '$.modelID'), '') AS modelID
  FROM message
  WHERE json_valid(data)
    AND json_extract(data, '$.providerID') = 'opencode-go'
    AND json_extract(data, '$.role') = 'assistant'
)
SELECT
  CAST(COALESCE(json_extract(p.data, '$.time.created'), p.time_created, m.createdMs) AS INTEGER)
    AS createdMs,
  CAST(json_extract(p.data, '$.cost') AS REAL) AS cost,
  1 AS requestCount,
  m.modelID AS modelID
FROM part p
JOIN provider_messages m ON m.messageID = p.message_id
WHERE json_valid(p.data)
  AND json_extract(p.data, '$.type') = 'step-finish'
  AND json_type(p.data, '$.cost') IN ('integer', 'real')
UNION ALL
SELECT createdMs, cost, 1 AS requestCount, modelID
FROM provider_messages m
WHERE hasCost
  AND NOT EXISTS (
    SELECT 1
    FROM part p
    WHERE p.message_id = m.messageID
      AND json_valid(p.data)
      AND json_extract(p.data, '$.type') = 'step-finish'
      AND json_type(p.data, '$.cost') IN ('integer', 'real')
  )
`

type usageRow struct {
	createdMs int64
	cost      float64
}

// fetch returns the latest calendar-day cost from the local database. A
// missing database, empty history, or unreadable file yields (nil, nil): the
// orchestrator leaves LocalCost unset, which is the "no local history" note.
func (r *localCostReader) fetch(ctx context.Context) (*model.Cost, error) {
	if r.dbPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(r.dbPath); err != nil {
		return nil, nil // database not found → no local cost (not an error)
	}
	rows, err := r.readRows(ctx)
	if err != nil {
		return nil, err
	}
	cost := latestDayCost(rows, r.now())
	if cost == nil {
		return nil, nil
	}
	return &model.Cost{Period: "local history", Currency: "USD", Amount: cost}, nil
}

// readRows mirrors readRows(immutable:) in the Swift reader: it opens
// read-only, and retries with immutable=1 only when a CANTOPEN error leaves a
// WAL-mode database without its sidecars. A plain read-only open still reads
// uncheckpointed WAL data; immutable does not, so it stays the fallback.
func (r *localCostReader) readRows(ctx context.Context) ([]usageRow, error) {
	db, err := r.openDB("file:" + filepath.ToSlash(r.dbPath) + "?mode=ro")
	if err != nil {
		if !isCantOpen(err) || r.walSidecarsPresent() {
			return nil, err
		}
		db, err = r.openDB("file:" + filepath.ToSlash(r.dbPath) + "?mode=ro&immutable=1")
		if err != nil {
			return nil, err
		}
	}
	defer db.Close()

	hasPart, err := hasTable(db, "part")
	if err != nil {
		return nil, err
	}
	query := messageUsageSQL
	if hasPart {
		query = messageAndPartUsageSQL
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rs, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	var rows []usageRow
	for rs.Next() {
		var createdMs sql.NullInt64
		var cost sql.NullFloat64
		var requestCount int64
		var model string
		if err := rs.Scan(&createdMs, &cost, &requestCount, &model); err != nil {
			return nil, err
		}
		// Mirrors the Swift row guard: NULL/zero timestamps and negative or
		// non-finite costs are skipped (sqlite3_column_int64 yields 0 for
		// NULL, and the Swift loop skips such rows).
		if !createdMs.Valid || createdMs.Int64 <= 0 || !cost.Valid ||
			cost.Float64 < 0 || math.IsInf(cost.Float64, 0) || math.IsNaN(cost.Float64) {
			continue
		}
		rows = append(rows, usageRow{createdMs: createdMs.Int64, cost: cost.Float64})
	}
	return rows, rs.Err()
}

func (r *localCostReader) openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn+"&_pragma=busy_timeout(250)")
	if err != nil {
		return nil, err
	}
	return db, nil
}

func (r *localCostReader) walSidecarsPresent() bool {
	return fileExists(r.dbPath+"-wal") || fileExists(r.dbPath+"-shm")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasTable(db *sql.DB, name string) (bool, error) {
	var found int
	err := db.QueryRow(
		"SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ? LIMIT 1",
		name).Scan(&found)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return found == 1, nil
}

func isCantOpen(err error) bool {
	var sqliteErr interface{ Code() int }
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == 14 // SQLITE_CANTOPEN
	}
	return false
}

// latestDayCost sums the most recent calendar-day bucket in device-local time
// (mirrors the Swift dailyEntries bucket semantics). Returns nil when there
// are no rows.
func latestDayCost(rows []usageRow, now time.Time) *float64 {
	// Day key is the local calendar date of the row.
	type dayKey struct {
		y, m, d int
	}
	dayOf := func(t time.Time) dayKey {
		local := t.In(time.Local)
		return dayKey{local.Year(), int(local.Month()), local.Day()}
	}
	totals := map[dayKey]float64{}
	var latest dayKey
	var haveLatest bool
	for _, row := range rows {
		t := time.UnixMilli(row.createdMs)
		key := dayOf(t)
		totals[key] += row.cost
		if !haveLatest || key.y > latest.y || (key.y == latest.y && (key.m > latest.m || (key.m == latest.m && key.d > latest.d))) {
			latest = key
			haveLatest = true
		}
	}
	if !haveLatest {
		return nil
	}
	v := totals[latest]
	return &v
}
