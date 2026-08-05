// SPDX-License-Identifier: Apache-2.0
package main

import (
	"database/sql"
	"fmt"
	"strconv"
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
	// Pragmas are applied via DSN (modernc.org/sqlite runs them on every
	// new connection the pool opens, not just the first) rather than a
	// one-time db.Exec after Open -- that distinction matters now that
	// MaxOpenConns is >1 (see below): synchronous and busy_timeout are
	// per-connection settings, so every connection needs them, not just
	// whichever one happened to be open first.
	//
	// Found necessary against real production data, not theoretical:
	// default SQLite settings (rollback-journal mode, synchronous=FULL,
	// one shared connection) measured escalating to 20-40+ real seconds
	// per request on a live deployment once flow_aggregate (never
	// pruned, by design) grew past ~500MB/2M rows -- confirmed via a
	// trivial session-lookup endpoint (touching none of that table)
	// still taking 4+ real seconds, proving it was connection-level
	// queuing (every request serialized through one connection), not
	// query cost (the same real query ran in under a second directly
	// against a copy of the same data). WAL mode plus a small reader
	// pool is the standard, well-established fix for exactly this shape
	// of workload (one writer, frequent small commits, many concurrent
	// readers) -- WAL specifically exists so readers don't block behind
	// the writer or each other.
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// SQLite still allows only one writer at a time regardless of journal
	// mode -- busy_timeout above makes a second would-be writer wait
	// briefly instead of instantly erroring. This pool size is for
	// concurrent readers (dashboard polls, multiple browser tabs), which
	// WAL mode makes safe without blocking on the writer.
	//
	// ":memory:" is the one real exception: a bare in-memory DSN gives
	// every new pooled connection its OWN independent, empty database
	// (no shared backing file for WAL to coordinate through) -- confirmed
	// directly, not assumed: a throwaway test opening 4 connections
	// against ":memory:" with MaxOpenConns(4) found connection 0 saw a
	// real inserted row and connections 1-3 each got "no such table."
	// Every real (non-test) call site here always passes a real file
	// path, so this only ever matters for the test suite's in-memory
	// helper -- and it must stay pinned at 1 connection to remain
	// correct, not just because it happened to pass so far.
	if path != ":memory:" {
		db.SetMaxOpenConns(4)
	}

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
	fix_manifest             TEXT,
	cloud                    TEXT,
	region                   TEXT,
	savings_low_inr          REAL,
	savings_high_inr         REAL
);
CREATE INDEX IF NOT EXISTS idx_flow_aggregate_cluster_time ON flow_aggregate(cluster_id, reported_at);

