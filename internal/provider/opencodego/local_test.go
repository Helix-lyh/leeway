package opencodego

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// buildTestDB creates a synthetic opencode.db. When withParts is true a part
// table is added and step-finish part costs override message costs.
func buildTestDB(t *testing.T, dir string, withParts bool, now time.Time) string {
	t.Helper()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mustExec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE message (
		id TEXT PRIMARY KEY,
		time_created INTEGER,
		data TEXT
	)`)
	if withParts {
		mustExec(`CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT,
			time_created INTEGER,
			data TEXT
		)`)
	}

	msg := func(id string, providerID, role string, cost any, createdMs int64, modelID string) {
		costJSON := "null"
		if cost != nil {
			switch c := cost.(type) {
			case int:
				costJSON = fmt.Sprintf("%d", c)
			case float64:
				costJSON = fmt.Sprintf("%v", c)
			case string:
				costJSON = `"` + c + `"`
			}
		}
		modelJSON := "null"
		if modelID != "" {
			modelJSON = `"` + modelID + `"`
		}
		data := fmt.Sprintf(`{"providerID": %q, "role": %q, "cost": %s, "modelID": %s, "time": {"created": %d}}`,
			providerID, role, costJSON, modelJSON, createdMs)
		mustExec(`INSERT INTO message (id, time_created, data) VALUES (?, ?, ?)`, id, createdMs, data)
	}
	part := func(id, messageID, partType string, cost *float64, createdMs int64) {
		costJSON := "null"
		if cost != nil {
			costJSON = fmt.Sprintf("%v", *cost)
		}
		data := fmt.Sprintf(`{"type": %q, "cost": %s, "time": {"created": %d}}`, partType, costJSON, createdMs)
		mustExec(`INSERT INTO part (id, message_id, time_created, data) VALUES (?, ?, ?, ?)`, id, messageID, createdMs, data)
	}

	nowMs := now.UnixMilli()
	yesterdayMs := now.Add(-26 * time.Hour).UnixMilli()

	// Today: opencode-go assistant rows.
	msg("m1", "opencode-go", "assistant", 1.25, nowMs-3600_000, "gpt-4o")
	msg("m2", "opencode-go", "assistant", 2, nowMs-1800_000, "")
	// Yesterday: excluded from the latest-day bucket.
	msg("m3", "opencode-go", "assistant", 10, yesterdayMs, "gpt-4o")
	// Wrong provider / wrong role: excluded by SQL.
	msg("m4", "opencode", "assistant", 99, nowMs-1000, "other")
	msg("m5", "opencode-go", "user", 5, nowMs-1000, "")
	// Non-numeric cost: excluded by the json_type filter.
	msg("m6", "opencode-go", "assistant", "notanumber", nowMs-900_000, "")
	// No cost at all: excluded.
	msg("m7", "opencode-go", "assistant", nil, nowMs-800_000, "")
	// Null timestamp: row guard skips it.
	msg("m8", "opencode-go", "assistant", 50, 0, "")

	if withParts {
		// step-finish part with cost overrides m1's message cost.
		part("p1", "m1", "step-finish", f64ptr(0.75), nowMs-3500_000)
		// step-finish part without cost: message cost stays in play.
		part("p2", "m2", "step-finish", nil, nowMs-1700_000)
		// Non-step-finish part with cost: ignored (m3's message cost counts).
		part("p3", "m3", "text-edit", f64ptr(7), yesterdayMs)
	}
	return dbPath
}

func TestFetchLocalCostMessageOnly(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	dbPath := buildTestDB(t, t.TempDir(), false, now)

	reader := &localCostReader{dbPath: dbPath, now: func() time.Time { return now }}
	cost, err := reader.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if cost == nil {
		t.Fatalf("expected a cost")
	}
	if cost.Period != "local history" || cost.Currency != "USD" {
		t.Fatalf("cost metadata wrong: %+v", cost)
	}
	// Today's opencode-go assistant rows: m1 (1.25) + m2 (2) = 3.25.
	if *cost.Amount != 3.25 {
		t.Fatalf("amount = %v, want 3.25", *cost.Amount)
	}
}

func TestFetchLocalCostWithParts(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	dbPath := buildTestDB(t, t.TempDir(), true, now)

	reader := &localCostReader{dbPath: dbPath, now: func() time.Time { return now }}
	cost, err := reader.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// m1 -> step-finish part 0.75; m2 -> no valid step-finish cost, message
	// cost 2 counts: total 2.75.
	if cost == nil || *cost.Amount != 2.75 {
		t.Fatalf("amount = %v, want 2.75", cost)
	}
}

func TestFetchLocalCostLatestDayWins(t *testing.T) {
	// Rows on two different days: only the most recent day bucket is summed.
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, time_created INTEGER, data TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	insert := func(id string, cost float64, createdMs int64) {
		data := fmt.Sprintf(`{"providerID": "opencode-go", "role": "assistant", "cost": %v, "time": {"created": %d}}`, cost, createdMs)
		if _, err := db.Exec(`INSERT INTO message (id, time_created, data) VALUES (?, ?, ?)`, id, createdMs, data); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("old1", 1, now.Add(-50*time.Hour).UnixMilli())    // Aug 4
	insert("old2", 2, now.Add(-50*time.Hour).UnixMilli())    // Aug 4
	insert("recent", 10, now.Add(-26*time.Hour).UnixMilli()) // Aug 5

	reader := &localCostReader{dbPath: dbPath, now: func() time.Time { return now }}
	cost, err := reader.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if cost == nil || *cost.Amount != 10 {
		t.Fatalf("latest-day semantics wrong: amount = %v, want 10", cost)
	}
}

func TestFetchLocalCostMissingDB(t *testing.T) {
	reader := &localCostReader{
		dbPath: filepath.Join(t.TempDir(), "nope.db"),
		now:    time.Now,
	}
	cost, err := reader.fetch(context.Background())
	if err != nil {
		t.Fatalf("missing DB must not error: %v", err)
	}
	if cost != nil {
		t.Fatalf("missing DB must yield nil cost, got %+v", cost)
	}
}

func TestFetchLocalCostEmptyDB(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, time_created INTEGER, data TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	db.Close()

	reader := &localCostReader{dbPath: dbPath, now: func() time.Time { return now }}
	cost, err := reader.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if cost != nil {
		t.Fatalf("empty history must yield nil cost, got %+v", cost)
	}
}

// TestFetchLocalCostDoesNotModifyDB verifies the read-only contract: the file
// bytes are untouched and no -wal/-shm sidecars appear.
func TestFetchLocalCostDoesNotModifyDB(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	dbPath := buildTestDB(t, dir, false, now)

	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	reader := &localCostReader{dbPath: dbPath, now: func() time.Time { return now }}
	if _, err := reader.fetch(context.Background()); err != nil {
		t.Fatalf("fetch: %v", err)
	}

	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if len(before) != len(after) {
		t.Fatalf("db bytes changed: %d -> %d", len(before), len(after))
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if _, err := os.Stat(sidecar); err == nil {
			t.Fatalf("sidecar %s was created", sidecar)
		}
	}
}

func TestLatestDayCost(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	rows := []usageRow{
		{createdMs: now.UnixMilli() - 60_000, cost: 1},
		{createdMs: now.UnixMilli() - 120_000, cost: 2},
		{createdMs: now.Add(-26 * time.Hour).UnixMilli(), cost: 10},
	}
	got := latestDayCost(rows, now)
	if got == nil || *got != 3 {
		t.Fatalf("latest day = %v, want 3", got)
	}
	if got := latestDayCost(nil, now); got != nil {
		t.Fatalf("empty rows must yield nil, got %v", *got)
	}
}

func TestLocalReaderRejectsBadDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	if err := os.WriteFile(dbPath, []byte("not a sqlite file at all"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	reader := &localCostReader{dbPath: dbPath, now: time.Now}
	cost, err := reader.fetch(context.Background())
	if err == nil {
		t.Fatalf("corrupt DB must error (cost=%+v)", cost)
	}
	if !strings.Contains(err.Error(), "file is not a database") && !strings.Contains(err.Error(), "SQLITE") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func f64ptr(v float64) *float64 { return &v }
