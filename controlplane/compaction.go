// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"time"
)

// DefaultRawRetention is how long a raw flow_aggregate row (one per real
// ingest cycle, every ~10-40s per real agent) is kept before being folded
// into flow_aggregate_daily and deleted. This is the real "automated
// retention/compaction" fix for flow_aggregate's measured unbounded growth
// (2.2M+ rows / 550MB+ on the live cluster, "never pruned, by design" --
// see OpenStore's comment in store.go) -- chosen over swapping the storage
// engine for something like ClickHouse because at this project's actual
// real scale (one real tenant, SQLite+WAL already fixed in §3.15 to
// genuinely handle millions of rows fine) a storage-engine migration is
// real, costly over-engineering for a problem this solves directly: every
// dashboard query that reads flow_aggregate either only wants the latest
// snapshot per cluster (Clusters, LatestFindings, Summary, SpendByClass,
// SpendByCloud, the outcome-measurement queries in outcome.go) or a
// recent, already-bounded window (GlobalTrend/CostTrend/forecast, capped
// at forecastQueryWindow=1500 points) -- none of them need fine-grained
// history older than a few days. The one real exception is
// WorkloadCostGrowth, which genuinely wants "cost change over this
// workload's full retained history" -- it reads the daily rollup too (see
// workloadgrowth.go) specifically so compaction doesn't quietly erase the
// old data that "full history" honestly means.
//
// 30 days is deliberately generous, not tuned to the live cluster's
// current ~week of real history: it's chosen so this never deletes real
// data an operator would still expect at raw granularity on day one, and
// only starts bounding growth once real retained history actually
// accumulates past it.
const DefaultRawRetention = 30 * 24 * time.Hour

// CompactFindingsOlderThan aggregates every real flow_aggregate row
// reported before cutoff into flow_aggregate_daily -- summed per real day
// (UTC, truncated by integer division on the unix timestamp), tenant,
// cluster, and workload pair -- then deletes the raw rows just folded in.
// Both the rollup writes and the raw deletes happen inside one
// transaction, committed only after every rollup upsert succeeds, so a
// crash partway through can never lose raw data without its aggregate
// already durably written first.
//
// Idempotent by construction: a real row's reported_at never moves once
// written (ingest always stamps time.Now(), never backdates), so a day
// once compacted and deleted can never be compacted again from the same
// source rows. The upsert still accumulates on conflict rather than
// overwriting, as a real defensive correctness measure in case this is
// ever invoked twice over overlapping cutoffs (e.g. a manual re-run) --
// cheap to get right, not defending against something expected to happen.
func (s *Store) CompactFindingsOlderThan(cutoff time.Time) (groupsCompacted int, rawRowsDeleted int, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("begin compaction tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op once Commit succeeds

	rows, err := tx.Query(`
		SELECT tenant_id, cluster_id, (reported_at / 86400) * 86400 AS day,
		       src_workload, dst_workload_or_endpoint, path_class,
		       SUM(bytes_tx), SUM(bytes_rx), SUM(cost_low_inr), SUM(cost_high_inr),
		       SUM(COALESCE(savings_low_inr, 0)), SUM(COALESCE(savings_high_inr, 0)), COUNT(*)
		FROM flow_aggregate
		WHERE reported_at < ?
		GROUP BY tenant_id, cluster_id, day, src_workload, dst_workload_or_endpoint, path_class
	`, cutoff.Unix())
	if err != nil {
		return 0, 0, fmt.Errorf("query rows to compact: %w", err)
	}

	type dailyGroup struct {
		tenantID                    int64
		clusterID, src, dst, pclass string
		day                         int64
		bytesTx, bytesRx            int64
		costLow, costHigh           float64
		savingsLow, savingsHigh     float64
		sampleCount                 int64
	}

	var groups []dailyGroup
	for rows.Next() {
		var g dailyGroup
		if err := rows.Scan(
			&g.tenantID, &g.clusterID, &g.day, &g.src, &g.dst, &g.pclass,
			&g.bytesTx, &g.bytesRx, &g.costLow, &g.costHigh, &g.savingsLow, &g.savingsHigh, &g.sampleCount,
		); err != nil {
			rows.Close()
			return 0, 0, fmt.Errorf("scan compaction group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, 0, fmt.Errorf("iterate compaction groups: %w", err)
	}
	rows.Close()

	if len(groups) == 0 {
		return 0, 0, tx.Commit()
	}

	upsert, err := tx.Prepare(`
		INSERT INTO flow_aggregate_daily
			(tenant_id, cluster_id, day, src_workload, dst_workload_or_endpoint, path_class,
			 bytes_tx, bytes_rx, cost_low_inr, cost_high_inr, savings_low_inr, savings_high_inr, sample_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tenant_id, cluster_id, day, src_workload, dst_workload_or_endpoint, path_class) DO UPDATE SET
			bytes_tx         = bytes_tx + excluded.bytes_tx,
			bytes_rx         = bytes_rx + excluded.bytes_rx,
			cost_low_inr     = cost_low_inr + excluded.cost_low_inr,
			cost_high_inr    = cost_high_inr + excluded.cost_high_inr,
			savings_low_inr  = savings_low_inr + excluded.savings_low_inr,
			savings_high_inr = savings_high_inr + excluded.savings_high_inr,
			sample_count     = sample_count + excluded.sample_count
	`)
	if err != nil {
		return 0, 0, fmt.Errorf("prepare daily rollup upsert: %w", err)
	}
	defer upsert.Close()

	for _, g := range groups {
		if _, err := upsert.Exec(
			g.tenantID, g.clusterID, g.day, g.src, g.dst, g.pclass,
			g.bytesTx, g.bytesRx, g.costLow, g.costHigh, g.savingsLow, g.savingsHigh, g.sampleCount,
		); err != nil {
			return 0, 0, fmt.Errorf("upsert daily rollup: %w", err)
		}
	}

	res, err := tx.Exec(`DELETE FROM flow_aggregate WHERE reported_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, 0, fmt.Errorf("delete compacted raw rows: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, 0, fmt.Errorf("count deleted rows: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit compaction: %w", err)
	}

	return len(groups), int(deleted), nil
}

// StartCompactionLoop runs CompactFindingsOlderThan once immediately, then
// on every tick, until stop is closed -- a real background maintenance
// loop, not a one-shot admin command, since this control plane already
// runs as one long-lived server process (main.go) with no separate
// cron/worker deployment to hand periodic jobs to. Errors are logged, not
// fatal: a transient failure (e.g. a concurrent write holding the SQLite
// write lock past busy_timeout) should retry next tick, not crash a
// running dashboard server over background maintenance.
func (s *Store) StartCompactionLoop(retention time.Duration, interval time.Duration, stop <-chan struct{}, logf func(format string, args ...any)) {
	runOnce := func() {
		cutoff := time.Now().Add(-retention)
		groups, deleted, err := s.CompactFindingsOlderThan(cutoff)
		if err != nil {
			logf("compaction: failed against cutoff %s: %v", cutoff.Format(time.RFC3339), err)
			return
		}
		if deleted > 0 {
			logf("compaction: folded %d raw row(s) into %d daily rollup group(s) older than %s", deleted, groups, cutoff.Format(time.RFC3339))
		}
	}

	runOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			runOnce()
		case <-stop:
			return
		}
	}
}