-- flow_aggregate_daily is the compacted cold tier (see compaction.go): one
-- real row per (tenant, cluster, day, workload pair) instead of one row
-- per real ingest cycle, for raw rows old enough to have aged out of the
-- retention window. sample_count records how many real raw rows were
-- folded into it, for honest transparency about granularity lost.
CREATE TABLE IF NOT EXISTS flow_aggregate_daily (
	id                       INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id                INTEGER NOT NULL,
	cluster_id               TEXT NOT NULL,
	day                      INTEGER NOT NULL,
	src_workload             TEXT NOT NULL,
	dst_workload_or_endpoint TEXT NOT NULL,
	path_class               TEXT NOT NULL,
	bytes_tx                 INTEGER NOT NULL,
	bytes_rx                 INTEGER NOT NULL,
	cost_low_inr             REAL NOT NULL,
	cost_high_inr            REAL NOT NULL,
	savings_low_inr          REAL NOT NULL DEFAULT 0,
	savings_high_inr         REAL NOT NULL DEFAULT 0,
	sample_count             INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_flow_aggregate_daily_key
	ON flow_aggregate_daily(tenant_id, cluster_id, day, src_workload, dst_workload_or_endpoint, path_class);

-- ai_eval_events is real observability for the AI features themselves
-- (see aieval.go): one row per real chat turn or anomaly-narration call,
-- capturing what actually happened -- latency, whether it succeeded,
-- how many tool calls it made and how many of those errored, real token
-- usage, and whether it had to give up at the round limit instead of
-- converging. Not customer-facing telemetry -- this answers "is the AI
-- actually working," a real gap this project had zero visibility into
-- before.
CREATE TABLE IF NOT EXISTS ai_eval_events (
	id                INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id         INTEGER NOT NULL,
	feature           TEXT NOT NULL,
	occurred_at       INTEGER NOT NULL,
	latency_ms        INTEGER NOT NULL,
	success           INTEGER NOT NULL,
	hit_round_limit   INTEGER NOT NULL DEFAULT 0,
	rounds            INTEGER NOT NULL DEFAULT 0,
	tool_call_count   INTEGER NOT NULL DEFAULT 0,
	tool_call_errors  INTEGER NOT NULL DEFAULT 0,
	prompt_tokens     INTEGER NOT NULL DEFAULT 0,
	completion_tokens INTEGER NOT NULL DEFAULT 0,
	error_message     TEXT
);
CREATE INDEX IF NOT EXISTS idx_ai_eval_events_tenant_feature ON ai_eval_events(tenant_id, feature);

-- anomaly_alerts is the real "proactively surfaced" record (see
-- anomaly_watch.go): today's anomaly detection is 100% pull (an operator
-- must open the Anomalies page or ask the chat assistant), so this table
-- exists to let a background loop actually notice a new anomaly and make
-- it visible without anyone having to go looking. Keyed by
-- (tenant_id, cluster_id, snapshot_reported_at) -- the real timestamp of
-- the snapshot the anomaly was computed from -- so the same underlying
-- anomaly (still true, nothing new happened) is never re-alerted as if
-- it were a fresh event.
CREATE TABLE IF NOT EXISTS anomaly_alerts (
	id                    INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id             INTEGER NOT NULL,
	cluster_id            TEXT NOT NULL,
	detected_at           INTEGER NOT NULL,
	snapshot_reported_at  INTEGER NOT NULL,
	previous_cost_inr     REAL NOT NULL,
	current_cost_inr      REAL NOT NULL,
	growth_ratio          REAL NOT NULL,
	acknowledged_at       INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_anomaly_alerts_identity
	ON anomaly_alerts(tenant_id, cluster_id, snapshot_reported_at);

CREATE TABLE IF NOT EXISTS settings (
	tenant_id INTEGER NOT NULL DEFAULT 1,
	key       TEXT NOT NULL,
	value     TEXT NOT NULL,
	PRIMARY KEY (tenant_id, key)
);

CREATE TABLE IF NOT EXISTS tenants (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id     INTEGER NOT NULL REFERENCES tenants(id),
	username      TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS api_tokens (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
	token_hash TEXT NOT NULL UNIQUE,
	label      TEXT,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id),
	tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS team_ownership (
	tenant_id  INTEGER NOT NULL,
	namespace  TEXT NOT NULL,
	team       TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (tenant_id, namespace)
);

CREATE TABLE IF NOT EXISTS deploy_event (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id   INTEGER NOT NULL,
	cluster_id  TEXT NOT NULL,
	namespace   TEXT NOT NULL,
	name        TEXT NOT NULL,
	reason      TEXT NOT NULL,
	message     TEXT,
	occurred_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_deploy_event_tenant_cluster_time ON deploy_event(tenant_id, cluster_id, occurred_at);

CREATE TABLE IF NOT EXISTS invites (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id  INTEGER NOT NULL REFERENCES tenants(id),
	email      TEXT NOT NULL UNIQUE,
	role       TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

-- Closed-loop outcome tracking (build manual's AI-roadmap groundwork):
-- every real fix recommendation surfaced on the dashboard is logged here,
-- and if an operator marks it applied, this is where the real before/after
-- cost gets recorded once fresher data confirms whether it actually
-- worked -- not a fabricated ROI number, an observed one.
CREATE TABLE IF NOT EXISTS recommendation_outcomes (
	id                        INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id                 INTEGER NOT NULL REFERENCES tenants(id),
	cluster_id                TEXT NOT NULL,
	source                    TEXT NOT NULL,
	destination               TEXT NOT NULL,
	path_class                TEXT NOT NULL,
	fix_hint                  TEXT NOT NULL,
	predicted_savings_low_inr REAL NOT NULL,
	predicted_savings_high_inr REAL NOT NULL,
	cost_before_inr           REAL NOT NULL,
	first_shown_at            INTEGER NOT NULL,
	last_shown_at             INTEGER NOT NULL,
	applied_at                INTEGER,
	cost_after_inr            REAL,
	measured_at               INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_recommendation_outcomes_identity
	ON recommendation_outcomes(tenant_id, cluster_id, source, destination, path_class);
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

	// cloud/region were added after this table's original release too --
	// same migration pattern, same reason.
	for _, col := range []string{"cloud", "region"} {
		if _, err := db.Exec(`ALTER TABLE flow_aggregate ADD COLUMN ` + col + ` TEXT`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate %s column: %w", col, err)
		}
	}

	// savings_low_inr/savings_high_inr were added once real optimization
	// recommendations shipped -- same migration pattern again.
	for _, col := range []string{"savings_low_inr", "savings_high_inr"} {
		if _, err := db.Exec(`ALTER TABLE flow_aggregate ADD COLUMN ` + col + ` REAL`); err != nil &&
			!strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate %s column: %w", col, err)
		}
	}

	// tenant_id was added once real multi-tenant isolation shipped. Every
	// pre-existing row (from before tenants existed at all) backfills to
	// tenant 1 -- main.go's bootstrap always creates the first tenant with
	// that id, so existing single-tenant installs keep working against
	// their own data after the upgrade instead of it becoming orphaned.
	if _, err := db.Exec(`ALTER TABLE flow_aggregate ADD COLUMN tenant_id INTEGER NOT NULL DEFAULT 1`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("migrate tenant_id column on flow_aggregate: %w", err)
	}

	// A covering index on (tenant_id, cluster_id, dst_workload_or_endpoint)
	// was tried here to speed up HistoricalFlowsToDestination
	// (traffic_replay.go) and deliberately REVERTED after measuring it on
	// the real live system, not in theory. It did make the lookup itself
	// ~4x faster (4.24s -> 1.0s per query, EXPLAIN QUERY PLAN confirmed it
	// was being used, measured against a real 4.1M-row copy of the
	// production database) -- but indexing a TEXT column on a table under
	// continuous real agent ingest cost far more than it saved: real
	// live measurement after deploying it showed /api/v1/findings, a read
	// path this feature never touches, regressing from 3.9s to 36-45s with
	// the pod pegged above a full CPU core and a 162MB WAL that stopped
	// checkpointing. The write amplification on every ingest overwhelmed
	// SQLite's single writer and queued the readers behind it.
	//
	// Not needed anyway: SimulateTrafficReplay only runs this lookup for
	// fixes that already cleared every other policy bar (remediation.go),
	// so it's a rare query, and paying a real ~4s once for a genuine
	// safety check beats permanently taxing every real ingest.
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_flow_aggregate_tenant_cluster_dst`); err != nil {
		db.Close()
		return nil, fmt.Errorf("drop reverted flow_aggregate destination index: %w", err)
	}

	// supabase_user_id links a users row to a real Supabase Auth identity
	// once Supabase-backed signup shipped -- nullable, since CLI-provisioned
	// (create-user) rows have no Supabase account at all. SQLite's
	// ALTER TABLE ADD COLUMN can't declare UNIQUE directly, so the
	// uniqueness constraint is a separate partial index: WHERE ... IS NOT
	// NULL means many rows can share a NULL (every CLI-provisioned user)
	// while still enforcing real uniqueness among the ones that do have a
	// Supabase identity.
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN supabase_user_id TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		db.Close()
		return nil, fmt.Errorf("migrate supabase_user_id column: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_supabase_user_id ON users(supabase_user_id) WHERE supabase_user_id IS NOT NULL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create supabase_user_id index: %w", err)
	}

	// settings needs more than ADD COLUMN: its original schema made `key`
	// alone the PRIMARY KEY, so two tenants both setting e.g. "budget_inr"
	// would silently overwrite each other's value. SQLite can't ALTER a
	// primary key in place, so a database still on the old shape gets its
	// settings table rebuilt with the real (tenant_id, key) composite key --
	// existing rows backfill to tenant 1, same as flow_aggregate above.
	var settingsCreateSQL string
	err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'settings'`).Scan(&settingsCreateSQL)
	if err != nil && err != sql.ErrNoRows {
		db.Close()
		return nil, fmt.Errorf("inspect settings schema: %w", err)
	}
	if err == nil && !strings.Contains(settingsCreateSQL, "PRIMARY KEY (tenant_id, key)") {
		migration := []string{
			`CREATE TABLE settings_new (
				tenant_id INTEGER NOT NULL DEFAULT 1,
				key       TEXT NOT NULL,
				value     TEXT NOT NULL,
				PRIMARY KEY (tenant_id, key)
			)`,
			`INSERT INTO settings_new (tenant_id, key, value) SELECT 1, key, value FROM settings`,
			`DROP TABLE settings`,
			`ALTER TABLE settings_new RENAME TO settings`,
		}
		tx, err := db.Begin()
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("begin settings migration: %w", err)
		}
		for _, stmt := range migration {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				db.Close()
				return nil, fmt.Errorf("migrate settings table (%s): %w", stmt, err)
			}
		}
		if err := tx.Commit(); err != nil {
			db.Close()
			return nil, fmt.Errorf("commit settings migration: %w", err)
		}
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Ingest records one full snapshot from a cluster's agent as a batch of
// rows sharing one reported_at timestamp, tagged with the tenant the
// ingesting API token resolved to -- this is the one write path where
// tenant isolation actually gets enforced; every read path below just
// trusts that every row already carries the right tenant_id.
func (s *Store) Ingest(tenantID int64, clusterID string, findings []Finding) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	reportedAt := time.Now().Unix()

	stmt, err := tx.Prepare(`
		INSERT INTO flow_aggregate
			(tenant_id, cluster_id, reported_at, src_workload, dst_workload_or_endpoint,
			 path_class, confidence, bytes_tx, bytes_rx, cost_low_inr, cost_high_inr, fix_hint, fix_manifest,
			 cloud, region, savings_low_inr, savings_high_inr)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, f := range findings {
		if _, err := stmt.Exec(
			tenantID, clusterID, reportedAt, f.Source, f.Destination,
			f.PathClass, f.Confidence, f.BytesTx, f.BytesRx,
			f.CostLowINR, f.CostHighINR, f.FixHint, f.FixManifest,
			f.Cloud, f.Region, f.SavingsLowINR, f.SavingsHighINR,
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
	// Cloud/Region reflect whichever price book the agent that shipped
	// this snapshot was configured with (MIN() is arbitrary but stable —
	// in practice one cluster's agent uses one price book, so every row
	// in a snapshot carries the same value).
	Cloud  string
	Region string
}

// Clusters returns every distinct cluster that has ever ingested under the
// given tenant, with its most recent snapshot's totals. Scoped by
// tenant_id at every level (the CTE and the join) so a cluster ID reused
// by two different tenants -- unlikely, but not something to trust --
// never merges their data.
func (s *Store) Clusters(tenantID int64) ([]ClusterSummary, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			WHERE tenant_id = ?
			GROUP BY cluster_id
		)
		SELECT fa.cluster_id, l.max_time, COUNT(*), COALESCE(SUM(fa.cost_low_inr), 0), COALESCE(SUM(fa.cost_high_inr), 0),
		       COALESCE(MIN(NULLIF(fa.cloud, '')), ''), COALESCE(MIN(NULLIF(fa.region, '')), '')
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		WHERE fa.tenant_id = ?
		GROUP BY fa.cluster_id, l.max_time
		ORDER BY l.max_time DESC
	`, tenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	out := make([]ClusterSummary, 0)
	for rows.Next() {
		var c ClusterSummary
		var lastSeen int64
		if err := rows.Scan(
			&c.ClusterID, &lastSeen, &c.FindingCount, &c.CostLowINR, &c.CostHighINR,
			&c.Cloud, &c.Region,
		); err != nil {
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

// LatestFindings returns the current state across every cluster belonging
// to the given tenant — each cluster's most recent snapshot, unioned and
// ranked by cost — capped at limit rows. This is the "multi-cluster view"
// (FR-C1), now scoped to one tenant's clusters rather than every cluster
// this control plane has ever ingested from.
func (s *Store) LatestFindings(tenantID int64, limit int) ([]FindingRow, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			WHERE tenant_id = ?
			GROUP BY cluster_id
		)
		SELECT fa.cluster_id, fa.reported_at, fa.src_workload, fa.dst_workload_or_endpoint,
		       fa.path_class, fa.confidence, fa.bytes_tx, fa.bytes_rx,
		       fa.cost_low_inr, fa.cost_high_inr, fa.fix_hint, fa.fix_manifest,
		       fa.cloud, fa.region, fa.savings_low_inr, fa.savings_high_inr
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		WHERE fa.tenant_id = ?
		ORDER BY fa.cost_high_inr DESC
		LIMIT ?
	`, tenantID, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("query latest findings: %w", err)
	}
	defer rows.Close()

	out := make([]FindingRow, 0)
	for rows.Next() {
		var r FindingRow
		var reportedAt int64
		var fixHint, fixManifest, cloud, region sql.NullString
		var savingsLow, savingsHigh sql.NullFloat64

		if err := rows.Scan(
			&r.ClusterID, &reportedAt, &r.Source, &r.Destination,
			&r.PathClass, &r.Confidence, &r.BytesTx, &r.BytesRx,
			&r.CostLowINR, &r.CostHighINR, &fixHint, &fixManifest,
			&cloud, &region, &savingsLow, &savingsHigh,
		); err != nil {
			return nil, fmt.Errorf("scan finding row: %w", err)
		}

		r.ReportedAt = time.Unix(reportedAt, 0)
		r.FixHint = fixHint.String
		r.FixManifest = fixManifest.String
		r.Cloud = cloud.String
		r.Region = region.String
		r.SavingsLowINR = savingsLow.Float64
		r.SavingsHighINR = savingsHigh.Float64
		out = append(out, r)
	}

	return out, rows.Err()
}

// HistoricalFlowsToDestination returns the real summed cost per source
// workload for every real flow this tenant's cluster has ever recorded to
// one specific destination — the data traffic_replay.go's
// EvaluateTrafficReplay needs to answer "would this generated
// NetworkPolicy also block any other real workload's traffic to the same
// destination?" Scans flow_aggregate UNION ALL flow_aggregate_daily (same
// pattern as WorkloadCostGrowth, workloadgrowth.go) so compaction folding
// old raw rows into daily rollups can't silently make old collateral
// traffic invisible to this safety check.
func (s *Store) HistoricalFlowsToDestination(tenantID int64, clusterID, destination string) ([]HistoricalFlowCost, error) {
	rows, err := s.db.Query(`
		SELECT src_workload, SUM(cost) FROM (
			SELECT src_workload, cost_high_inr AS cost
			FROM flow_aggregate
			WHERE tenant_id = ? AND cluster_id = ? AND dst_workload_or_endpoint = ?
			UNION ALL
			SELECT src_workload, cost_high_inr AS cost
			FROM flow_aggregate_daily
			WHERE tenant_id = ? AND cluster_id = ? AND dst_workload_or_endpoint = ?
		) combined
		GROUP BY src_workload
	`, tenantID, clusterID, destination, tenantID, clusterID, destination)
	if err != nil {
		return nil, fmt.Errorf("query historical flows to destination: %w", err)
	}
	defer rows.Close()

	out := make([]HistoricalFlowCost, 0)
	for rows.Next() {
		var hf HistoricalFlowCost
		if err := rows.Scan(&hf.SrcWorkload, &hf.CostHighINR); err != nil {
			return nil, fmt.Errorf("scan historical flow cost: %w", err)
		}
		out = append(out, hf)
	}

	return out, rows.Err()
}

// Summary is the aggregate, cross-cluster headline state — every number
// here is a real sum/count over the latest snapshot per cluster, nothing
// modeled or estimated beyond what CostINR already prices.
type Summary struct {
	ClusterCount     int
	WorkloadCount    int
	FindingCount     int
	TotalBytesTx     uint64
	TotalBytesRx     uint64
	TotalCostLowINR  float64
	TotalCostHighINR float64
}

// Summary aggregates the current (latest-per-cluster) state for one
// tenant into the headline numbers a dashboard's stat row needs.
func (s *Store) Summary(tenantID int64) (Summary, error) {
	var out Summary

	row := s.db.QueryRow(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			WHERE tenant_id = ?
			GROUP BY cluster_id
		)
		SELECT
			COUNT(DISTINCT fa.cluster_id),
			COUNT(DISTINCT fa.src_workload),
			COUNT(*),
			COALESCE(SUM(fa.bytes_tx), 0),
			COALESCE(SUM(fa.bytes_rx), 0),
			COALESCE(SUM(fa.cost_low_inr), 0),
			COALESCE(SUM(fa.cost_high_inr), 0)
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		WHERE fa.tenant_id = ?
	`, tenantID, tenantID)

	if err := row.Scan(
		&out.ClusterCount, &out.WorkloadCount, &out.FindingCount,
		&out.TotalBytesTx, &out.TotalBytesRx,
		&out.TotalCostLowINR, &out.TotalCostHighINR,
	); err != nil {
		return Summary{}, fmt.Errorf("query summary: %w", err)
	}

	return out, nil
}

// ClassSpend is the total cost attributed to one path class, across the
// latest snapshot per cluster.
type ClassSpend struct {
	PathClass    string
	CostHighINR  float64
	FindingCount int
}

// SpendByClass groups one tenant's current state by path class, highest
// cost first — the real data behind a "spend by category" breakdown.
func (s *Store) SpendByClass(tenantID int64) ([]ClassSpend, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			WHERE tenant_id = ?
			GROUP BY cluster_id
		)
		SELECT fa.path_class, COALESCE(SUM(fa.cost_high_inr), 0), COUNT(*)
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		WHERE fa.tenant_id = ?
		GROUP BY fa.path_class
		ORDER BY SUM(fa.cost_high_inr) DESC
	`, tenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query spend by class: %w", err)
	}
	defer rows.Close()

	out := make([]ClassSpend, 0)
	for rows.Next() {
		var c ClassSpend
		if err := rows.Scan(&c.PathClass, &c.CostHighINR, &c.FindingCount); err != nil {
			return nil, fmt.Errorf("scan class spend: %w", err)
		}
		out = append(out, c)
	}

	return out, rows.Err()
}

// CloudSpend is the total cost attributed to one (cloud, region) pair,
// across the latest snapshot per cluster. Real infrastructure, honestly
// scoped: with only one price book (AWS/ap-south-1) configured across
// every agent today, this correctly shows a single 100% slice -- it
// becomes a genuine multi-cloud breakdown the moment a second cluster
// ships with a different price book, not before.
type CloudSpend struct {
	Cloud        string
	Region       string
	CostHighINR  float64
	FindingCount int
}

// SpendByCloud groups one tenant's current state by (cloud, region),
// highest cost first. Rows with no cloud/region recorded (findings shipped
// before this field existed) fold into a single "unknown" bucket rather
// than being silently dropped.
func (s *Store) SpendByCloud(tenantID int64) ([]CloudSpend, error) {
	rows, err := s.db.Query(`
		WITH latest AS (
			SELECT cluster_id, MAX(reported_at) AS max_time
			FROM flow_aggregate
			WHERE tenant_id = ?
			GROUP BY cluster_id
		)
		SELECT COALESCE(NULLIF(fa.cloud, ''), 'unknown'), COALESCE(NULLIF(fa.region, ''), 'unknown'),
		       COALESCE(SUM(fa.cost_high_inr), 0), COUNT(*)
		FROM flow_aggregate fa
		JOIN latest l ON fa.cluster_id = l.cluster_id AND fa.reported_at = l.max_time
		WHERE fa.tenant_id = ?
		GROUP BY 1, 2
		ORDER BY SUM(fa.cost_high_inr) DESC
	`, tenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query spend by cloud: %w", err)
	}
	defer rows.Close()

	out := make([]CloudSpend, 0)
	for rows.Next() {
		var c CloudSpend
		if err := rows.Scan(&c.Cloud, &c.Region, &c.CostHighINR, &c.FindingCount); err != nil {
			return nil, fmt.Errorf("scan cloud spend: %w", err)
		}
		out = append(out, c)
	}

	return out, rows.Err()
}

// GlobalTrend sums cost across every cluster at each reported_at, oldest
// first. Snapshots across independently-scheduled agents won't align
// perfectly, so this is a real but approximate combined trend — accurate
// per-cluster trends are what CostTrend is for.
func (s *Store) GlobalTrend(tenantID int64, maxPoints int) ([]CostTrendPoint, error) {
	rows, err := s.db.Query(`
		SELECT reported_at, SUM(cost_high_inr)
		FROM flow_aggregate
		WHERE tenant_id = ?
		GROUP BY reported_at
		ORDER BY reported_at DESC
		LIMIT ?
	`, tenantID, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("query global trend: %w", err)
	}
	defer rows.Close()

	out := make([]CostTrendPoint, 0)
	for rows.Next() {
		var p CostTrendPoint
		var reportedAt int64
		if err := rows.Scan(&reportedAt, &p.CostHigh); err != nil {
			return nil, fmt.Errorf("scan global trend point: %w", err)
		}
		p.ReportedAt = time.Unix(reportedAt, 0)
		out = append(out, p)
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
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
func (s *Store) CostTrend(tenantID int64, clusterID string, maxPoints int) ([]CostTrendPoint, error) {
	rows, err := s.db.Query(`
		SELECT reported_at, SUM(cost_high_inr)
		FROM flow_aggregate
		WHERE tenant_id = ? AND cluster_id = ?
		GROUP BY reported_at
		ORDER BY reported_at DESC
		LIMIT ?
	`, tenantID, clusterID, maxPoints)
	if err != nil {
		return nil, fmt.Errorf("query cost trend: %w", err)
	}
	defer rows.Close()

	out := make([]CostTrendPoint, 0)
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

// budgetSettingKey is the settings table key for the one real, user-set
// budget figure this control plane supports: a single overall INR ceiling
// compared against real total spend. Not per-cluster, not per-period —
// deliberately as simple as "budget status" can honestly be without
// inventing a fiscal-calendar/rollover model nobody asked for.
const budgetSettingKey = "budget_inr"

// SetBudget stores one tenant's user-set budget figure, overwriting any
// previous value for that same tenant only.
func (s *Store) SetBudget(tenantID int64, amountINR float64) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (tenant_id, key, value) VALUES (?, ?, ?)
		 ON CONFLICT(tenant_id, key) DO UPDATE SET value = excluded.value`,
		tenantID, budgetSettingKey, strconv.FormatFloat(amountINR, 'f', -1, 64),
	)
	if err != nil {
		return fmt.Errorf("set budget: %w", err)
	}

	return nil
}

// GetBudget returns one tenant's stored budget and whether one has been
// set at all — a zero budget and "never set" are different states; the
// frontend treats the latter as "no budget configured" rather than
// "budget is ₹0."
func (s *Store) GetBudget(tenantID int64) (amountINR float64, isSet bool, err error) {
	var raw string

	err = s.db.QueryRow(`SELECT value FROM settings WHERE tenant_id = ? AND key = ?`, tenantID, budgetSettingKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("get budget: %w", err)
	}

	amountINR, err = strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse stored budget %q: %w", raw, err)
	}

	return amountINR, true, nil
}
