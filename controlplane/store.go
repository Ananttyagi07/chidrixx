// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists ingested findings in the schema from the build manual's
// §11.2 (flow_aggregate), in pure-Go SQLite — no cgo, matching the
// manual's explicit choice, and enough for a single-binary control plane
// before this ever needs to become ClickHouse+Postgres.
type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// SQLite handles one writer at a time; a single connection avoids
	// "database is locked" errors under concurrent ingest.
	db.SetMaxOpenConns(1)

	const schema = `
CREATE TABLE IF NOT EXISTS flow_aggregate (
	id                       INTEGER PRIMARY KEY AUTOINCREMENT,
	cluster_id               TEXT NOT NULL,
	reported_at              INTEGER NOT NULL,
	src_workload             TEXT NOT NULL,
	dst_workload_or_endpoint TEXT NOT NULL,
	path_class               TEXT NOT NULL,
	confidence               TEXT NOT NULL,
	bytes_tx                 INTEGER NOT NULL,
	bytes_rx                 INTEGER NOT NULL,
	cost_low_inr             REAL NOT NULL,
	cost_high_inr            REAL NOT NULL,
	fix_hint                 TEXT,
	fix_manifest             TEXT
);
CREATE INDEX IF NOT EXISTS idx_flow_aggregate_cluster_time ON flow_aggregate(cluster_id, reported_at);
`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// fix_manifest was added after this table's original release;
	// CREATE TABLE IF NOT EXISTS above is a no-op against a database that
	// already has the table without it, so add the column explicitly for
	// databases created before this change. SQLite has no
	// "ADD COLUMN IF NOT EXISTS," so ignore the one error that means it's
	// already there.
	if _, err := db.Exec(`ALTER TABLE flow_aggregate ADD COLUMN fix_manifest TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("migrate fix_manifest column: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Ingest records one full snapshot from a cluster's agent as a batch of
// rows sharing one reported_at timestamp.
func (s *Store) Ingest(clusterID string, findings []Finding) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	reportedAt := time.Now().Unix()

	stmt, err := tx.Prepare(`
		INSERT INTO flow_aggregate
			(cluster_id, reported_at, src_workload, dst_workload_or_endpoint,
			 path_class, confidence, bytes_tx, bytes_rx, cost_low_inr, cost_high_inr, fix_hint, fix_manifest)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, f := range findings {
		if _, err := stmt.Exec(
			clusterID, reportedAt, f.Source, f.Destination,
			f.PathClass, f.Confidence, f.BytesTx, f.BytesRx,
			f.CostLowINR, f.CostHighINR, f.FixHint, f.FixManifest,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert finding: %w", err)
		}
	}

	return tx.Commit()
}

// ClusterSummary describes one cluster's most recent snapshot.
type ClusterSummary struct {
	ClusterID    string
	LastSeen     time.Time
	FindingCount int
	CostLowINR   float64
	CostHighINR  float64
}

// Clusters returns every distinct cluster that has ever ingested, with its
// most recent snapshot's totals.
func (s *Store) Clusters() ([]ClusterSummary, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			GROUP BY cluster_id
		)
		SELECT fa.cluster_id, l.max_time, COUNT(*), COALESCE(SUM(fa.cost_low_inr), 0), COALESCE(SUM(fa.cost_high_inr), 0)
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		GROUP BY fa.cluster_id, l.max_time
		ORDER BY l.max_time DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var out []ClusterSummary
	for rows.Next() {
		var c ClusterSummary
		var lastSeen int64
		if err := rows.Scan(&c.ClusterID, &lastSeen, &c.FindingCount, &c.CostLowINR, &c.CostHighINR); err != nil {
			return nil, fmt.Errorf("scan cluster row: %w", err)
		}
		c.LastSeen = time.Unix(lastSeen, 0)
		out = append(out, c)
	}

	return out, rows.Err()
}

// FindingRow is a stored finding plus which cluster and when it was seen.
type FindingRow struct {
	Finding
	ClusterID  string
	ReportedAt time.Time
}

// LatestFindings returns the current state across every cluster — each
// cluster's most recent snapshot, unioned and ranked by cost — capped at
// limit rows. This is the "multi-cluster view" (FR-C1).
func (s *Store) LatestFindings(limit int) ([]FindingRow, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			GROUP BY cluster_id
		)
		SELECT fa.cluster_id, fa.reported_at, fa.src_workload, fa.dst_workload_or_endpoint,
		       fa.path_class, fa.confidence, fa.bytes_tx, fa.bytes_rx,
		       fa.cost_low_inr, fa.cost_high_inr, fa.fix_hint, fa.fix_manifest
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		ORDER BY fa.cost_high_inr DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query latest findings: %w", err)
	}
	defer rows.Close()

	var out []FindingRow
	for rows.Next() {
		var r FindingRow
		var reportedAt int64
		var fixHint, fixManifest sql.NullString

		if err := rows.Scan(
			&r.ClusterID, &reportedAt, &r.Source, &r.Destination,
			&r.PathClass, &r.Confidence, &r.BytesTx, &r.BytesRx,
			&r.CostLowINR, &r.CostHighINR, &fixHint, &fixManifest,
		); err != nil {
			return nil, fmt.Errorf("scan finding row: %w", err)
		}

		r.ReportedAt = time.Unix(reportedAt, 0)
		r.FixHint = fixHint.String
		r.FixManifest = fixManifest.String
		out = append(out, r)
	}

	return out, rows.Err()
}

// CostTrendPoint is one snapshot's total cost for a cluster, for a
// sparkline.
type CostTrendPoint struct {
	ReportedAt time.Time
	CostHigh   float64
}

// CostTrend returns the total estimated cost at each snapshot time for one
// cluster, oldest first — the series a trend sparkline plots.
func (s *Store) CostTrend(clusterID string, maxPoints int) ([]CostTrendPoint, error) {
	rows, err := s.db.Query(`
		SELECT reported_at, SUM(cost_high_inr)
		FROM flow_aggregate
		WHERE cluster_id = ?
		GROUP BY reported_at
		ORDER BY reported_at DESC
		LIMIT ?
	`, clusterID, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("query cost trend: %w", err)
	}
	defer rows.Close()

	var out []CostTrendPoint
	for rows.Next() {
		var p CostTrendPoint
		var reportedAt int64
		if err := rows.Scan(&reportedAt, &p.CostHigh); err != nil {
			return nil, fmt.Errorf("scan trend point: %w", err)
		}
		p.ReportedAt = time.Unix(reportedAt, 0)
		out = append(out, p)
	}

	// oldest first, for plotting left-to-right
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}

	return out, rows.Err()
}
